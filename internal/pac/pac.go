// Package pac evaluates proxy auto-config (PAC) scripts in pure Go.
package pac

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/robertkrimen/otto"
)

// ErrTimeout is returned when a script does not finish within its budget.
var ErrTimeout = errors.New("pac: evaluation timed out")

type halt struct{}

// Script is a compiled PAC script. It is safe for concurrent use: each
// evaluation runs in its own VM.
type Script struct {
	src string
}

// Compile checks that src parses and defines FindProxyForURL.
func Compile(src string) (*Script, error) {
	vm := otto.New()
	if err := register(vm); err != nil {
		return nil, err
	}
	if _, err := vm.Run(src); err != nil {
		return nil, fmt.Errorf("pac: script error: %v", err)
	}
	f, err := vm.Get("FindProxyForURL")
	if err != nil || !f.IsFunction() {
		return nil, errors.New("pac: script does not define FindProxyForURL")
	}
	return &Script{src: src}, nil
}

// FindProxyForURL runs the script for rawURL and returns its raw answer, such as
// "PROXY 127.0.0.1:9000; DIRECT". The whole evaluation, including DNS lookups made
// by the script, is bounded by budget.
func (s *Script) FindProxyForURL(rawURL string, budget time.Duration) (answer string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("pac: invalid URL: %v", err)
	}

	vm := otto.New()
	if err := register(vm); err != nil {
		return "", err
	}
	vm.Interrupt = make(chan func(), 1)

	timer := time.AfterFunc(budget, func() {
		vm.Interrupt <- func() { panic(halt{}) }
	})
	defer timer.Stop()

	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(halt); ok {
				answer, err = "", ErrTimeout
				return
			}
			panic(r)
		}
	}()

	if _, err := vm.Run(s.src); err != nil {
		return "", fmt.Errorf("pac: script error: %v", err)
	}
	v, err := vm.Call("FindProxyForURL", nil, rawURL, u.Hostname())
	if err != nil {
		return "", fmt.Errorf("pac: FindProxyForURL failed: %v", err)
	}
	if v.IsUndefined() || v.IsNull() {
		return "", nil
	}
	return v.ToString()
}

// FirstEntry returns the first entry of a PAC answer as a proxy "host:port", or ""
// for a direct connection. Only PROXY entries are supported; empty, DIRECT,
// malformed and unsupported entries (SOCKS, HTTPS, ...) mean direct, and ok is
// false for the malformed or unsupported cases so callers can log them.
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
