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
	c, err := buildConfig([]ClientOption{
		WithProxyMode(ProxyEnvironment),
		WithProxyURL("http://user:secret@127.0.0.1:9000"),
		WithUserAgent("my-tool/1.0"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if c.proxyMode != ProxyEnvironment || c.userAgent != "my-tool/1.0" {
		t.Fatalf("options not applied: %+v", c)
	}
	hc := &http.Client{}
	c2, err := buildConfig([]ClientOption{WithHTTPClient(hc), WithUserAgent("ua")})
	if err != nil || c2.httpClient != hc {
		t.Fatalf("WithHTTPClient not applied: %+v %v", c2, err)
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
		"http client with mode":    {WithHTTPClient(&http.Client{}), WithProxyMode(ProxyNone)},
		"http client with URL":     {WithHTTPClient(&http.Client{}), WithProxyURL("http://127.0.0.1:9000")},
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

func TestUnparseableProxyURLErrorDoesNotLeakCredentials(t *testing.T) {
	for _, raw := range []string{
		"http://user:p@ss word@proxy:8080",
		"http://user:hunter2@proxy:notaport",
		"http://user:hunter2\x7f@proxy:8080",
	} {
		_, err := buildConfig([]ClientOption{WithProxyURL(raw)})
		if err == nil {
			t.Errorf("%q: expected an error", raw)
			continue
		}
		for _, secret := range []string{"p@ss word", "hunter2", "user:"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%q: error leaks %q: %v", raw, secret, err)
			}
		}
	}
}

// Sam's review: a value without "scheme://" parses as an opaque URL, and its
// credentials must not be echoed.
func TestSchemelessProxyURLDoesNotLeakCredentials(t *testing.T) {
	for _, raw := range []string{"user:S3cretPw@proxy.corp:8080", "S3cretPw@proxy.corp:8080", "proxy.corp:8080"} {
		_, err := buildConfig([]ClientOption{WithProxyURL(raw)})
		if err == nil {
			t.Errorf("%q: expected an error", raw)
			continue
		}
		if strings.Contains(err.Error(), "S3cretPw") || strings.Contains(err.Error(), "user:") {
			t.Errorf("%q: error leaks credentials: %v", raw, err)
		}
	}
	if got := redactRawURL("user:S3cretPw@proxy.corp:8080"); strings.Contains(got, "S3cretPw") {
		t.Errorf("redactRawURL leaks: %q", got)
	}
}
