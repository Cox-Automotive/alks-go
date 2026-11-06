package alks

import (
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"

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
	return &http.Client{Transport: t}
}

// proxySelector implements the auto mode. The OS settings are read lazily, at
// most once, and decisions are cached per scheme, host and port.
type proxySelector struct {
	envFunc func(*url.URL) (*url.URL, error)
	noProxy func(*url.URL) (*url.URL, error) // nil result = host exempt by NO_PROXY
	src     osProxySource

	osOnce sync.Once
	osCfg  osProxyConfig
	osErr  error

	mu      sync.Mutex
	cache   map[string]proxyDecision
	logOnce sync.Once
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
		cache:   map[string]proxyDecision{},
	}
}

func (s *proxySelector) proxy(req *http.Request) (*url.URL, error) {
	d := s.decide(req.URL)
	s.logOnce.Do(func() {
		if d.fallback != "" {
			debugf("proxy mode=auto source=%s proxy=%s (OS proxy discovery failed: %s)", d.source, redactURL(d.proxy), d.fallback)
			return
		}
		debugf("proxy mode=auto source=%s proxy=%s", d.source, redactURL(d.proxy))
	})
	return d.proxy, nil
}

func (s *proxySelector) decide(u *url.URL) proxyDecision {
	key := u.Scheme + "://" + u.Host
	s.mu.Lock()
	if d, ok := s.cache[key]; ok {
		s.mu.Unlock()
		return d
	}
	s.mu.Unlock()

	d := s.compute(u)

	s.mu.Lock()
	s.cache[key] = d
	s.mu.Unlock()
	return d
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
	s.osOnce.Do(func() { s.osCfg, s.osErr = s.src.config() })
	if s.osErr != nil {
		return proxyDecision{source: sourceDirect, fallback: s.osErr.Error()}
	}
	cfg := s.osCfg

	// 4. PAC (or Windows automatic detection): the first entry of the answer wins.
	if cfg.hasPAC() {
		hostPort, err := s.src.pacProxyFor(cfg, u)
		if err != nil {
			return proxyDecision{source: sourceDirect, fallback: err.Error()}
		}
		if hostPort == "" {
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
