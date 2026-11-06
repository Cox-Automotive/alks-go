package alks

import (
	"os"
	"reflect"
	"testing"
)

func TestParseScutilProxy(t *testing.T) {
	cases := map[string]osProxyConfig{
		"testdata/scutil-pac.txt": {
			pacURL:                 "http://127.0.0.1:9000/systemproxy-test.pac",
			exceptions:             []string{"*.local", "169.254/16"},
			excludeSimpleHostnames: true,
		},
		"testdata/scutil-static.txt": {
			staticHTTPS: "secure-proxy.example.com:8443",
			exceptions:  []string{"*.internal.example.com", "localhost"},
		},
		"testdata/scutil-none.txt": {
			exceptions: []string{"*.local"},
		},
	}
	for file, want := range cases {
		b, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if got := parseScutilProxy(b); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got  %+v\n want %+v", file, got, want)
		}
	}
}

func TestMatchesException(t *testing.T) {
	cfg := osProxyConfig{exceptions: []string{"*.internal.example.com", ".corp", "exact.host", "<local>"}}
	for host, want := range map[string]bool{
		"a.internal.example.com": true,
		"internal.example.com":   true,
		"x.corp":                 true,
		"exact.host":             true,
		"intranet":               true,
		"alks.coxautoinc.com":    false,
	} {
		if got := matchesException(host, cfg); got != want {
			t.Errorf("%s: got %v want %v", host, got, want)
		}
	}
}
