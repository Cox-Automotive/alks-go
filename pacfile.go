package alks

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Cox-Automotive/alks-go/internal/pac"
)

const (
	pacFetchLimit = 5 * time.Second
	pacEvalBudget = 2 * time.Second
	pacMaxSize    = 2 << 20 // 2 MiB
)

// pacFile fetches a PAC script once per URL and evaluates it in pure Go. It is
// used by the macOS source and by tests on every platform.
type pacFile struct {
	mu      sync.Mutex
	scripts map[string]cachedScript
	budget  time.Duration
}

type cachedScript struct {
	s       *pac.Script
	expires time.Time
}

func newPACFile() *pacFile {
	return &pacFile{scripts: map[string]cachedScript{}, budget: pacEvalBudget}
}

// proxyFor returns the proxy named by the first entry of the PAC answer for u, as
// "host:port", or "" for a direct connection.
func (p *pacFile) proxyFor(pacURL string, u *url.URL) (string, error) {
	script, err := p.script(pacURL)
	if err != nil {
		return "", err
	}
	answer, err := script.FindProxyForURL(u.String(), p.budget)
	if err != nil {
		return "", err
	}
	hostPort, ok := pac.FirstEntry(answer)
	if !ok {
		debugf("PAC answer %q for %s: first entry is not a PROXY entry, so connecting directly (only the first entry is used)", pac.Redact(answer), u.Host)
	}
	return hostPort, nil
}

func (p *pacFile) script(pacURL string) (*pac.Script, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if c, ok := p.scripts[pacURL]; ok && now().Before(c.expires) {
		return c.s, nil
	}
	src, err := fetchPAC(pacURL)
	if err != nil {
		return nil, err
	}
	s, err := pac.Compile(src, p.budget)
	if err != nil {
		return nil, err
	}
	// Re-fetch after decisionTTL so a changed PAC file is picked up. A failed
	// fetch or compile is not cached; the caller's failure decision is retried
	// after failureTTL.
	p.scripts[pacURL] = cachedScript{s: s, expires: now().Add(decisionTTL)}
	return s, nil
}

// fetchPAC downloads a PAC file directly, never through a proxy.
func fetchPAC(pacURL string) (string, error) {
	c := &http.Client{Timeout: pacFetchLimit, Transport: &http.Transport{Proxy: nil}}
	resp, err := c.Get(pacURL)
	if err != nil {
		return "", fmt.Errorf("fetching PAC file: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching PAC file: HTTP %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, pacMaxSize))
	if err != nil {
		return "", fmt.Errorf("reading PAC file: %v", err)
	}
	return string(b), nil
}
