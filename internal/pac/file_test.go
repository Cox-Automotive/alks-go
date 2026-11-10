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
	clock = clock.Add(FirstBackoff + time.Second)
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

// Clawdius review of alks-go #6: ErrBusy is about other evaluations, so it does
// not put a healthy PAC file into backoff.
func TestFileBusyIsNotBackedOff(t *testing.T) {
	pacURL, _ := serve(t, `function FindProxyForURL(u, h) { return "PROXY 127.0.0.1:9000"; }`)
	f := &File{TTL: time.Minute}
	u := mustURL(t, "https://alks.example.test/")
	abandoned.Add(maxAbandoned) // Add, not Load/Store: see TestTooManyAbandonedEvaluations
	_, err := f.ProxyFor(pacURL, u)
	abandoned.Add(-maxAbandoned)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v, want ErrBusy", err)
	}
	if got, err := f.ProxyFor(pacURL, u); err != nil || got != "127.0.0.1:9000" {
		t.Fatalf("after the busy period: got %q, %v; want the proxy (no backoff)", got, err)
	}
}

// Cody's review: the backoff starts at FirstBackoff (as before this change), doubles
// with each consecutive strike up to BackoffAfterAbort, and resets on success.
func TestBackoffEscalatesAndCaps(t *testing.T) {
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 15 * time.Minute, 15 * time.Minute}
	for i, w := range want {
		if got := backoffFor(i + 1); got != w {
			t.Errorf("strike %d: backoff %v, want %v", i+1, got, w)
		}
	}
}

func TestFileBackoffEscalatesThenResetsOnSuccess(t *testing.T) {
	waitIdle(t)
	var slow atomic.Bool
	slow.Store(true)
	n := &atomic.Int32{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n.Add(1)
		if slow.Load() {
			fmt.Fprint(w, `function FindProxyForURL(u, h) { while (true) {} }`)
			return
		}
		fmt.Fprint(w, `function FindProxyForURL(u, h) { return "PROXY 127.0.0.1:9000"; }`)
	}))
	t.Cleanup(srv.Close)
	pacURL := srv.URL + "/proxy.pac"
	clock := time.Unix(0, 0)
	f := &File{TTL: time.Minute, Budget: 50 * time.Millisecond, Now: func() time.Time { return clock }}
	u := mustURL(t, "https://alks.example.test/")

	for i, wait := range []time.Duration{FirstBackoff, 2 * FirstBackoff} {
		if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrTimeout) {
			t.Fatalf("strike %d: got %v, want ErrTimeout", i+1, err)
		}
		clock = clock.Add(wait - time.Second)
		if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrBackoff) {
			t.Fatalf("strike %d: got %v just before %v, want ErrBackoff", i+1, err, wait)
		}
		clock = clock.Add(2 * time.Second)
	}
	slow.Store(false)
	if got, err := f.ProxyFor(pacURL, u); err != nil || got != "127.0.0.1:9000" {
		t.Fatalf("healthy PAC: got %q, %v", got, err)
	}
	// After a success, the next timeout starts again at FirstBackoff.
	slow.Store(true)
	clock = clock.Add(2 * time.Minute) // past the script TTL, so it is re-fetched
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrTimeout) {
		t.Fatalf("after reset: got %v, want ErrTimeout", err)
	}
	clock = clock.Add(FirstBackoff + time.Second)
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrTimeout) {
		t.Fatalf("after reset: got %v, want a fresh evaluation after FirstBackoff", err)
	}
}

// Cody's review: a memory stop that left a goroutine behind does back off.
func TestFileAbandonedMemoryStopIsBackedOff(t *testing.T) {
	waitIdle(t)
	defer blockingHelpers(t)()
	pacURL, _ := serve(t, `function FindProxyForURL(u, h) { hog(); return "DIRECT"; }`)
	f := &File{TTL: time.Minute, Budget: 10 * time.Second}
	u := mustURL(t, "https://alks.example.test/")
	if _, err := f.ProxyFor(pacURL, u); !errors.Is(err, ErrMemory) || !errors.Is(err, ErrAbandoned) {
		t.Fatalf("got %v, want ErrMemory wrapping ErrAbandoned", err)
	}
	if _, ok := f.inBackoff(pacURL); !ok {
		t.Fatal("not in backoff after an abandoned memory stop")
	}
}
