package alks

import (
	"net/url"
	"strings"

	"github.com/mattn/go-ieproxy"
)

// windowsOSProxy reads the Windows proxy settings (WinHTTP / Internet Options)
// through go-ieproxy, which is pure Go on Windows. WinHTTP evaluates the PAC and
// go-ieproxy keeps only the first entry of the answer.
type windowsOSProxy struct{}

func platformOSProxySource() osProxySource { return windowsOSProxy{} }

func (windowsOSProxy) config() (osProxyConfig, error) {
	return configFromIEProxy(ieproxy.GetConf()), nil
}

func (windowsOSProxy) pacProxyFor(_ osProxyConfig, u *url.URL) (string, error) {
	// An empty answer means DIRECT, or that automatic detection found nothing.
	auto := ieproxy.GetConf().Automatic
	hostPort := strings.TrimSpace(auto.FindProxyForURL(u.String()))
	return normalizeWinHTTPProxy(hostPort), nil
}
