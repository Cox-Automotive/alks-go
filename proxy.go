package alks

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"golang.org/x/net/http/httpproxy"
)

// Decision sources, recorded in the debug log.
const (
	sourceExplicit     = "explicit"
	sourceEnv          = "env"
	sourcePAC          = "pac"
	sourceOSStatic     = "os-static"
	sourceDirect       = "direct"
	sourceNoneMode     = "none-mode"
	sourceEnvMode      = "env-mode"
	sourceCustomClient = "custom-client"
)

type proxyDecision struct {
	proxy    *url.URL
	source   string
	fallback string // discovery error, if the decision followed a failure
}

// newHTTPClient builds the client's *http.Client from its configuration.
func newHTTPClient(cfg resolvedConfig) *http.Client {
	if cfg.httpClient != nil {
		debugf("proxy mode=%s source=%s (caller-supplied *http.Client)", cfg.proxyMode, sourceCustomClient)
		return cfg.httpClient
	}
	t := cleanhttp.DefaultTransport()
	switch {
	case cfg.proxyURL != nil:
		debugf("proxy mode=%s source=%s proxy=%s", cfg.proxyMode, sourceExplicit, redactURL(cfg.proxyURL))
		t.Proxy = http.ProxyURL(cfg.proxyURL)
	case cfg.proxyMode == ProxyNone:
		debugf("proxy mode=%s source=%s proxy=direct", cfg.proxyMode, sourceNoneMode)
		t.Proxy = nil
	case cfg.proxyMode == ProxyEnvironment:
		debugf("proxy mode=%s source=%s (HTTPS_PROXY/HTTP_PROXY/NO_PROXY only)", cfg.proxyMode, sourceEnvMode)
		t.Proxy = http.ProxyFromEnvironment
	default:
		t.Proxy = newProxySelector(httpproxy.FromEnvironment(), newOSProxySource()).proxy
	}
	return &http.Client{Transport: t, CheckRedirect: keepCredentialsOnRedirect}
}

// stsSecretHeaders carry STS credentials (see credentials.go). Go removes the
// Authorization header on a redirect to another host, but copies custom headers,
// so these must be removed by hand.
var stsSecretHeaders = []string{accessKeyHeader, secretKeyHeader, sessionTokenHeader}

// keepCredentialsOnRedirect is the client's CheckRedirect. It follows redirects
// as Go does by default (up to 10), but never forwards the ALKS STS credential
// headers to a different host or port, or from https to http.
func keepCredentialsOnRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("alks: stopped after 10 redirects")
	}
	first := via[0].URL
	if strings.EqualFold(req.URL.Host, first.Host) && !(first.Scheme == "https" && req.URL.Scheme != "https") {
		return nil
	}
	removed := false
	for _, h := range stsSecretHeaders {
		if req.Header.Get(h) != "" {
			req.Header.Del(h)
			removed = true
		}
	}
	if removed {
		warnf("redirect from %s://%s to %s://%s: ALKS credentials were not forwarded", first.Scheme, first.Host, req.URL.Scheme, req.URL.Host)
	}
	return nil
}

// Cache lifetimes. Successful decisions and OS settings are re-checked after
// decisionTTL so that a long-running process picks up network changes (for
// example joining Zscaler after starting). Decisions that followed a discovery
// failure, and failed OS reads, are retried after failureTTL instead of being
// kept for the life of the client.
var (
	decisionTTL = 5 * time.Minute
	failureTTL  = 30 * time.Second
	now         = time.Now
)

type cachedDecision struct {
	d       proxyDecision
	expires time.Time
}

// proxySelector implements the auto mode. The OS settings are read lazily and
// re-read when they expire; decisions are cached per scheme, host and port.
type proxySelector struct {
	envFunc func(*url.URL) (*url.URL, error)
	noProxy func(*url.URL) (*url.URL, error) // nil result = host exempt by NO_PROXY
	src     osProxySource

	osMu      sync.Mutex
	osCfg     osProxyConfig
	osErr     error
	osExpires time.Time

	mu     sync.Mutex
	cache  map[string]cachedDecision
	logged map[string]proxyDecision
}

func newProxySelector(env *httpproxy.Config, src osProxySource) *proxySelector {
	probe := &httpproxy.Config{
		HTTPProxy:  "http://no-proxy-probe.invalid:1",
		HTTPSProxy: "http://no-proxy-probe.invalid:1",
		NoProxy:    env.NoProxy,
	}
	return &proxySelector{
		envFunc: env.ProxyFunc(),
		noProxy: probe.ProxyFunc(),
		src:     src,
		cache:   map[string]cachedDecision{},
		logged:  map[string]proxyDecision{},
	}
}

func (s *proxySelector) proxy(req *http.Request) (*url.URL, error) {
	return s.decide(req.URL).proxy, nil
}

func (s *proxySelector) decide(u *url.URL) proxyDecision {
	key := u.Scheme + "://" + u.Host
	s.mu.Lock()
	if c, ok := s.cache[key]; ok && now().Before(c.expires) {
		s.mu.Unlock()
		return c.d
	}
	s.mu.Unlock()

	d := s.compute(u)

	ttl := decisionTTL
	if d.fallback != "" {
		ttl = failureTTL
	}
	s.mu.Lock()
	s.cache[key] = cachedDecision{d: d, expires: now().Add(ttl)}
	prev, seen := s.logged[key]
	s.logged[key] = d
	s.mu.Unlock()

	// A discovery failure is logged every time it is (re)computed, at most once
	// per failureTTL per host, at warning level: it means the OS proxy could not
	// be used. A normal decision is logged at debug level when it is first made
	// or when it changes.
	switch {
	case d.fallback != "":
		warnf("OS proxy discovery failed for %s, connecting directly: %s", u.Host, d.fallback)
	case !seen || prev.source != d.source || redactURL(prev.proxy) != redactURL(d.proxy):
		debugf("proxy mode=auto host=%s source=%s proxy=%s", u.Host, d.source, redactURL(d.proxy))
	}
	return d
}

// osConfig returns the OS settings, re-reading them when they have expired.
func (s *proxySelector) osConfig() (osProxyConfig, error) {
	s.osMu.Lock()
	defer s.osMu.Unlock()
	if now().Before(s.osExpires) {
		return s.osCfg, s.osErr
	}
	s.osCfg, s.osErr = s.src.config()
	if s.osErr != nil {
		s.osExpires = now().Add(failureTTL)
	} else {
		s.osExpires = now().Add(decisionTTL)
	}
	return s.osCfg, s.osErr
}

func (s *proxySelector) compute(u *url.URL) proxyDecision {
	// 1. HTTPS_PROXY / HTTP_PROXY, filtered by NO_PROXY: exactly today's behaviour.
	if p, err := s.envFunc(u); err == nil && p != nil {
		return proxyDecision{proxy: p, source: sourceEnv}
	}

	// 2. Never proxy localhost or loopback, and honour NO_PROXY for OS proxies too.
	host := u.Hostname()
	if isLoopbackHost(host) {
		return proxyDecision{source: sourceDirect}
	}
	if p, err := s.noProxy(u); err == nil && p == nil {
		return proxyDecision{source: sourceDirect}
	}

	// 3. The OS configuration, read once.
	cfg, err := s.osConfig()
	if err != nil {
		return proxyDecision{source: sourceDirect, fallback: err.Error()}
	}

	// 4. PAC (or Windows automatic detection): the first entry of the answer wins.
	if cfg.hasPAC() {
		hostPort, err := s.src.pacProxyFor(cfg, u)
		if err != nil {
			return proxyDecision{source: sourceDirect, fallback: err.Error()}
		}
		if hostPort == "" {
			// With only automatic detection (no PAC URL), an empty answer cannot be
			// told apart from detection finding nothing, so use the static proxy if
			// there is one, as Windows and go-ieproxy do. An explicit PAC that says
			// DIRECT is honoured.
			if cfg.pacURL == "" && cfg.autoDetect && cfg.staticHTTPS != "" && !matchesException(host, cfg) {
				if p := parseHostPort(cfg.staticHTTPS); p != nil {
					return proxyDecision{proxy: p, source: sourceOSStatic}
				}
			}
			return proxyDecision{source: sourcePAC}
		}
		if p := parseHostPort(hostPort); p != nil {
			return proxyDecision{proxy: p, source: sourcePAC}
		}
		debugf("PAC returned an unusable proxy %q; connecting directly", hostPort)
		return proxyDecision{source: sourcePAC}
	}

	// 5. The OS static HTTPS proxy, unless the host is in its exception list.
	if cfg.staticHTTPS != "" && !matchesException(host, cfg) {
		if p := parseHostPort(cfg.staticHTTPS); p != nil {
			return proxyDecision{proxy: p, source: sourceOSStatic}
		}
	}

	// 6. Direct.
	return proxyDecision{source: sourceDirect}
}

// parseHostPort turns "host:port" (or "http://host:port") into a proxy URL.
func parseHostPort(hostPort string) *url.URL {
	if !strings.Contains(hostPort, "://") {
		hostPort = "http://" + hostPort
	}
	u, err := url.Parse(hostPort)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
