// Package pac evaluates proxy auto-config (PAC) scripts in pure Go.
//
// A PAC file can be fetched over plain HTTP, so it is treated as untrusted input.
// Before a script runs, Compile rejects a script with more than maxOpeners "(",
// "[" and "{" in total (strings and comments included): otto's recursive-descent
// parser overflows the Go stack, which no recover can catch, at roughly 225,000
// nested "(" or "[" (about 450 KB, under MaxSize). Nesting can never be deeper than
// the number of openers, so nothing in the script can hide nesting from this cap.
//
// Every evaluation, including compiling the script's top level, is then bounded:
//   - a time budget, enforced through otto's interrupt channel (checked between
//     statements) and by abandoning the evaluation if it has not finished (a
//     native builtin can ignore the interrupt);
//   - a heap-growth check, sampled while the script runs. One native builtin call
//     cannot be interrupted and can allocate far past the limit before it
//     returns; the evaluation is then abandoned;
//   - a JavaScript call-depth limit, so unbounded JavaScript recursion becomes a
//     script error instead of a Go stack overflow (parser depth is the opener
//     cap above);
//   - one evaluation at a time, so the process-wide heap check of one script is
//     not tripped by another (real evaluations take milliseconds);
//   - a cap on abandoned evaluations still running, so their goroutines cannot
//     pile up (evaluations that finish normally never count).
//
// File adds a size limit, a fetch-failure cache and a backoff after a stalled run.
//
// Only the macOS proxy source imports this package, so builds for other
// platforms do not include the JavaScript interpreter.
package pac

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/robertkrimen/otto"
)

// ErrTimeout is returned when a script does not finish within its budget.
var ErrTimeout = errors.New("pac: evaluation timed out")

// ErrMemory is returned when the heap grows by more than memoryLimit during an
// evaluation.
var ErrMemory = errors.New("pac: evaluation stopped: memory limit exceeded")

// ErrAbandoned is wrapped into ErrTimeout or ErrMemory when the evaluation did
// not stop at the interrupt within stopGrace (a native builtin is still running),
// so its goroutine was left behind.
var ErrAbandoned = errors.New("pac: evaluation abandoned while still running")

// ErrBusy is returned when maxAbandoned evaluations were abandoned (after a
// timeout or memory stop) and are still running, so no further goroutine is
// started, or when the queue for the evaluation slot stopped moving (or exceeded
// maxSlotWait). Healthy
// evaluations running at the same time never count against the cap.
var ErrBusy = errors.New("pac: too busy to evaluate the PAC script now")

const (
	// stackDepthLimit bounds JavaScript call depth. Real PAC files are flat; this
	// is far above anything legitimate and far below Go's own stack limit.
	stackDepthLimit = 500
	// memoryLimit is the heap growth allowed while a script runs. Real PAC
	// evaluation allocates kilobytes.
	memoryLimit = 256 << 20 // 256 MiB
	// memoryCheckEvery is how often the heap is sampled during an evaluation.
	memoryCheckEvery = 20 * time.Millisecond
	// stopGrace is how long run waits, after asking the VM to stop, to see whether
	// it stopped (it checks the interrupt between statements) before treating the
	// evaluation as abandoned.
	stopGrace = 100 * time.Millisecond
	// maxOpeners is the most "(", "[" and "{" a script may contain in total. It is
	// the guard against the parser stack overflow (near 225,000 nested), with a
	// wide margin; real PAC files are a few KB and stay far below it.
	maxOpeners = 50000
	// maxNesting is the deepest nesting Compile accepts, for a clear error on a
	// deeply nested file. It is not the guard (see nestingDepth).
	maxNesting = 1000
	// maxAbandoned bounds evaluations that were abandoned but have not returned,
	// so a builtin that ignores the interrupt cannot pile up goroutines.
	maxAbandoned = 4
)

// abandoned counts abandoned evaluations whose goroutine has not returned.
var abandoned atomic.Int32

// evalSlot lets one evaluation run at a time. It is released when run returns,
// including when it gives up on an abandoned goroutine, so a stuck builtin does
// not hold it.
var evalSlot = make(chan struct{}, 1)

// slotReleases counts releases of evalSlot. A waiter keeps waiting while the queue
// is moving (some evaluation finishes within each window of two budgets), so a
// burst of slow but healthy first lookups queues instead of being refused. It
// gives up only if a whole window passes with no release, or after maxSlotWait.
var slotReleases atomic.Uint64

// maxSlotWait caps how long an evaluation waits for its turn.
const maxSlotWait = time.Minute

// vmHook, when set (tests only), is called on each new VM after the PAC helpers
// are registered.
var vmHook func(*otto.Otto)

// States of one evaluation, so that exactly one side (the goroutine finishing,
// or run giving up on it) decides whether it counts as abandoned.
const (
	evalRunning int32 = iota
	evalFinished
	evalAbandoned
)

type halt struct{ err error }

// Script is a compiled PAC script. It is safe for concurrent use: each
// evaluation runs in its own VM.
type Script struct {
	src string
}

// Compile checks, within budget, that src runs and defines FindProxyForURL.
func Compile(src string, budget time.Duration) (*Script, error) {
	if n := openerCount(src); n > maxOpeners {
		return nil, fmt.Errorf("pac: script has %d brackets (limit %d)", n, maxOpeners)
	}
	if d := nestingDepth(src); d > maxNesting {
		return nil, fmt.Errorf("pac: script nests brackets %d deep (limit %d)", d, maxNesting)
	}
	_, err := run(src, budget, func(vm *otto.Otto) (string, error) {
		f, err := vm.Get("FindProxyForURL")
		if err != nil || !f.IsFunction() {
			return "", errors.New("pac: script does not define FindProxyForURL")
		}
		return "", nil
	})
	if err != nil {
		return nil, err
	}
	return &Script{src: src}, nil
}

// openerCount counts every "(", "[" and "{" in src, strings and comments
// included. It is the real guard: it needs no parsing, so nothing can hide from it.
func openerCount(src string) int {
	return strings.Count(src, "(") + strings.Count(src, "[") + strings.Count(src, "{")
}

// nestingDepth returns the deepest nesting of "(", "[" and "{" in src, outside
// string literals and comments, for a clearer error on deeply nested files. It does
// not recognise regular-expression literals, so it can be fooled in either
// direction (a regex containing a quote or "//" makes it skip code, so it
// undercounts). It is therefore not a safety check; openerCount is.
func nestingDepth(src string) int {
	depth, deepest := 0, 0
	for i := 0; i < len(src); i++ {
		switch c := src[i]; c {
		case '"', '\'':
			for i++; i < len(src) && src[i] != c && src[i] != '\n'; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				for i < len(src) && src[i] != '\n' {
					i++
				}
			} else if i+1 < len(src) && src[i+1] == '*' {
				end := strings.Index(src[i+2:], "*/")
				if end < 0 {
					return deepest
				}
				i += end + 3
			}
		case '(', '[', '{':
			if depth++; depth > deepest {
				deepest = depth
			}
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		}
	}
	return deepest
}

// FindProxyForURL runs the script for rawURL and returns its raw answer, such as
// "PROXY 127.0.0.1:9000; DIRECT", within budget.
func (s *Script) FindProxyForURL(rawURL string, budget time.Duration) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.New("pac: invalid URL")
	}
	host := u.Hostname()
	return run(s.src, budget, func(vm *otto.Otto) (string, error) {
		v, err := vm.Call("FindProxyForURL", nil, rawURL, host)
		if err != nil {
			return "", fmt.Errorf("pac: FindProxyForURL failed: %v", err)
		}
		if v.IsUndefined() || v.IsNull() {
			return "", nil
		}
		return v.ToString()
	})
}

type result struct {
	answer string
	err    error
}

// run executes src in a fresh VM and then calls after, all within budget and
// within memoryLimit of heap growth.
func run(src string, budget time.Duration, after func(*otto.Otto) (string, error)) (string, error) {
	if n := abandoned.Load(); n >= maxAbandoned {
		return "", fmt.Errorf("%w: %d abandoned evaluations still running", ErrBusy, n)
	}
	if err := acquireSlot(budget); err != nil {
		return "", err
	}
	defer func() {
		<-evalSlot
		slotReleases.Add(1)
	}()
	vm := otto.New()
	if err := register(vm); err != nil {
		return "", err
	}
	if vmHook != nil {
		vmHook(vm)
	}
	vm.SetStackDepthLimit(stackDepthLimit)
	vm.Interrupt = make(chan func(), 1)
	var state atomic.Int32 // evalRunning
	stop := func(err error) {
		select {
		case vm.Interrupt <- func() { panic(halt{err: err}) }:
		default:
		}
		if state.CompareAndSwap(evalRunning, evalAbandoned) {
			abandoned.Add(1)
		}
	}

	// The heap baseline is read before the script starts, so growth from its very
	// first statement is counted.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc

	done := make(chan result, 1)
	go func() {
		defer func() {
			if !state.CompareAndSwap(evalRunning, evalFinished) {
				abandoned.Add(-1) // run gave up on us and counted us
			}
		}()
		defer func() {
			if r := recover(); r != nil {
				if h, ok := r.(halt); ok {
					done <- result{err: h.err}
					return
				}
				done <- result{err: fmt.Errorf("pac: script failed: %v", r)}
			}
		}()
		if _, err := vm.Run(src); err != nil {
			done <- result{err: fmt.Errorf("pac: script error: %v", err)}
			return
		}
		a, err := after(vm)
		done <- result{answer: a, err: err}
	}()

	// giveUp asks the VM to stop and waits up to stopGrace for it. If it stopped,
	// cause is returned as is; if not, its goroutine is left behind and the error
	// also wraps ErrAbandoned.
	giveUp := func(cause error) error {
		stop(cause)
		grace := time.NewTimer(stopGrace)
		defer grace.Stop()
		select {
		case <-done:
			return cause
		case <-grace.C:
			return fmt.Errorf("%w (%w)", cause, ErrAbandoned)
		}
	}

	tick := time.NewTicker(memoryCheckEvery)
	defer tick.Stop()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	for {
		select {
		case r := <-done:
			return r.answer, r.err
		case <-tick.C:
			// The VM stops at its next statement. Growth inside one native
			// builtin call is only seen here while it runs, and that call
			// cannot be interrupted; its goroutine is then abandoned.
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > base && ms.HeapAlloc-base > memoryLimit {
				return "", giveUp(ErrMemory)
			}
		case <-timer.C:
			// Ask the VM to stop at its next statement. A native builtin that is
			// still running cannot be stopped; its goroutine is abandoned (and
			// counted against maxAbandoned until it returns), and the caller
			// treats the timeout as a discovery failure.
			return "", giveUp(ErrTimeout)
		}
	}
}

// acquireSlot waits for evalSlot. An evaluation holds the slot for at most its
// budget plus stopGrace, so a window of twice that with no release means the
// queue is stuck; otherwise the wait continues, up to maxSlotWait in total.
func acquireSlot(budget time.Duration) error {
	window := 2 * (budget + stopGrace)
	start := time.Now()
	for {
		seen := slotReleases.Load()
		t := time.NewTimer(window)
		select {
		case evalSlot <- struct{}{}:
			t.Stop()
			return nil
		case <-t.C:
			if slotReleases.Load() == seen || time.Since(start) > maxSlotWait {
				return fmt.Errorf("%w: waited %v for the evaluation slot", ErrBusy, time.Since(start).Round(time.Millisecond))
			}
		}
	}
}

// FirstEntry returns the first entry of a PAC answer as a proxy "host:port", or ""
// for a direct connection. Only PROXY entries are supported (FR-003: first entry
// only); empty, DIRECT, malformed and unsupported entries (SOCKS, HTTPS, ...) mean
// direct, and ok is false for the malformed or unsupported cases so callers can
// log them.
func FirstEntry(answer string) (hostPort string, ok bool) {
	first := strings.TrimSpace(strings.SplitN(answer, ";", 2)[0])
	if first == "" {
		return "", true
	}
	fields := strings.Fields(first)
	switch strings.ToUpper(fields[0]) {
	case "DIRECT":
		return "", true
	case "PROXY":
		if len(fields) == 2 && strings.Contains(fields[1], ":") {
			return fields[1], true
		}
	}
	return "", false
}

var userinfo = regexp.MustCompile(`[^\s;/@]+@`)

// Redact removes any user information ("user:pass@") from a PAC answer, for logs.
func Redact(answer string) string { return userinfo.ReplaceAllString(answer, "") }
