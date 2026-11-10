package pac

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	// FetchLimit bounds the PAC download.
	FetchLimit = 5 * time.Second
	// FetchFailureTTL is how long a failed fetch or compile is remembered, so
	// lookups for many hosts do not each wait for an unreachable PAC server. The
	// last good script, if any, keeps being used meanwhile.
	FetchFailureTTL = 30 * time.Second
	// EvalBudget bounds each compile and each FindProxyForURL call.
	EvalBudget = 2 * time.Second
	// MaxSize is the largest PAC file accepted. Real PAC files are a few KB;
	// anything larger is rejected rather than truncated.
	MaxSize = 512 << 10 // 512 KiB
	// FirstBackoff is how long a PAC URL is left alone after its first timed-out
	// or abandoned evaluation. It matches the proxy selector's failure TTL, so one
	// slow evaluation (for example slow DNS on VPN) costs no more than before.
	FirstBackoff = 30 * time.Second
	// BackoffAfterAbort caps the backoff, which doubles with each consecutive
	// timed-out or abandoned evaluation of the same PAC URL (30s, 1m, 2m ... 15m),
	// so a hostile or broken script is not re-run, and another goroutine
	// abandoned, every few seconds. A successful evaluation resets it.
	BackoffAfterAbort = 15 * time.Minute
)

// ErrBackoff is returned while a PAC URL is in backoff after a timed-out or
// abandoned evaluation.
var ErrBackoff = errors.New("pac: evaluation recently timed out or was abandoned for this PAC file; not retrying yet")

// File fetches a PAC script once per URL, caches it, and evaluates it. It is
// safe for concurrent use.
type File struct {
	// TTL is how long a fetched script is reused before it is fetched again.
	TTL time.Duration
	// Budget bounds each evaluation; zero means EvalBudget.
	Budget time.Duration
	// Now returns the current time; nil means time.Now.
	Now func() time.Time
	// Logf receives debug lines; nil discards them.
	Logf func(format string, args ...interface{})
	// FetchTimeout bounds each download; zero means FetchLimit.
	FetchTimeout time.Duration

	// mu guards the maps below. It is never held during a fetch, compile or
	// evaluation, so a slow PAC server cannot block lookups for other URLs or
	// the backoff check.
	mu      sync.Mutex
	scripts map[string]*cachedScript
	backoff map[string]time.Time
	strikes map[string]int // consecutive timed-out or abandoned evaluations
}

// NewFile returns a File that reuses a fetched script for ttl, reads the time from
// now and logs through logf.
func NewFile(ttl time.Duration, now func() time.Time, logf func(string, ...interface{})) *File {
	return &File{TTL: ttl, Now: now, Logf: logf}
}

// cachedScript is the state for one PAC URL.
type cachedScript struct {
	s        *Script       // last good script (may be past expires: stale)
	expires  time.Time     // when s should be re-fetched
	inflight chan struct{} // non-nil while a fetch is running; closed when it ends
	err      error         // last fetch or compile error
	errUntil time.Time     // err is reused until then (FetchFailureTTL)
}

// ProxyFor returns the proxy named by the first entry of the PAC answer for u, as
// "host:port", or "" for a direct connection.
func (f *File) ProxyFor(pacURL string, u *url.URL) (string, error) {
	if until, ok := f.inBackoff(pacURL); ok {
		f.logf("PAC file evaluation is in backoff until %s after a timed-out or abandoned run", until.Format(time.RFC3339))
		return "", ErrBackoff
	}
	script, err := f.script(pacURL)
	if err != nil {
		f.noteAbort(pacURL, err)
		return "", err
	}
	answer, err := script.FindProxyForURL(u.String(), f.budget())
	if err != nil {
		f.noteAbort(pacURL, err)
		return "", err
	}
	f.clearStrikes(pacURL)
	hostPort, ok := FirstEntry(answer)
	if !ok {
		f.logf("PAC answer %q for %s: first entry is not a PROXY entry, so connecting directly (only the first entry is used)", Redact(answer), u.Host)
	}
	return hostPort, nil
}

// script returns the script for pacURL. One fetch runs at a time per URL, outside
// the lock; other callers wait for it, or get the last good script if there is one.
// A failed fetch or compile is remembered for FetchFailureTTL, during which the
// last good script (if any) is used and nothing is fetched.
func (f *File) script(pacURL string) (*Script, error) {
	f.mu.Lock()
	if f.scripts == nil {
		f.scripts = map[string]*cachedScript{}
	}
	c := f.scripts[pacURL]
	if c == nil {
		c = &cachedScript{}
		f.scripts[pacURL] = c
	}
	now := f.now()
	switch {
	case c.s != nil && now.Before(c.expires):
		s := c.s
		f.mu.Unlock()
		return s, nil
	case c.err != nil && now.Before(c.errUntil):
		s, err := c.s, c.err
		f.mu.Unlock()
		if s != nil {
			f.logf("PAC file refresh failed recently (%v); using the last good script", err)
			return s, nil
		}
		return nil, err
	case c.inflight != nil:
		if c.s != nil {
			s := c.s
			f.mu.Unlock()
			return s, nil // a refresh is running: use the last good script
		}
		ch := c.inflight
		f.mu.Unlock()
		<-ch
		f.mu.Lock()
		s, err := c.s, c.err
		f.mu.Unlock()
		if s == nil && err == nil {
			err = errors.New("pac: PAC file was dropped while fetching")
		}
		if s != nil {
			return s, nil
		}
		return nil, err
	}
	ch := make(chan struct{})
	c.inflight = ch
	f.mu.Unlock()

	src, err := fetch(pacURL, f.fetchTimeout())
	var s *Script
	if err == nil {
		s, err = Compile(src, f.budget())
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	defer close(ch)
	c.inflight = nil
	if err != nil {
		if isAbort(err) {
			// A stopped compile is for the backoff rules (ProxyFor's noteAbort),
			// not the fetch-failure cache.
			return nil, err
		}
		c.err, c.errUntil = err, f.now().Add(FetchFailureTTL)
		if c.s != nil {
			f.logf("PAC file refresh failed (%v); using the last good script", err)
			return c.s, nil
		}
		return nil, err
	}
	// Re-fetch after TTL so a changed PAC file is picked up.
	c.s, c.expires, c.err = s, f.now().Add(f.TTL), nil
	return s, nil
}

// isAbort reports whether err stopped an evaluation (rather than a fetch or a
// script error): those are handled by the backoff, not the failure cache.
func isAbort(err error) bool {
	return errors.Is(err, ErrTimeout) || errors.Is(err, ErrMemory) || errors.Is(err, ErrBusy)
}

func (f *File) inBackoff(pacURL string) (time.Time, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	until, ok := f.backoff[pacURL]
	if ok && f.now().Before(until) {
		return until, true
	}
	delete(f.backoff, pacURL)
	return time.Time{}, false
}

// noteAbort puts pacURL into backoff after a timeout, or after any stop that left
// a goroutine behind (ErrAbandoned, for example a memory stop inside a builtin
// that ignores the interrupt). The backoff starts at FirstBackoff and doubles with
// each consecutive strike, up to BackoffAfterAbort. Nothing else backs off:
//   - ErrBusy is about other, abandoned evaluations, not this script;
//   - a memory stop at which the VM did stop can be a false positive, because the
//     heap is measured for the whole process, so it costs one failure TTL.
func (f *File) noteAbort(pacURL string, err error) {
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, ErrAbandoned) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.backoff == nil {
		f.backoff = map[string]time.Time{}
		f.strikes = map[string]int{}
	}
	f.strikes[pacURL]++
	f.backoff[pacURL] = f.now().Add(backoffFor(f.strikes[pacURL]))
	if c := f.scripts[pacURL]; c != nil {
		c.s, c.err = nil, nil // drop the script; it is fetched again after the backoff
	}
}

// backoffFor is FirstBackoff doubled for each strike after the first, capped at
// BackoffAfterAbort.
func backoffFor(strikes int) time.Duration {
	d := FirstBackoff
	for i := 1; i < strikes && d < BackoffAfterAbort; i++ {
		d *= 2
	}
	if d > BackoffAfterAbort {
		d = BackoffAfterAbort
	}
	return d
}

func (f *File) clearStrikes(pacURL string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.strikes, pacURL)
}

func (f *File) fetchTimeout() time.Duration {
	if f.FetchTimeout > 0 {
		return f.FetchTimeout
	}
	return FetchLimit
}

func (f *File) budget() time.Duration {
	if f.Budget > 0 {
		return f.Budget
	}
	return EvalBudget
}

func (f *File) now() time.Time {
	if f.Now != nil {
		return f.Now()
	}
	return time.Now()
}

func (f *File) logf(format string, args ...interface{}) {
	if f.Logf != nil {
		f.Logf(format, args...)
	}
}

// Fetch downloads a PAC file directly, never through a proxy, within FetchLimit.
// A file larger than MaxSize is an error.
func Fetch(pacURL string) (string, error) { return fetch(pacURL, FetchLimit) }

func fetch(pacURL string, timeout time.Duration) (string, error) {
	// One request per client, so keep-alives would only leave idle connections
	// (and their goroutines) behind.
	c := &http.Client{Timeout: timeout, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	resp, err := c.Get(pacURL)
	if err != nil {
		return "", fmt.Errorf("fetching PAC file: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching PAC file: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, MaxSize+1))
	if err != nil {
		return "", fmt.Errorf("reading PAC file: %v", err)
	}
	if len(b) > MaxSize {
		return "", fmt.Errorf("PAC file is larger than %d KiB", MaxSize>>10)
	}
	return string(b), nil
}
