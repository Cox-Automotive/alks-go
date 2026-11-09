package pac

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func serve(t *testing.T, body string) (string, *atomic.Int32) {
	t.Helper()
	n := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		fmt.Fprint(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/proxy.pac", n
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestFileCachesAndRefetchesAfterTTL(t *testing.T) {
	pacURL, fetches := serve(t, `function FindProxyForURL(u, h) { return "PROXY 127.0.0.1:9000"; }`)
	clock := time.Unix(0, 0)
	f := &File{TTL: time.Minute, Now: func() time.Time { return clock }}
	for i := 0; i < 3; i++ {
		got, err := f.ProxyFor(pacURL, mustURL(t, "https://alks.example.test/"))
		if err != nil || got != "127.0.0.1:9000" {
			t.Fatalf("got %q, %v", got, err)
		}
	}
	if n := fetches.Load(); n != 1 {
		t.Fatalf("fetched %d times before the TTL, want 1", n)
	}
	clock = clock.Add(2 * time.Minute)
	if _, err := f.ProxyFor(pacURL, mustURL(t, "https://alks.example.test/")); err != nil {
		t.Fatal(err)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("fetched %d times after the TTL, want 2", n)
	}
}

// Cody's review sec-2: after an aborted run the PAC file is left alone for
// BackoffAfterAbort, so no new goroutine is started every failure TTL.
func TestFileBacksOffAfterTimeout(t *testing.T) {
	waitIdle(t)
	pacURL, fetches := serve(t, `function FindProxyForURL(u, h) { while (true) {} }`)
	clock := time.Unix(0, 0)
	f := &File{TTL: time.Minute, Budget: 100 * time.Millisecond, Now: func() time.Time { return clock }}
	u := mustURL(t, "https://alks.example.test/")
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrTimeout) {
		t.Fatalf("first call: got %v, want ErrTimeout", err)
	}
	start := time.Now()
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrBackoff) {
		t.Fatalf("second call: got %v, want ErrBackoff", err)
	}
	if time.Since(start) > 50*time.Millisecond {
		t.Fatalf("backoff call took %v; it should not evaluate", time.Since(start))
	}
	clock = clock.Add(BackoffAfterAbort + time.Second)
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrTimeout) {
		t.Fatalf("after backoff: got %v, want a fresh evaluation (ErrTimeout)", err)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("fetched %d times, want 2 (the script is dropped after an abort)", n)
	}
}

func TestFileOrdinaryErrorIsNotBackedOff(t *testing.T) {
	pacURL, _ := serve(t, `function FindProxyForURL(u, h) { throw "boom"; }`)
	f := &File{TTL: time.Minute}
	u := mustURL(t, "https://alks.example.test/")
	for i := 0; i < 2; i++ {
		if _, err := f.ProxyFor(pacURL, u); err == nil || errors.Is(err, ErrBackoff) {
			t.Fatalf("call %d: got %v, want the script error each time", i, err)
		}
	}
}

func TestFetchRejectsOversizedFile(t *testing.T) {
	big := "function FindProxyForURL(u, h) { return 'DIRECT'; }\n//" + strings.Repeat("x", MaxSize)
	pacURL, _ := serve(t, big)
	if _, err := Fetch(pacURL); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("got %v, want a size error (not a truncated script)", err)
	}
	ok := "function FindProxyForURL(u, h) { return 'DIRECT'; }"
	pacURL, _ = serve(t, ok)
	if got, err := Fetch(pacURL); err != nil || got != ok {
		t.Fatalf("got %q, %v", got, err)
	}
}

// A memory stop is not backed off: the heap is process-wide, so a false stop must
// only cost one retry interval.
func TestFileMemoryStopIsNotBackedOff(t *testing.T) {
	waitIdle(t)
	pacURL, _ := serve(t, `function FindProxyForURL(u, h) { var s = 'x'; while (true) { s = s + s; } }`)
	f := &File{TTL: time.Minute, Budget: 10 * time.Second}
	u := mustURL(t, "https://alks.example.test/")
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrMemory) {
		t.Fatalf("got %v, want ErrMemory", err)
	}
	if until, ok := f.inBackoff(pacURL); ok {
		t.Fatalf("in backoff until %v after a memory stop", until)
	}
}
