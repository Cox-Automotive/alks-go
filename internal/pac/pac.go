// Package pac evaluates proxy auto-config (PAC) scripts in pure Go.
//
// Every evaluation, including compiling the script's top level, is bounded:
//   - a time budget, enforced through otto's interrupt channel (checked between
//     statements) and by abandoning the evaluation if it has not finished (a
//     native builtin can ignore the interrupt);
//   - a heap-growth limit, sampled while the script runs;
//   - a JavaScript stack-depth limit, so unbounded recursion becomes a script
//     error instead of a Go stack overflow that would kill the process;
//   - a cap on abandoned evaluations still running, so their goroutines cannot
//     pile up (evaluations that finish normally never count).
//
// File adds a size limit on the script and a backoff after an aborted run. A
// PAC file can be fetched over plain HTTP, so it is treated as untrusted input.
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
// started. Healthy evaluations running at the same time never count.
var ErrBusy = errors.New("pac: too many abandoned evaluations still running")

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
	// maxAbandoned bounds evaluations that were abandoned but have not returned,
	// so a builtin that ignores the interrupt cannot pile up goroutines.
	maxAbandoned = 4
)

// abandoned counts abandoned evaluations whose goroutine has not returned.
var abandoned atomic.Int32

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
	if abandoned.Load() >= maxAbandoned {
		return "", ErrBusy
	}
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
