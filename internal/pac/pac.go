// Package pac evaluates proxy auto-config (PAC) scripts in pure Go.
//
// Every evaluation, including compiling the script's top level, is bounded:
//   - a time budget, enforced through otto's interrupt channel (checked between
//     statements) and by abandoning the evaluation if it has not finished (a
//     native builtin can ignore the interrupt);
//   - a heap-growth limit, sampled while the script runs;
//   - a JavaScript stack-depth limit, so unbounded recursion becomes a script
//     error instead of a Go stack overflow that would kill the process;
//   - a cap on evaluations still running, so abandoned goroutines cannot pile up.
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

// ErrBusy is returned when maxRunning evaluations are already running (or were
// abandoned and have not finished), so no further goroutine is started.
var ErrBusy = errors.New("pac: too many evaluations still running")

const (
	// stackDepthLimit bounds JavaScript call depth. Real PAC files are flat; this
	// is far above anything legitimate and far below Go's own stack limit.
	stackDepthLimit = 500
	// memoryLimit is the heap growth allowed while a script runs. Real PAC
	// evaluation allocates kilobytes.
	memoryLimit = 256 << 20 // 256 MiB
	// memoryCheckEvery is how often the heap is sampled during an evaluation.
	memoryCheckEvery = 20 * time.Millisecond
	// maxRunning bounds concurrent evaluations, including abandoned ones, so a
	// builtin that ignores the interrupt cannot pile up goroutines.
	maxRunning = 4
)

// running counts evaluations whose goroutine has not returned.
var running atomic.Int32

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
	if running.Add(1) > maxRunning {
		running.Add(-1)
		return "", ErrBusy
	}
	vm := otto.New()
	if err := register(vm); err != nil {
		running.Add(-1)
		return "", err
	}
	vm.SetStackDepthLimit(stackDepthLimit)
	vm.Interrupt = make(chan func(), 1)
	stop := func(err error) {
		select {
		case vm.Interrupt <- func() { panic(halt{err: err}) }:
		default:
		}
	}

	done := make(chan result, 1)
	go func() {
		defer running.Add(-1)
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

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapAlloc
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
			// cannot be interrupted; the goroutine is then abandoned as for
			// a timeout.
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > base && ms.HeapAlloc-base > memoryLimit {
				stop(ErrMemory)
				return "", ErrMemory
			}
		case <-timer.C:
			// Ask the VM to stop at its next statement. A native builtin that is
			// still running cannot be stopped; its goroutine is abandoned (and
			// counted against maxRunning until it returns), and the caller treats
			// the timeout as a discovery failure.
			stop(ErrTimeout)
			return "", ErrTimeout
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
