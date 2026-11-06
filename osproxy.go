package alks

import (
	"net/url"
	"strings"
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
// Exceptions are host names, "*.example.com" / ".example.com" suffixes, or "*".
func matchesException(host string, cfg osProxyConfig) bool {
	h := strings.ToLower(host)
	if cfg.excludeSimpleHostnames && !strings.Contains(h, ".") {
		return true
	}
	for _, e := range cfg.exceptions {
		e = strings.ToLower(strings.TrimSpace(e))
		switch {
		case e == "":
			continue
		case e == "*", e == "<local>" && !strings.Contains(h, "."):
			return true
		case strings.HasPrefix(e, "*."):
			if strings.HasSuffix(h, e[1:]) || h == e[2:] {
				return true
			}
		case strings.HasPrefix(e, "."):
			if strings.HasSuffix(h, e) {
				return true
			}
		case strings.HasPrefix(e, "*") && strings.HasSuffix(h, e[1:]):
			return true
		case h == e:
			return true
		}
	}
	return false
}
