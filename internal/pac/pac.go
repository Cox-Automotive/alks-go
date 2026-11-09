// Package pac evaluates proxy auto-config (PAC) scripts in pure Go.
//
// Every evaluation, including compiling the script's top level, is bounded:
// a time budget enforced both through otto's interrupt channel (checked between
// statements) and by abandoning the evaluation if it has not finished (a native
// builtin can ignore the interrupt), and a JavaScript stack-depth limit so that
// unbounded recursion becomes a script error instead of a Go stack overflow that
// would kill the process. A PAC file is fetched over plain HTTP, so it is
// treated as untrusted input.
package pac

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/robertkrimen/otto"
)

// ErrTimeout is returned when a script does not finish within its budget.
var ErrTimeout = errors.New("pac: evaluation timed out")

// stackDepthLimit bounds JavaScript call depth. Real PAC files are flat; this is
// far above anything legitimate and far below Go's own stack limit.
const stackDepthLimit = 500

type halt struct{}

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

// run executes src in a fresh VM and then calls after, all within budget.
func run(src string, budget time.Duration, after func(*otto.Otto) (string, error)) (string, error) {
	vm := otto.New()
	if err := register(vm); err != nil {
		return "", err
	}
	vm.SetStackDepthLimit(stackDepthLimit)
	vm.Interrupt = make(chan func(), 1)

	done := make(chan result, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(halt); ok {
					done <- result{err: ErrTimeout}
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

	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case r := <-done:
		return r.answer, r.err
	case <-timer.C:
		// Ask the VM to stop at its next statement. A native builtin that is
		// still running cannot be stopped; its goroutine is abandoned and the
		// caller treats the timeout as a discovery failure.
		select {
		case vm.Interrupt <- func() { panic(halt{}) }:
		default:
		}
		return "", ErrTimeout
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
