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
	// BackoffAfterAbort is how long a PAC URL is left alone after an evaluation
	// was aborted (timeout or memory limit), so a hostile or broken script is not
	// re-run, and another goroutine abandoned, every few seconds.
	BackoffAfterAbort = 15 * time.Minute
)

// ErrBackoff is returned while a PAC URL is in backoff after an aborted evaluation.
var ErrBackoff = errors.New("pac: evaluation recently aborted for this PAC file; not retrying yet")

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
}

type cachedScript struct {
	s       *Script
	expires time.Time
}

// ProxyFor returns the proxy named by the first entry of the PAC answer for u, as
// "host:port", or "" for a direct connection.
func (f *File) ProxyFor(pacURL string, u *url.URL) (string, error) {
	if until, ok := f.inBackoff(pacURL); ok {
		f.logf("PAC file evaluation is in backoff until %s after an aborted run", until.Format(time.RFC3339))
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

// noteAbort puts pacURL into backoff after a timeout: this script stalled, and
// re-running it can leave another goroutine behind. Nothing else backs off:
//   - ErrBusy is about other, abandoned evaluations, not this script;
//   - a memory stop can be a false positive, because the heap is measured for
//     the whole process. A false stop should cost one failure TTL, not
//     BackoffAfterAbort, and a real memory bomb is stopped again within the
//     limit on every retry.
func (f *File) noteAbort(pacURL string, err error) {
	if !errors.Is(err, ErrTimeout) {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.backoff == nil {
		f.backoff = map[string]time.Time{}
	}
	f.backoff[pacURL] = f.now().Add(BackoffAfterAbort)
	delete(f.scripts, pacURL)
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
