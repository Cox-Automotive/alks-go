package alks

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/net/http/httpproxy"
)

func loginRoleServer(t *testing.T, seenUA *[]string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seenUA != nil {
			*seenUA = append(*seenUA, r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"statusMessage":"Success","requestId":"r1","loginRole":{"account":"012345678910/ALKSAdmin - awstest","role":"Admin","iamKeyActive":true,"maxKeyDuration":2}}`)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func captureLog(t *testing.T) *bytes.Buffer {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(io.Discard) })
	return &buf
}

// US4: no credentials in logs, for every credential type (SC-004).
func TestNoCredentialsInLogs(t *testing.T) {
	buf := captureLog(t)
	srv := loginRoleServer(t, nil)
	secrets := []string{"s3cr3t-pass", "AKID-SECRET", "SK-SECRET", "TOKEN-SECRET", "BEARER-SECRET", "proxy-pass"}

	basic, err := NewClient(srv.URL, "dave", "s3cr3t-pass", "012345678910", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := basic.Durations(); err != nil {
		t.Fatal(err)
	}
	sts, err := NewSTSClient(srv.URL, "AKID-SECRET", "SK-SECRET", "TOKEN-SECRET")
	if err != nil {
		t.Fatal(err)
	}
	sts.Durations()
	bearer, err := NewBearerTokenClient(srv.URL, "BEARER-SECRET", "012345678910", "Admin")
	if err != nil {
		t.Fatal(err)
	}
	bearer.Durations()
	if _, err := NewClient(srv.URL, "u", "p", "a", "r", WithProxyURL("http://user:proxy-pass@127.0.0.1:1")); err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	for _, s := range secrets {
		if strings.Contains(out, s) {
			t.Errorf("log contains %q", s)
		}
	}
	for _, h := range []string{"Authorization", "ALKS-STS-Secret-Key", "ALKS-STS-Session-Token"} {
		if strings.Contains(out, h) {
			t.Errorf("log contains header %q", h)
		}
	}
	if !strings.Contains(out, "request GET") {
		t.Errorf("expected a redacted request line, got %q", out)
	}
}

// US4: one proxy-decision line per client (FR-019).
func TestDecisionLoggedOnce(t *testing.T) {
	buf := captureLog(t)
	s := newProxySelector(&httpproxy.Config{}, zscalerPAC())
	u, _ := url.Parse(alksURL)
	for i := 0; i < 3; i++ {
		s.proxy(&http.Request{URL: u})
	}
	if n := strings.Count(buf.String(), "proxy mode=auto"); n != 1 {
		t.Fatalf("decision logged %d times: %q", n, buf.String())
	}
	if !strings.Contains(buf.String(), "source=pac proxy=http://127.0.0.1:9000") {
		t.Fatalf("decision line missing details: %q", buf.String())
	}
}

// US3: options apply to the request the constructor itself makes (FR-014).
func TestUserAgentOnConstructorRequest(t *testing.T) {
	var seen []string
	srv := loginRoleServer(t, &seen)

	c, err := NewSTSClient(srv.URL, "a", "b", "c", WithUserAgent("alks-terraform-provider-2.9.0 terraform/1.7.5 darwin/arm64"))
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "alks-terraform-provider-2.9.0 terraform/1.7.5 darwin/arm64" {
		t.Fatalf("constructor request User-Agent = %v", seen)
	}
	if c.AccountDetails.Role != "Admin" {
		t.Fatalf("login role not applied: %+v", c.AccountDetails)
	}

	seen = nil
	if _, err := NewSTSClient(srv.URL, "a", "b", "c"); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0] != "alks-go" {
		t.Fatalf("default User-Agent = %v", seen)
	}
}

// US3: a caller-supplied client is used as is (FR-015), and SetUserAgent still works.
func TestWithHTTPClientAndSetUserAgent(t *testing.T) {
	var seen []string
	srv := loginRoleServer(t, &seen)
	hc := &http.Client{}
	c, err := NewClient(srv.URL, "u", "p", "012345678910", "Admin", WithHTTPClient(hc))
	if err != nil {
		t.Fatal(err)
	}
	if c.http != hc {
		t.Fatal("caller-supplied client not used")
	}
	c.SetUserAgent("later-ua")
	c.Durations()
	if len(seen) != 1 || seen[0] != "later-ua" {
		t.Fatalf("SetUserAgent not honoured: %v", seen)
	}
}

// US3: invalid options are reported by every constructor.
func TestConstructorsReturnOptionErrors(t *testing.T) {
	bad := []ClientOption{WithProxyMode(ProxyNone), WithProxyURL("http://127.0.0.1:1")}
	if _, err := NewClient("http://x", "u", "p", "a", "r", bad...); err == nil {
		t.Error("NewClient accepted invalid options")
	}
	if _, err := NewSTSClient("http://x", "a", "b", "c", bad...); err == nil {
		t.Error("NewSTSClient accepted invalid options")
	}
	if _, err := NewBearerTokenClient("http://x", "t", "a", "r", bad...); err == nil {
		t.Error("NewBearerTokenClient accepted invalid options")
	}
}
