//go:build !darwin && !windows

package alks

import "net/url"

// On Linux, FreeBSD and other platforms there is no single OS proxy facility, so
// nothing is reported and the selection reduces to the environment, then direct.
type noOSProxy struct{}

func platformOSProxySource() osProxySource { return noOSProxy{} }

func (noOSProxy) config() (osProxyConfig, error) { return osProxyConfig{}, nil }

func (noOSProxy) pacProxyFor(osProxyConfig, *url.URL) (string, error) { return "", nil }
