package alks

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Cox-Automotive/alks-go/internal/pac"
	cleanhttp "github.com/hashicorp/go-cleanhttp"
	"golang.org/x/net/http/httpproxy"
)

// pacSource serves a real PAC file over loopback and evaluates it with the same
// code the macOS source uses, so this runs on every platform.
type pacSource struct {
	pacURL string
	pf     *pac.File
}

// testPACFile builds the evaluator exactly as the macOS source's newPACFile does
// (same pacFileArgs), without the darwin build tag.
func testPACFile() *pac.File { return pac.NewFile(pacFileArgs()) }

func (p *pacSource) config() (osProxyConfig, error) { return osProxyConfig{pacURL: p.pacURL}, nil }

func (p *pacSource) pacProxyFor(cfg osProxyConfig, u *url.URL) (string, error) {
	return p.pf.ProxyFor(cfg.pacURL, u)
}

// recordingProxy is a forward proxy that records the requests it receives and
// answers them itself.
type recordingProxy struct {
	mu   sync.Mutex
	seen []string
	srv  *httptest.Server
}

func newRecordingProxy(t *testing.T) *recordingProxy {
	rp := &recordingProxy{}
	rp.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rp.mu.Lock()
		rp.seen = append(rp.seen, r.Host)
		rp.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"via":"proxy"}`)
	}))
	t.Cleanup(rp.srv.Close)
	return rp
}

func (rp *recordingProxy) hostPort() string { return strings.TrimPrefix(rp.srv.URL, "http://") }

func (rp *recordingProxy) count() int {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return len(rp.seen)
}

func closedPort(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}

func servePAC(t *testing.T, script string) string {
	u, _ := servePACCounted(t, script)
	return u
}

// servePACCounted is servePAC that also counts how often the file is fetched.
func servePACCounted(t *testing.T, script string) (string, *atomic.Int32) {
	n := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
		fmt.Fprint(w, script)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/proxy.pac", n
}

func clientFor(env httpproxy.Config, src osProxySource) *http.Client {
	tr := cleanhttp.DefaultTransport()
	tr.Proxy = newProxySelector(&env, src).proxy
	return &http.Client{Transport: tr}
}

// The ALKS host is a reserved .test name so that nothing can ever go direct.
const testALKS = "http://alks.example.test/rest/loginRoles/id/me"

func TestIntegrationPACRoutesThroughProxy(t *testing.T) {
	rp := newRecordingProxy(t)
	dead := closedPort(t)
	pacURL, fetches := servePACCounted(t, fmt.Sprintf(`function FindProxyForURL(url, host) {
		if (host == "alks.example.test") return "PROXY %s";
		if (host == "dead.example.test") return "PROXY %s; DIRECT";
		return "DIRECT";
	}`, rp.hostPort(), dead))
	src := &pacSource{pacURL: pacURL, pf: testPACFile()}
	c := clientFor(httpproxy.Config{}, src)

	// Proxied by the PAC.
	resp, err := c.Get(testALKS)
	if err != nil {
		t.Fatalf("request through PAC proxy failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(body), "proxy") || rp.count() != 1 {
		t.Fatalf("request did not go through the recording proxy (seen %d)", rp.count())
	}

	// First entry unreachable: fail, name the proxy, do not go direct.
	_, err = c.Get("http://dead.example.test/")
	if err == nil || !strings.Contains(err.Error(), dead) {
		t.Fatalf("want an error naming %s, got %v", dead, err)
	}

	// 1,000 requests to one host: the PAC is evaluated once (SC-007).
	for i := 0; i < 1000; i++ {
		r, err := c.Get(testALKS)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("PAC fetched %d times, want once", n)
	}
}

func TestIntegrationNoProxyAndFallback(t *testing.T) {
	rp := newRecordingProxy(t)
	pacURL := servePAC(t, fmt.Sprintf(`function FindProxyForURL(url, host) { return "PROXY %s"; }`, rp.hostPort()))

	// NO_PROXY exempts the host from the PAC's proxy; the request then fails to
	// resolve the reserved name, which proves it went direct.
	c := clientFor(httpproxy.Config{NoProxy: "alks.example.test"}, &pacSource{pacURL: pacURL, pf: testPACFile()})
	if _, err := c.Get(testALKS); err == nil || rp.count() != 0 {
		t.Fatalf("NO_PROXY not honoured (proxy saw %d)", rp.count())
	}

	// PAC server down: discovery failure falls back to direct, without a proxy error.
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	down := &pacSource{pacURL: "http://" + closedPort(t) + "/proxy.pac", pf: testPACFile()}
	_, err := clientFor(httpproxy.Config{}, down).Get(testALKS)
	if err != nil && strings.Contains(err.Error(), "proxyconnect") {
		t.Fatalf("fell back to a proxy instead of direct: %v", err)
	}
	if !strings.Contains(buf.String(), "OS proxy discovery failed") {
		t.Fatalf("fallback not logged: %q", buf.String())
	}
}

func TestIntegrationEnvironmentModeIgnoresPAC(t *testing.T) {
	restore := newOSProxySource
	defer func() { newOSProxySource = restore }()
	rp := newRecordingProxy(t)
	pacURL := servePAC(t, fmt.Sprintf(`function FindProxyForURL(url, host) { return "PROXY %s"; }`, rp.hostPort()))
	newOSProxySource = func() osProxySource { return &pacSource{pacURL: pacURL, pf: testPACFile()} }

	cfg, _ := buildConfig([]ClientOption{WithProxyMode(ProxyEnvironment)})
	newHTTPClient(cfg).Get(testALKS)
	if rp.count() != 0 {
		t.Fatal("environment mode used the OS PAC")
	}
}

// Sam's review: end to end, a real PAC script sees only scheme://host/ for an https
// request. The PAC returns the proxy only for exactly "https://alks.example.test/",
// so the request reaches the recording proxy (as a CONNECT) only if the path and
// query were removed before the PAC was asked. The tunnel itself then fails, which
// is fine: the recording proxy does not speak TLS.
func TestIntegrationPACSeesOnlySchemeAndHostForHTTPS(t *testing.T) {
	rp := newRecordingProxy(t)
	pacURL := servePAC(t, fmt.Sprintf(`function FindProxyForURL(url, host) {
		if (url == "https://alks.example.test/") return "PROXY %s";
		return "DIRECT";
	}`, rp.hostPort()))
	c := clientFor(httpproxy.Config{}, &pacSource{pacURL: pacURL, pf: testPACFile()})
	resp, err := c.Get("https://alks.example.test/rest/loginRoles/id/me?account=123#frag")
	if err == nil {
		resp.Body.Close()
	}
	if rp.count() != 1 {
		t.Fatalf("the PAC did not choose the proxy (proxy saw %d requests); it was given more than scheme and host", rp.count())
	}
}
