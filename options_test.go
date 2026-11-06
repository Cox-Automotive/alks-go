package alks

import (
	"net/http"
	"strings"
	"testing"
)

func TestBuildConfigDefaults(t *testing.T) {
	c, err := buildConfig(nil)
	if err != nil {
		t.Fatal(err)
	}
	if c.proxyMode != ProxyAuto || c.userAgent != "alks-go" || c.proxyURL != nil || c.httpClient != nil {
		t.Fatalf("unexpected defaults: %+v", c)
	}
}

func TestBuildConfigOptions(t *testing.T) {
	hc := &http.Client{}
	c, err := buildConfig([]ClientOption{
		WithProxyMode(ProxyEnvironment),
		WithProxyURL("http://user:secret@127.0.0.1:9000"),
		WithUserAgent("my-tool/1.0"),
		WithHTTPClient(hc),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.proxyMode != ProxyEnvironment || c.userAgent != "my-tool/1.0" || c.httpClient != hc {
		t.Fatalf("options not applied: %+v", c)
	}
	if c.proxyURL == nil || c.proxyURL.Host != "127.0.0.1:9000" {
		t.Fatalf("proxy URL not parsed: %v", c.proxyURL)
	}
}

func TestWithUserAgentEmptyKeepsDefault(t *testing.T) {
	c, _ := buildConfig([]ClientOption{WithUserAgent("")})
	if c.userAgent != "alks-go" {
		t.Fatalf("got %q", c.userAgent)
	}
}

func TestBuildConfigErrors(t *testing.T) {
	cases := map[string][]ClientOption{
		"proxy URL with none mode": {WithProxyMode(ProxyNone), WithProxyURL("http://127.0.0.1:9000")},
		"unknown mode":             {WithProxyMode(ProxyMode(42))},
		"non-http scheme":          {WithProxyURL("socks5://127.0.0.1:1080")},
		"no host":                  {WithProxyURL("http://")},
		"nil http client":          {WithHTTPClient(nil)},
	}
	for name, opts := range cases {
		if _, err := buildConfig(opts); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestProxyURLErrorDoesNotLeakCredentials(t *testing.T) {
	_, err := buildConfig([]ClientOption{WithProxyURL("ftp://user:secret@host:1")})
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaks credentials or is missing: %v", err)
	}
}

func TestProxyModeString(t *testing.T) {
	for m, want := range map[ProxyMode]string{ProxyAuto: "auto", ProxyEnvironment: "environment", ProxyNone: "none"} {
		if m.String() != want {
			t.Errorf("%d: got %q", m, m.String())
		}
	}
}
