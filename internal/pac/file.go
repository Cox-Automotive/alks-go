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

	mu      sync.Mutex
	scripts map[string]cachedScript
	backoff map[string]time.Time
	strikes map[string]int // consecutive timed-out or abandoned evaluations
}

// NewFile returns a File that reuses a fetched script for ttl, reads the time from
// now and logs through logf.
func NewFile(ttl time.Duration, now func() time.Time, logf func(string, ...interface{})) *File {
	return &File{TTL: ttl, Now: now, Logf: logf}
}

type cachedScript struct {
	s       *Script
	expires time.Time
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

func (f *File) script(pacURL string) (*Script, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.scripts[pacURL]; ok && f.now().Before(c.expires) {
		return c.s, nil
	}
	src, err := Fetch(pacURL)
	if err != nil {
		return nil, err
	}
	s, err := Compile(src, f.budget())
	if err != nil {
		return nil, err
	}
	// Re-fetch after TTL so a changed PAC file is picked up. A failed fetch or
	// compile is not cached.
	if f.scripts == nil {
		f.scripts = map[string]cachedScript{}
	}
	f.scripts[pacURL] = cachedScript{s: s, expires: f.now().Add(f.TTL)}
	return s, nil
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
	delete(f.scripts, pacURL)
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

// Fetch downloads a PAC file directly, never through a proxy. A file larger
// than MaxSize is an error.
func Fetch(pacURL string) (string, error) {
	c := &http.Client{Timeout: FetchLimit, Transport: &http.Transport{Proxy: nil}}
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
