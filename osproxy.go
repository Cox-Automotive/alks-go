package alks

import (
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/mattn/go-ieproxy"
	"golang.org/x/net/http/httpproxy"
)

// osProxyConfig is a snapshot of the operating system's proxy settings.
type osProxyConfig struct {
	pacURL                 string   // PAC location when PAC is enabled
	autoDetect             bool     // Windows automatic detection (WPAD)
	staticHTTPS            string   // static HTTPS proxy "host:port"
	exceptions             []string // static-proxy exception list
	excludeSimpleHostnames bool     // macOS: bypass hosts without a dot
}

func (c osProxyConfig) hasPAC() bool { return c.pacURL != "" || c.autoDetect }

// osProxySource reads the operating system's proxy configuration. Each platform
// has one implementation; tests replace it with a fake.
type osProxySource interface {
	// config returns the OS settings, or a discovery error.
	config() (osProxyConfig, error)
	// pacProxyFor returns the proxy named by the first entry of the PAC answer
	// for u as "host:port", or "" for a direct connection (DIRECT, no answer, or an
	// unsupported entry). An error is a discovery failure.
	pacProxyFor(cfg osProxyConfig, u *url.URL) (string, error)
}

// newOSProxySource returns the source for the current platform. Tests replace it.
var newOSProxySource = platformOSProxySource

// matchesException reports whether host matches an OS static-proxy exception.
// Exceptions are host names, "*.example.com" / ".example.com" suffixes, "*", IP
// addresses and CIDR ranges (including the macOS short form "169.254/16"), plus
// the Windows "<local>" token (hosts without a dot). Matching reuses Go's NO_PROXY
// rules from golang.org/x/net/http/httpproxy.
func matchesException(host string, cfg osProxyConfig) bool {
	h := strings.ToLower(host)
	simple := !strings.Contains(h, ".") && net.ParseIP(h) == nil
	if cfg.excludeSimpleHostnames && simple {
		return true
	}
	var noProxy []string
	for _, e := range cfg.exceptions {
		e = strings.ToLower(strings.TrimSpace(e))
		switch {
		case e == "":
		case e == "<local>":
			if simple {
				return true
			}
		case strings.HasPrefix(e, "*."):
			noProxy = append(noProxy, e[1:], e[2:]) // ".example.com" and the domain itself
		case strings.HasPrefix(e, "*") && len(e) > 1:
			noProxy = append(noProxy, e[1:])
		default:
			noProxy = append(noProxy, expandShortCIDR(e))
		}
	}
	if len(noProxy) == 0 {
		return false
	}
	probe := (&httpproxy.Config{
		HTTPSProxy: "http://exception-probe.invalid:1",
		HTTPProxy:  "http://exception-probe.invalid:1",
		NoProxy:    strings.Join(noProxy, ","),
	}).ProxyFunc()
	p, err := probe(&url.URL{Scheme: "https", Host: h})
	return err == nil && p == nil
}

// expandShortCIDR turns the macOS short form "169.254/16" into "169.254.0.0/16".
// Anything else is returned unchanged.
func expandShortCIDR(e string) string {
	addr, bits, ok := strings.Cut(e, "/")
	if !ok || net.ParseIP(addr) != nil {
		return e
	}
	parts := strings.Split(addr, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return e
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return e
		}
	}
	for len(parts) < 4 {
		parts = append(parts, "0")
	}
	return strings.Join(parts, ".") + "/" + bits
}

// normalizeWinHTTPProxy turns a WinHTTP proxy answer into "host:port". WinHTTP can
// answer "host:port", "http://host:port", "PROXY host:port" or the per-scheme form
// "http=host:port" / "https=host:port"; go-ieproxy has already cut the answer at
// the first ';'. "DIRECT" or an empty answer means a direct connection.
func normalizeWinHTTPProxy(answer string) string {
	a := strings.TrimSpace(answer)
	if strings.EqualFold(a, "DIRECT") {
		return ""
	}
	if i := strings.Index(a, "="); i > 0 && !strings.Contains(a[:i], "/") {
		// Per-scheme form. Only an http or https proxy can carry ALKS traffic;
		// socks=, ftp= and others are treated as no usable proxy.
		switch strings.ToLower(strings.TrimSpace(a[:i])) {
		case "http", "https":
			a = a[i+1:]
		default:
			return ""
		}
	}
	if len(a) >= 6 && strings.EqualFold(a[:6], "PROXY ") {
		a = a[6:]
	}
	a = strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(a), "http://"), "HTTP://")
	return strings.TrimSpace(a)
}

// configFromIEProxy maps go-ieproxy's view of the Windows settings to an
// osProxyConfig. It is a pure function so it is tested on every platform.
func configFromIEProxy(conf ieproxy.ProxyConf) osProxyConfig {
	cfg := osProxyConfig{}
	if conf.Automatic.Active {
		cfg.pacURL = conf.Automatic.PreConfiguredURL
		cfg.autoDetect = conf.Automatic.PreConfiguredURL == ""
	}
	if conf.Static.Active {
		p := conf.Static.Protocols["https"]
		if p == "" {
			p = conf.Static.Protocols[""]
		}
		cfg.staticHTTPS = strings.TrimSpace(p)
		for _, e := range strings.FieldsFunc(conf.Static.NoProxy, func(r rune) bool { return r == ',' || r == ';' }) {
			cfg.exceptions = append(cfg.exceptions, strings.TrimSpace(e))
		}
	}
	return cfg
}
