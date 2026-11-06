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
	conf := ieproxy.GetConf()
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
	return cfg, nil
}

func (windowsOSProxy) pacProxyFor(_ osProxyConfig, u *url.URL) (string, error) {
	// An empty answer means DIRECT, or that automatic detection found nothing.
	auto := ieproxy.GetConf().Automatic
	hostPort := strings.TrimSpace(auto.FindProxyForURL(u.String()))
	hostPort = strings.TrimPrefix(strings.TrimPrefix(hostPort, "http://"), "PROXY ")
	return hostPort, nil
}
