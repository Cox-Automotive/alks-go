package pac

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func load(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestZscalerLikePAC(t *testing.T) {
	s, err := Compile(load(t, "pac-zscaler-like.js"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := []struct{ url, want string }{
		{"https://alks.coxautoinc.com/rest/loginRoles/id/me", "PROXY 127.0.0.1:9000"},
		{"https://login.microsoftonline.com/x", "DIRECT"},
		{"https://list.example.com/", "PROXY 127.0.0.1:9000; DIRECT"},
		{"http://intranet/", "DIRECT"},
		{"https://10.1.2.3/", "DIRECT"},
		{"https://other.example.org/", "PROXY 127.0.0.1:9000"},
	}
	for _, c := range cases {
		got, err := s.FindProxyForURL(c.url, 2*time.Second)
		if err != nil {
			t.Errorf("%s: %v", c.url, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.url, got, c.want)
		}
	}
}

func TestBrokenPAC(t *testing.T) {
	if _, err := Compile(load(t, "pac-broken.js")); err == nil {
		t.Fatal("expected a compile error")
	}
	if _, err := Compile("var x = 1;"); err == nil {
		t.Fatal("expected an error for a script without FindProxyForURL")
	}
}

func TestSlowPACTimesOut(t *testing.T) {
	s, err := Compile(load(t, "pac-slow.js"))
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	start := time.Now()
	_, err = s.FindProxyForURL("https://alks.coxautoinc.com/", 200*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("timeout took too long: %v", time.Since(start))
	}
}

func TestFirstEntry(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"PROXY 127.0.0.1:9000", "127.0.0.1:9000", true},
		{"PROXY a:9000; DIRECT", "a:9000", true},
		{"  proxy a:1 ;PROXY b:2", "a:1", true},
		{"DIRECT", "", true},
		{"DIRECT; PROXY a:9000", "", true},
		{"", "", true},
		{"SOCKS a:1080", "", false},
		{"HTTPS a:443", "", false},
		{"PROXY", "", false},
		{"PROXY nohostport", "", false},
		{"garbage", "", false},
	}
	for _, c := range cases {
		got, ok := FirstEntry(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("FirstEntry(%q) = %q, %v; want %q, %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestGlobMatch(t *testing.T) {
	cases := []struct {
		p, s string
		want bool
	}{
		{"*.example.com", "a.example.com", true},
		{"*.example.com", "example.com", false},
		{"*/path/*", "http://h/path/x", true},
		{"a?c", "abc", true},
		{"a?c", "ac", false},
		{"*", "", true},
		{"exact", "exact", true},
	}
	for _, c := range cases {
		if got := globMatch(c.p, c.s); got != c.want {
			t.Errorf("globMatch(%q, %q) = %v, want %v", c.p, c.s, got, c.want)
		}
	}
}

func TestHelpersInScript(t *testing.T) {
	src := `function FindProxyForURL(url, host) {
		var r = [];
		r.push(isPlainHostName("intranet"));
		r.push(dnsDomainIs("a.example.com", ".example.com"));
		r.push(localHostOrDomainIs("www", "www.example.com"));
		r.push(isInNet("10.1.2.3", "10.0.0.0", "255.0.0.0"));
		r.push(isInNet("11.1.2.3", "10.0.0.0", "255.0.0.0"));
		r.push(dnsDomainLevels("a.b.c"));
		r.push(dnsResolve("127.0.0.1"));
		r.push(isResolvable("127.0.0.1"));
		return r.join(",");
	}`
	s, err := Compile(src)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	got, err := s.FindProxyForURL("http://x/", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	want := "true,true,true,true,false," + itoa(2) + ",127.0.0.1,true"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
