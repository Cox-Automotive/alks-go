//go:build !darwin && !windows

package alks

import "testing"

// On Linux, FreeBSD and other platforms, auto mode reads no OS settings and so
// behaves like the environment-only mode (FR-009, problem spec R13).
func TestOtherPlatformsReportNothing(t *testing.T) {
	cfg, err := platformOSProxySource().config()
	if err != nil || cfg.hasPAC() || cfg.staticHTTPS != "" {
		t.Fatalf("expected an empty configuration, got %+v, %v", cfg, err)
	}
}
