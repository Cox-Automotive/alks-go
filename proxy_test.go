package alks

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mattn/go-ieproxy"
	"golang.org/x/net/http/httpproxy"
)

// fakeOS is an osProxySource for tests: no real OS settings are read.
type fakeOS struct {
	cfg       osProxyConfig
	cfgErr    error
	pacAnswer map[string]string // host -> "host:port" or "" (direct)
	pacErr    error
	pacCalls  int
	cfgCalls  int
}

func (f *fakeOS) config() (osProxyConfig, error) { f.cfgCalls++; return f.cfg, f.cfgErr }

func (f *fakeOS) pacProxyFor(_ osProxyConfig, u *url.URL) (string, error) {
	f.pacCalls++
	if f.pacErr != nil {
		return "", f.pacErr
	}
	return f.pacAnswer[u.Hostname()], nil
}

const alksURL = "https://alks.coxautoinc.com/rest/loginRoles/id/me"

func decideFor(t *testing.T, env httpproxy.Config, os *fakeOS, raw string) proxyDecision {
	t.Helper()
	u, _ := url.Parse(raw)
	return newProxySelector(&env, os).decide(u)
}

func proxyHost(d proxyDecision) string {
	if d.proxy == nil {
		return "direct"
	}
	return d.proxy.Host
}

func zscalerPAC() *fakeOS {
	return &fakeOS{
		cfg:       osProxyConfig{pacURL: "http://127.0.0.1:9000/x.pac"},
		pacAnswer: map[string]string{"alks.coxautoinc.com": "127.0.0.1:9000", "login.microsoftonline.com": ""},
	}
}

// US1: the OS proxy is followed.
func TestAutoUsesPAC(t *testing.T) {
	d := decideFor(t, httpproxy.Config{}, zscalerPAC(), alksURL)
	if proxyHost(d) != "127.0.0.1:9000" || d.source != sourcePAC {
		t.Fatalf("got %s / %s", proxyHost(d), d.source)
	}
}

func TestAutoPACPerHost(t *testing.T) {
	os := zscalerPAC()
	if d := decideFor(t, httpproxy.Config{}, os, "https://login.microsoftonline.com/x"); d.proxy != nil {
		t.Fatalf("PAC DIRECT host was proxied via %s", proxyHost(d))
	}
}

func TestAutoStaticProxyAndExceptions(t *testing.T) {
	os := &fakeOS{cfg: osProxyConfig{staticHTTPS: "proxy.example.com:8443", exceptions: []string{"*.internal.example.com"}}}
	if d := decideFor(t, httpproxy.Config{}, os, alksURL); proxyHost(d) != "proxy.example.com:8443" || d.source != sourceOSStatic {
		t.Fatalf("static proxy not used: %s / %s", proxyHost(d), d.source)
	}
	if d := decideFor(t, httpproxy.Config{}, os, "https://a.internal.example.com/"); d.proxy != nil {
		t.Fatal("exception host was proxied")
	}
	simple := &fakeOS{cfg: osProxyConfig{staticHTTPS: "proxy.example.com:8443", excludeSimpleHostnames: true}}
	if d := decideFor(t, httpproxy.Config{}, simple, "https://intranet/"); d.proxy != nil {
		t.Fatal("simple hostname was proxied")
	}
}

func TestAutoPACWinsOverStatic(t *testing.T) {
	os := zscalerPAC()
	os.cfg.staticHTTPS = "static.example.com:1"
	if d := decideFor(t, httpproxy.Config{}, os, "https://login.microsoftonline.com/"); d.proxy != nil {
		t.Fatal("PAC DIRECT was overridden by the static proxy")
	}
}

func TestAutoNothingConfiguredIsDirect(t *testing.T) {
	if d := decideFor(t, httpproxy.Config{}, &fakeOS{}, alksURL); d.proxy != nil || d.source != sourceDirect {
		t.Fatalf("got %s / %s", proxyHost(d), d.source)
	}
}

// US2: existing behaviour and explicit settings still win.
func TestEnvironmentWinsOverOS(t *testing.T) {
	d := decideFor(t, httpproxy.Config{HTTPSProxy: "http://env-proxy:3128"}, zscalerPAC(), alksURL)
	if proxyHost(d) != "env-proxy:3128" || d.source != sourceEnv {
		t.Fatalf("got %s / %s", proxyHost(d), d.source)
	}
}

func TestNoProxyExemptsOSProxy(t *testing.T) {
	for _, np := range []string{"alks.coxautoinc.com", "coxautoinc.com", ".coxautoinc.com", "*"} {
		os := zscalerPAC()
		d := decideFor(t, httpproxy.Config{NoProxy: np}, os, alksURL)
		if d.proxy != nil {
			t.Errorf("NO_PROXY=%q: still proxied via %s", np, proxyHost(d))
		}
		if os.pacCalls != 0 {
			t.Errorf("NO_PROXY=%q: PAC was evaluated", np)
		}
	}
	if d := decideFor(t, httpproxy.Config{NoProxy: ".other.com"}, zscalerPAC(), alksURL); d.proxy == nil {
		t.Error("unrelated NO_PROXY entry exempted the ALKS host")
	}
	if d := decideFor(t, httpproxy.Config{NoProxy: "10.0.0.0/8"}, zscalerPAC(), "https://10.1.2.3/"); d.proxy != nil {
		t.Error("CIDR NO_PROXY entry not honoured")
	}
}

func TestLoopbackNeverProxied(t *testing.T) {
	os := &fakeOS{cfg: osProxyConfig{staticHTTPS: "proxy.example.com:8443"}}
	for _, raw := range []string{"http://localhost:4200/x", "http://127.0.0.1:8080/", "http://[::1]:1/"} {
		if d := decideFor(t, httpproxy.Config{}, os, raw); d.proxy != nil {
			t.Errorf("%s was proxied", raw)
		}
	}
	if os.cfgCalls != 0 {
		t.Error("the OS settings were read for a loopback request")
	}
}

func TestDiscoveryFailuresFallBack(t *testing.T) {
	cfgFail := &fakeOS{cfgErr: errors.New("scutil failed")}
	if d := decideFor(t, httpproxy.Config{}, cfgFail, alksURL); d.proxy != nil || d.fallback == "" {
		t.Fatalf("config failure: %s / fallback %q", proxyHost(d), d.fallback)
	}
	pacFail := zscalerPAC()
	pacFail.pacErr = errors.New("PAC server down")
	if d := decideFor(t, httpproxy.Config{}, pacFail, alksURL); d.proxy != nil || d.fallback == "" {
		t.Fatalf("PAC failure: %s / fallback %q", proxyHost(d), d.fallback)
	}
	// The environment still applies when OS discovery would fail.
	if d := decideFor(t, httpproxy.Config{HTTPSProxy: "http://env:1"}, cfgFail, alksURL); proxyHost(d) != "env:1" {
		t.Fatalf("env not used: %s", proxyHost(d))
	}
}

func TestDecisionsAreCached(t *testing.T) {
	os := zscalerPAC()
	s := newProxySelector(&httpproxy.Config{}, os)
	u, _ := url.Parse(alksURL)
	for i := 0; i < 1000; i++ {
		s.decide(u)
	}
	if os.pacCalls != 1 || os.cfgCalls != 1 {
		t.Fatalf("PAC evaluated %d times, config read %d times; want 1 and 1", os.pacCalls, os.cfgCalls)
	}
}

func TestModesOtherThanAuto(t *testing.T) {
	restore := newOSProxySource
	defer func() { newOSProxySource = restore }()
	called := false
	newOSProxySource = func() osProxySource { called = true; return zscalerPAC() }

	for _, opts := range [][]ClientOption{
		{WithProxyMode(ProxyEnvironment)},
		{WithProxyMode(ProxyNone)},
		{WithProxyURL("http://127.0.0.1:9000")},
	} {
		cfg, err := buildConfig(opts)
		if err != nil {
			t.Fatal(err)
		}
		newHTTPClient(cfg)
	}
	if called {
		t.Fatal("OS discovery was set up outside auto mode")
	}
}

// Windows: automatic detection with no answer falls through to the static proxy;
// an explicit PAC that says DIRECT does not.
func TestAutoDetectEmptyFallsThroughToStatic(t *testing.T) {
	detect := &fakeOS{cfg: osProxyConfig{autoDetect: true, staticHTTPS: "manual.example.com:8080"}, pacAnswer: map[string]string{}}
	if d := decideFor(t, httpproxy.Config{}, detect, alksURL); proxyHost(d) != "manual.example.com:8080" || d.source != sourceOSStatic {
		t.Fatalf("auto-detect empty: got %s / %s", proxyHost(d), d.source)
	}
	detectExc := &fakeOS{cfg: osProxyConfig{autoDetect: true, staticHTTPS: "manual.example.com:8080", exceptions: []string{"*.coxautoinc.com"}}, pacAnswer: map[string]string{}}
	if d := decideFor(t, httpproxy.Config{}, detectExc, alksURL); d.proxy != nil {
		t.Fatal("exception host was proxied after empty auto-detect")
	}
	explicit := &fakeOS{cfg: osProxyConfig{pacURL: "http://pac/x.pac", staticHTTPS: "manual.example.com:8080"}, pacAnswer: map[string]string{}}
	if d := decideFor(t, httpproxy.Config{}, explicit, alksURL); d.proxy != nil {
		t.Fatal("explicit PAC DIRECT was overridden by the static proxy")
	}
}

func TestNormalizeWinHTTPProxy(t *testing.T) {
	for in, want := range map[string]string{
		"127.0.0.1:9000":         "127.0.0.1:9000",
		"http://127.0.0.1:9000":  "127.0.0.1:9000",
		"PROXY 127.0.0.1:9000":   "127.0.0.1:9000",
		"http=127.0.0.1:9000":    "127.0.0.1:9000",
		"https=proxy.example:80": "proxy.example:80",
		"DIRECT":                 "",
		"":                       "",
		"proxy 127.0.0.1:9000":   "127.0.0.1:9000",
		"socks=127.0.0.1:1080":   "",
		"ftp=127.0.0.1:21":       "",
		"HTTPS=proxy.example:80": "proxy.example:80",
	} {
		if got := normalizeWinHTTPProxy(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

// Sam's review: failures are retried, successes expire, failures are warned.
func TestFailuresAreRetriedAndSuccessesExpire(t *testing.T) {
	restore := now
	defer func() { now = restore }()
	clock := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	now = func() time.Time { return clock }

	os := &fakeOS{cfgErr: errors.New("scutil failed")}
	s := newProxySelector(&httpproxy.Config{}, os)
	u, _ := url.Parse(alksURL)

	if d := s.decide(u); d.fallback == "" || d.proxy != nil {
		t.Fatalf("expected a fallback decision, got %+v", d)
	}
	s.decide(u) // cached within failureTTL
	if os.cfgCalls != 1 {
		t.Fatalf("config read %d times within failureTTL", os.cfgCalls)
	}

	// The OS proxy becomes available (for example, Zscaler starts).
	os.cfgErr = nil
	os.cfg = osProxyConfig{pacURL: "http://127.0.0.1:9000/x.pac"}
	os.pacAnswer = map[string]string{"alks.coxautoinc.com": "127.0.0.1:9000"}
	clock = clock.Add(failureTTL + time.Second)
	if d := s.decide(u); proxyHost(d) != "127.0.0.1:9000" {
		t.Fatalf("did not recover after failureTTL: %+v", d)
	}

	// A successful decision is kept within decisionTTL and re-checked after it.
	calls := os.pacCalls
	clock = clock.Add(decisionTTL - time.Second)
	s.decide(u)
	if os.pacCalls != calls {
		t.Fatal("successful decision re-evaluated within decisionTTL")
	}
	clock = clock.Add(2 * time.Second)
	s.decide(u)
	if os.pacCalls != calls+1 {
		t.Fatal("successful decision not re-evaluated after decisionTTL")
	}
}

func TestDiscoveryFailureLoggedAsWarning(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(io.Discard)
	s := newProxySelector(&httpproxy.Config{}, &fakeOS{cfgErr: errors.New("scutil failed")})
	u, _ := url.Parse(alksURL)
	s.decide(u)
	if !strings.Contains(buf.String(), "[WARN] alks-go: OS proxy discovery failed for alks.coxautoinc.com") {
		t.Fatalf("no warning: %q", buf.String())
	}
}

// Sam's review: OS exception lists can hold IP and CIDR entries.
func TestExceptionsMatchIPsAndCIDRs(t *testing.T) {
	cfg := osProxyConfig{exceptions: []string{"169.254/16", "10.0.0.0/8", "192.168.1.5", "*.local"}}
	for host, want := range map[string]bool{
		"169.254.169.254":     true,
		"10.1.2.3":            true,
		"192.168.1.5":         true,
		"printer.local":       true,
		"11.0.0.1":            false,
		"alks.coxautoinc.com": false,
	} {
		if got := matchesException(host, cfg); got != want {
			t.Errorf("%s: got %v want %v", host, got, want)
		}
	}
}

func TestConfigFromIEProxy(t *testing.T) {
	var pacOnly, detect, static ieproxy.ProxyConf
	pacOnly.Automatic.Active, pacOnly.Automatic.PreConfiguredURL = true, "http://127.0.0.1:9000/x.pac"
	detect.Automatic.Active = true
	static.Static.Active = true
	static.Static.Protocols = map[string]string{"https": "proxy.example:8443", "http": "proxy.example:8080"}
	static.Static.NoProxy = "*.corp;<local>,10.0.0.0/8"

	if c := configFromIEProxy(pacOnly); c.pacURL != "http://127.0.0.1:9000/x.pac" || c.autoDetect {
		t.Errorf("PAC: %+v", c)
	}
	if c := configFromIEProxy(detect); !c.autoDetect || c.pacURL != "" {
		t.Errorf("auto-detect: %+v", c)
	}
	c := configFromIEProxy(static)
	if c.staticHTTPS != "proxy.example:8443" || len(c.exceptions) != 3 {
		t.Errorf("static: %+v", c)
	}
	var all ieproxy.ProxyConf
	all.Static.Active = true
	all.Static.Protocols = map[string]string{"": "single.example:3128"}
	if c := configFromIEProxy(all); c.staticHTTPS != "single.example:3128" {
		t.Errorf("single proxy for all protocols: %+v", c)
	}
}

// Cody's review sec-3: for https, the PAC sees only scheme://host[:port]/.
func TestPACTargetStripsHTTPSPathAndQuery(t *testing.T) {
	for in, want := range map[string]string{
		"https://alks.example.test/rest/loginRoles/id/me?account=123": "https://alks.example.test/",
		"https://alks.example.test:8443/x#frag":                       "https://alks.example.test:8443/",
		"http://intranet/path?q=1":                                    "http://intranet/path?q=1",
		"http://user:secret@intranet/path#frag":                       "http://intranet/path",
		"http://user@intranet:8080/":                                  "http://intranet:8080/",
	} {
		u, err := url.Parse(in)
		if err != nil {
			t.Fatal(err)
		}
		if got := pacTarget(u).String(); got != want {
			t.Errorf("pacTarget(%q) = %q, want %q", in, got, want)
		}
	}
}

// The selector passes the stripped URL to the OS source.
func TestSelectorPassesStrippedURLToPAC(t *testing.T) {
	src := &recordingPACSource{}
	sel := newProxySelector(&httpproxy.Config{}, src)
	req, _ := http.NewRequest("GET", "https://alks.example.test/rest/getAccounts?secret=1", nil)
	if _, err := sel.proxy(req); err != nil {
		t.Fatal(err)
	}
	if src.seen != "https://alks.example.test/" {
		t.Fatalf("PAC asked about %q, want only scheme and host", src.seen)
	}
}

type recordingPACSource struct{ seen string }

func (r *recordingPACSource) config() (osProxyConfig, error) {
	return osProxyConfig{pacURL: "http://pac.example.test/proxy.pac"}, nil
}

func (r *recordingPACSource) pacProxyFor(_ osProxyConfig, u *url.URL) (string, error) {
	r.seen = u.String()
	return "", nil
}

// slowCountingSource answers the PAC lookup after a delay and counts lookups.
type slowCountingSource struct {
	mu    sync.Mutex
	calls int
	delay time.Duration
}

func (s *slowCountingSource) config() (osProxyConfig, error) {
	return osProxyConfig{pacURL: "http://pac.example.test/proxy.pac"}, nil
}

func (s *slowCountingSource) pacProxyFor(osProxyConfig, *url.URL) (string, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	time.Sleep(s.delay)
	return "127.0.0.1:9000", nil
}

// Cody's review: concurrent first lookups for one host share one decision.
func TestConcurrentFirstLookupsShareOneDecision(t *testing.T) {
	src := &slowCountingSource{delay: 100 * time.Millisecond}
	sel := newProxySelector(&httpproxy.Config{}, src)
	u, _ := url.Parse("https://alks.example.test/rest/x")
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if d := sel.decide(u); d.proxy == nil || d.proxy.Host != "127.0.0.1:9000" {
				t.Errorf("decision %+v, want the PAC proxy", d)
			}
		}()
	}
	wg.Wait()
	if src.calls != 1 {
		t.Fatalf("PAC consulted %d times for one host, want 1", src.calls)
	}
}

// Cody's review: a fallback decision never overwrites a fresher successful one.
func TestFallbackDoesNotOverwriteGoodDecision(t *testing.T) {
	sel := newProxySelector(&httpproxy.Config{}, &slowCountingSource{})
	key := "https://alks.example.test"
	good := proxyDecision{proxy: &url.URL{Scheme: "http", Host: "127.0.0.1:9000"}, source: sourcePAC}
	sel.mu.Lock()
	defer sel.mu.Unlock()
	sel.store(key, good)
	got := sel.store(key, proxyDecision{source: sourceDirect, fallback: "pac: too busy to evaluate the PAC script now"})
	if got.fallback != "" || got.proxy == nil {
		t.Fatalf("store returned %+v, want the good decision kept", got)
	}
	if c := sel.cache[key]; c.d.fallback != "" {
		t.Fatalf("cache now holds the fallback %+v", c.d)
	}
}
