package alks

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

// ProxyMode selects how the client chooses a proxy for ALKS requests.
type ProxyMode int

const (
	// ProxyAuto is the default. For each request it uses HTTPS_PROXY (filtered by
	// NO_PROXY) if set; otherwise the operating system's proxy configuration (the
	// PAC result for the request URL, then the OS static proxy) on macOS and
	// Windows; otherwise a direct connection. NO_PROXY also exempts hosts from an
	// OS-discovered proxy. On other platforms it behaves like ProxyEnvironment.
	ProxyAuto ProxyMode = iota

	// ProxyEnvironment uses only the standard HTTPS_PROXY, HTTP_PROXY and NO_PROXY
	// variables, which is how alks-go behaved before v1.1.0.
	ProxyEnvironment

	// ProxyNone always connects directly and ignores every proxy setting,
	// including HTTPS_PROXY.
	ProxyNone
)

// String returns "auto", "environment" or "none".
func (m ProxyMode) String() string {
	switch m {
	case ProxyAuto:
		return "auto"
	case ProxyEnvironment:
		return "environment"
	case ProxyNone:
		return "none"
	default:
		return fmt.Sprintf("ProxyMode(%d)", int(m))
	}
}

// ClientOption configures a Client. Pass options as trailing arguments to
// NewClient, NewSTSClient or NewBearerTokenClient. Options take effect before the
// constructor makes any request.
type ClientOption func(*clientConfig)

type clientConfig struct {
	proxyMode     ProxyMode
	proxyURLRaw   string
	proxyURLSet   bool
	userAgent     string
	httpClient    *http.Client
	httpClientSet bool
}

// WithProxyMode sets the proxy mode. The default is ProxyAuto.
func WithProxyMode(mode ProxyMode) ClientOption {
	return func(c *clientConfig) { c.proxyMode = mode }
}

// WithProxyURL sends every ALKS request through the given proxy (an http or https
// URL, for example "http://127.0.0.1:9000"). NO_PROXY does not apply to it. It
// cannot be combined with ProxyNone.
func WithProxyURL(proxyURL string) ClientOption {
	return func(c *clientConfig) {
		c.proxyURLRaw = proxyURL
		c.proxyURLSet = true
	}
}

// WithUserAgent sets the User-Agent for every request, including any request the
// constructor itself makes. An empty value keeps the default "alks-go".
func WithUserAgent(userAgent string) ClientOption {
	return func(c *clientConfig) {
		if userAgent != "" {
			c.userAgent = userAgent
		}
	}
}

// WithHTTPClient makes the client use the given *http.Client as is. Proxy options
// are then ignored and the supplied client's own proxy settings apply.
func WithHTTPClient(httpClient *http.Client) ClientOption {
	return func(c *clientConfig) {
		c.httpClient = httpClient
		c.httpClientSet = true
	}
}

// resolvedConfig is a validated clientConfig.
type resolvedConfig struct {
	proxyMode  ProxyMode
	proxyURL   *url.URL
	userAgent  string
	httpClient *http.Client
}

func buildConfig(opts []ClientOption) (resolvedConfig, error) {
	c := clientConfig{proxyMode: ProxyAuto, userAgent: "alks-go"}
	for _, opt := range opts {
		if opt != nil {
			opt(&c)
		}
	}

	r := resolvedConfig{proxyMode: c.proxyMode, userAgent: c.userAgent}

	switch c.proxyMode {
	case ProxyAuto, ProxyEnvironment, ProxyNone:
	default:
		return r, fmt.Errorf("alks: unknown proxy mode %d", int(c.proxyMode))
	}

	if c.httpClientSet {
		if c.httpClient == nil {
			return r, fmt.Errorf("alks: WithHTTPClient was given a nil *http.Client")
		}
		r.httpClient = c.httpClient
	}

	if c.proxyURLSet {
		if c.proxyMode == ProxyNone {
			return r, fmt.Errorf("alks: a proxy URL cannot be combined with ProxyNone")
		}
		u, err := url.Parse(c.proxyURLRaw)
		if err != nil {
			// *url.Error's text embeds the raw URL, credentials included; keep only
			// the underlying reason.
			var ue *url.Error
			if errors.As(err, &ue) {
				err = ue.Err
			}
			return r, fmt.Errorf("alks: invalid proxy URL %q: %v", redactRawURL(c.proxyURLRaw), err)
		}
		if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return r, fmt.Errorf("alks: proxy URL %q must be http:// or https:// with a host", redactRawURL(c.proxyURLRaw))
		}
		r.proxyURL = u
	}

	return r, nil
}
