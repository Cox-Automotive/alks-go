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

// Sam's review: the test must fail if a response body is logged. A session
// response carries fresh credentials, so check CreateSession for every
// credential type.
func TestNoSessionCredentialsInLogs(t *testing.T) {
	buf := captureLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "loginRoles") {
			fmt.Fprint(w, `{"statusMessage":"Success","loginRole":{"account":"012345678910/ALKSAdmin - awstest","role":"Admin","maxKeyDuration":2}}`)
			return
		}
		fmt.Fprint(w, `{"statusMessage":"Success","requestId":"r1","accessKey":"RESP-AKID-SECRET","secretKey":"RESP-SK-SECRET","sessionToken":"RESP-TOKEN-SECRET","sessionDuration":1}`)
	}))
	t.Cleanup(srv.Close)

	clients := []*Client{}
	if c, err := NewClient(srv.URL, "u", "p", "012345678910/ALKSAdmin - awstest", "Admin"); err == nil {
		clients = append(clients, c)
	}
	if c, err := NewSTSClient(srv.URL, "a", "b", "c"); err == nil {
		clients = append(clients, c)
	}
	if c, err := NewBearerTokenClient(srv.URL, "t", "012345678910/ALKSAdmin - awstest", "Admin"); err == nil {
		clients = append(clients, c)
	}
	for _, c := range clients {
		resp, err := c.CreateSession(1, false)
		if err != nil || resp.AccessKey != "RESP-AKID-SECRET" {
			t.Fatalf("CreateSession did not return the session: %+v %v", resp, err)
		}
	}
	for _, secret := range []string{"RESP-AKID-SECRET", "RESP-SK-SECRET", "RESP-TOKEN-SECRET"} {
		if strings.Contains(buf.String(), secret) {
			t.Errorf("log contains %q", secret)
		}
	}
}

// A redirect to another host must not carry the STS credential headers. Go
// strips Authorization itself, but copies custom headers.
func TestSTSCredentialsNotForwardedOnCrossHostRedirect(t *testing.T) {
	captureLog(t)
	var got http.Header
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"statusMessage":"Success"}`)
	}))
	t.Cleanup(other.Close)
	sameHostHits := 0
	var sameHostHeader string
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/loginRoles/id/me":
			http.Redirect(w, r, other.URL+"/elsewhere", http.StatusTemporaryRedirect)
		case "/rest/same-host":
			http.Redirect(w, r, "/rest/landed", http.StatusTemporaryRedirect)
		case "/rest/landed":
			sameHostHits++
			sameHostHeader = r.Header.Get(secretKeyHeader)
			fmt.Fprint(w, `{}`)
		}
	}))
	t.Cleanup(origin.Close)

	c, err := NewSTSClient(origin.URL+"/rest", "AKID-SECRET", "SK-SECRET", "TOKEN-SECRET")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("the redirect was not followed")
	}
	for _, h := range stsSecretHeaders {
		if v := got.Get(h); v != "" {
			t.Errorf("%s forwarded to another host: %q", h, v)
		}
	}

	// A redirect on the same host keeps the credentials.
	req, _ := c.NewRequest(nil, "GET", "/same-host")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if sameHostHits != 1 || sameHostHeader != "SK-SECRET" {
		t.Fatalf("same-host redirect lost credentials: hits=%d header=%q", sameHostHits, sameHostHeader)
	}
}

// A request that fails outright (no response at all) returns an error instead of
// crashing on a nil response.
func TestFailedRequestsDoNotPanic(t *testing.T) {
	captureLog(t)
	dead := "http://" + closedPort(t) + "/rest"
	c, err := NewClient(dead, "u", "p", "012345678910/ALKSAdmin - awstest", "Admin", WithProxyMode(ProxyNone))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panicked on a failed request: %v", r)
		}
	}()
	if _, aerr := c.CreateIamSession(); aerr == nil {
		t.Error("CreateIamSession: expected an error")
	}
	if _, aerr := c.GetLoginRole(); aerr == nil {
		t.Error("GetLoginRole: expected an error")
	}
	if _, aerr := c.IsIamEnabled(""); aerr == nil {
		t.Error("IsIamEnabled: expected an error")
	}
	sts, _ := NewSTSClient(dead, "a", "b", "c", WithProxyMode(ProxyNone))
	if _, aerr := sts.CreateSession(1, false); aerr == nil {
		t.Error("CreateSession: expected an error")
	}
}
