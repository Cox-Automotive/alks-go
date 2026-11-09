package alks

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"time"
)

const (
	scutilTimeout = 5 * time.Second
	scutilCommand = "/usr/sbin/scutil"
)

// darwinOSProxy reads the macOS system proxy with scutil and evaluates the PAC in
// pure Go, so it works with CGO_ENABLED=0.
type darwinOSProxy struct {
	pac *pacFile
}

func platformOSProxySource() osProxySource { return &darwinOSProxy{pac: newPACFile()} }

func (d *darwinOSProxy) config() (osProxyConfig, error) {
	ctx, cancel := context.WithTimeout(context.Background(), scutilTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, scutilCommand, "--proxy").Output()
	if err != nil {
		return osProxyConfig{}, fmt.Errorf("reading macOS proxy settings: %v", err)
	}
	return parseScutilProxy(out), nil
}

func (d *darwinOSProxy) pacProxyFor(cfg osProxyConfig, u *url.URL) (string, error) {
	return d.pac.proxyFor(cfg.pacURL, u)
}
