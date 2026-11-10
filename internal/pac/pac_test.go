package pac

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/robertkrimen/otto"
)

// waitIdle makes the test wait, at cleanup, for any evaluation goroutine it
// abandoned, so later tests do not see its heap use or hit maxAbandoned.
func waitIdle(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		deadline := time.Now().Add(60 * time.Second)
		for abandoned.Load() > 0 && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
		if n := abandoned.Load(); n > 0 {
			t.Errorf("%d evaluations still running after 60s", n)
		}
	})
}

func load(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestZscalerLikePAC(t *testing.T) {
	s, err := Compile(load(t, "pac-zscaler-like.js"), 2*time.Second)
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
	if _, err := Compile(load(t, "pac-broken.js"), 2*time.Second); err == nil {
		t.Fatal("expected a compile error")
	}
	if _, err := Compile("var x = 1;", 2*time.Second); err == nil {
		t.Fatal("expected an error for a script without FindProxyForURL")
	}
}

func TestSlowPACTimesOut(t *testing.T) {
	waitIdle(t)
	s, err := Compile(load(t, "pac-slow.js"), 2*time.Second)
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
	s, err := Compile(src, 2*time.Second)
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

// Sam's review: the interpreter must be bounded in every phase.
func TestCompileTopLevelLoopTimesOut(t *testing.T) {
	waitIdle(t)
	start := time.Now()
	_, err := Compile("while (true) {}\nfunction FindProxyForURL(u, h) { return 'DIRECT'; }", 200*time.Millisecond)
	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestUnboundedRecursionIsAScriptError(t *testing.T) {
	s, err := Compile("function f(n) { return f(n + 1); }\nfunction FindProxyForURL(u, h) { return f(0); }", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Without a stack-depth limit this is a Go "fatal error: stack overflow"
	// that recover() cannot catch, and the whole test binary dies.
	if _, err := s.FindProxyForURL("https://alks.example.test/", 5*time.Second); err == nil {
		t.Fatal("expected an error from unbounded recursion")
	}
}

func TestBuiltinIgnoringInterruptStillReturnsInBudget(t *testing.T) {
	waitIdle(t)
	s, err := Compile("function FindProxyForURL(u, h) { var a = []; a[30000000] = 1; return a.join('x'); }", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.FindProxyForURL("https://alks.example.test/", 300*time.Millisecond)
	// The 30M-element join is stopped by the budget or, usually first, by the
	// memory limit; either way the call returns without waiting for the builtin.
	if !errors.Is(err, ErrTimeout) && !errors.Is(err, ErrMemory) {
		t.Fatalf("got %v, want ErrTimeout or ErrMemory", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatalf("returned after %v, want about the 300ms budget", time.Since(start))
	}
}

func TestRedact(t *testing.T) {
	if got := Redact("PROXY user:secret@proxy:8080; DIRECT"); got != "PROXY proxy:8080; DIRECT" {
		t.Fatalf("got %q", got)
	}
}

func TestWeekdayAndTimeRange(t *testing.T) {
	restore := now
	defer func() { now = restore }()
	// Saturday 2026-11-07 23:30 UTC
	now = func() time.Time { return time.Date(2026, 11, 7, 23, 30, 0, 0, time.UTC) }
	src := `function FindProxyForURL(u, h) { return [
		weekdayRange("SAT"), weekdayRange("MON", "FRI"), weekdayRange("FRI", "MON"),
		timeRange(23), timeRange(9, 17), timeRange(22, 2), timeRange(23, 0, 23, 45, "GMT"), weekdayRange("XYZ")
	].join(","); }`
	s, err := Compile(src, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.FindProxyForURL("http://x/", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if want := "true,false,true,true,false,true,true,false"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// Cody's review sec-2: heap growth is bounded, not only time.
func TestMemoryGrowthIsStopped(t *testing.T) {
	waitIdle(t)
	s, err := Compile("function FindProxyForURL(u, h) { var s = 'x'; while (true) { s = s + s; } }", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, err = s.FindProxyForURL("https://alks.example.test/", 10*time.Second)
	if !errors.Is(err, ErrMemory) {
		t.Fatalf("got %v, want ErrMemory", err)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("stopped after %v, want well before the 10s budget", time.Since(start))
	}
}

func TestTooManyAbandonedEvaluations(t *testing.T) {
	// Add, not Load/Store: a goroutine abandoned by an earlier test may still
	// decrement the counter meanwhile.
	abandoned.Add(maxAbandoned)
	defer abandoned.Add(-maxAbandoned)
	if _, err := Compile("function FindProxyForURL(u, h) { return 'DIRECT'; }", time.Second); !errors.Is(err, ErrBusy) {
		t.Fatalf("got %v, want ErrBusy", err)
	}
}

// Clawdius review of alks-go #6: healthy evaluations running at the same time
// never hit the cap; only abandoned ones count.
func TestConcurrentHealthyEvaluationsAreNotRefused(t *testing.T) {
	s, err := Compile("function FindProxyForURL(u, h) { for (var i = 0; i < 20000; i++) {} return 'PROXY 127.0.0.1:9000'; }", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	const n = 4 * maxAbandoned
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			_, err := s.FindProxyForURL("https://alks.example.test/", 5*time.Second)
			errs <- err
		}()
	}
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Errorf("concurrent evaluation failed: %v", err)
		}
	}
	if a := abandoned.Load(); a != 0 {
		t.Fatalf("abandoned = %d after healthy evaluations, want 0", a)
	}
}

func TestAbandonedCountReturnsToZero(t *testing.T) {
	waitIdle(t)
	s, err := Compile("function FindProxyForURL(u, h) { while (true) {} }", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.FindProxyForURL("https://alks.example.test/", 50*time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("got %v, want ErrTimeout", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for abandoned.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a := abandoned.Load(); a != 0 {
		t.Fatalf("abandoned = %d after the interrupted loop stopped, want 0", a)
	}
}

// blockingHelpers registers test-only builtins: block() waits for release; hog()
// allocates about 300 MiB, then waits for release (both ignore the interrupt, as a
// native builtin can); grow() allocates 16 MiB and returns at once, so a loop
// calling it grows the heap but stops at the interrupt.
func blockingHelpers(t *testing.T) (release func()) {
	t.Helper()
	ch := make(chan struct{})
	var (
		mu   sync.Mutex
		keep [][]byte
	)
	vmHook = func(vm *otto.Otto) {
		_ = vm.Set("block", func(otto.FunctionCall) otto.Value { <-ch; return otto.UndefinedValue() })
		_ = vm.Set("grow", func(otto.FunctionCall) otto.Value {
			b := make([]byte, 16<<20)
			for i := 0; i < len(b); i += 4096 {
				b[i] = 1
			}
			mu.Lock()
			keep = append(keep, b)
			mu.Unlock()
			return otto.UndefinedValue()
		})
		_ = vm.Set("hog", func(otto.FunctionCall) otto.Value {
			b := make([]byte, 300<<20)
			for i := 0; i < len(b); i += 4096 {
				b[i] = 1
			}
			mu.Lock()
			keep = append(keep, b)
			mu.Unlock()
			<-ch
			return otto.UndefinedValue()
		})
	}
	var once sync.Once
	release = func() {
		once.Do(func() {
			close(ch)
			vmHook = nil
			mu.Lock()
			keep = nil
			mu.Unlock()
		})
	}
	t.Cleanup(release)
	// Collect earlier tests' garbage first. The memory guard measures heap growth
	// for the whole process, and a collection during hog() would otherwise hide
	// its allocation.
	runtime.GC()
	return release
}

// Sam's review (LOW): the time-budget abandon path, deterministically. The
// counter is 1 while the builtin is blocked and 0 once it returns.
func TestTimeoutInBlockedBuiltinIsAbandoned(t *testing.T) {
	waitIdle(t)
	release := blockingHelpers(t)
	s, err := Compile("function FindProxyForURL(u, h) { block(); return 'DIRECT'; }", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.FindProxyForURL("https://alks.example.test/", 50*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || !errors.Is(err, ErrAbandoned) {
		t.Fatalf("got %v, want ErrTimeout wrapping ErrAbandoned", err)
	}
	if a := abandoned.Load(); a != 1 {
		t.Fatalf("abandoned = %d while blocked, want 1", a)
	}
	release()
	deadline := time.Now().Add(5 * time.Second)
	for abandoned.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if a := abandoned.Load(); a != 0 {
		t.Fatalf("abandoned = %d after release, want 0", a)
	}
}

// A loop stops at the interrupt, so a timeout there is not reported as abandoned.
func TestTimeoutInLoopIsNotAbandoned(t *testing.T) {
	waitIdle(t)
	s, err := Compile("function FindProxyForURL(u, h) { while (true) {} }", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.FindProxyForURL("https://alks.example.test/", 50*time.Millisecond)
	if !errors.Is(err, ErrTimeout) || errors.Is(err, ErrAbandoned) {
		t.Fatalf("got %v, want ErrTimeout without ErrAbandoned", err)
	}
}

// Cody's review: a memory stop inside a builtin that ignores the interrupt is
// reported as abandoned.
func TestMemoryStopInBlockedBuiltinIsAbandoned(t *testing.T) {
	waitIdle(t)
	defer blockingHelpers(t)()
	s, err := Compile("function FindProxyForURL(u, h) { hog(); return 'DIRECT'; }", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.FindProxyForURL("https://alks.example.test/", 10*time.Second)
	if !errors.Is(err, ErrMemory) || !errors.Is(err, ErrAbandoned) {
		t.Fatalf("got %v, want ErrMemory wrapping ErrAbandoned", err)
	}
}

// Sam's review: a valid-sized PAC file with deep bracket nesting must be rejected
// before otto's parser can overflow the Go stack (fatal at about 225,000 levels).
func TestDeepNestingIsRejected(t *testing.T) {
	deep := "function FindProxyForURL(u, h) { return " + strings.Repeat("(", 200000) + "1" + strings.Repeat(")", 200000) + "; }"
	if len(deep) >= MaxSize {
		t.Fatalf("test script is %d bytes, want it under MaxSize", len(deep))
	}
	if _, err := Compile(deep, time.Second); err == nil || !strings.Contains(err.Error(), "nests brackets") {
		t.Fatalf("got %v, want the nesting error", err)
	}
	over := "function FindProxyForURL(u, h) { return " + strings.Repeat("[", maxNesting+1) + strings.Repeat("]", maxNesting+1) + "; }"
	if _, err := Compile(over, time.Second); err == nil {
		t.Fatal("nesting just over the limit was accepted")
	}
	// Brackets inside strings and comments do not count.
	ok := "// ((((((((\n/* [[[[[[[[ */ function FindProxyForURL(u, h) { var s = \"((((\\\"((((\"; var t = '[[[['; return 'DIRECT'; }"
	if d := nestingDepth(ok); d != 1 {
		t.Fatalf("nestingDepth = %d, want 1 (the function body)", d)
	}
	if _, err := Compile(ok, time.Second); err != nil {
		t.Fatalf("ordinary script rejected: %v", err)
	}
	if _, err := Compile(load(t, "pac-zscaler-like.js"), time.Second); err != nil {
		t.Fatalf("Zscaler-like PAC rejected: %v", err)
	}
}

// Sam's review: the heap check is process-wide, so evaluations run one at a time;
// a memory-heavy script started first does not stop a healthy one started 30 ms
// later.
func TestMemoryHeavyEvaluationDoesNotStopAHealthyOne(t *testing.T) {
	waitIdle(t)
	defer blockingHelpers(t)()
	// Grows 16 MiB per step with a short loop between steps, so it takes most of a
	// second to pass the limit and overlaps the healthy evaluation.
	bomb, err := Compile("function FindProxyForURL(u, h) { while (true) { grow(); for (var i = 0; i < 50000; i++) {} } }", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// The healthy script runs for over a second, so that without one-at-a-time
	// evaluation it would overlap the memory-heavy one and see its heap growth.
	healthy, err := Compile("function FindProxyForURL(u, h) { for (var i = 0; i < 1500000; i++) {} return 'PROXY 127.0.0.1:9000'; }", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bombErr := make(chan error, 1)
	go func() {
		_, err := bomb.FindProxyForURL("https://alks.example.test/", 10*time.Second)
		bombErr <- err
	}()
	time.Sleep(30 * time.Millisecond)
	answer, err := healthy.FindProxyForURL("https://alks.example.test/", 10*time.Second)
	if err != nil || answer != "PROXY 127.0.0.1:9000" {
		t.Fatalf("healthy evaluation: got %q, %v", answer, err)
	}
	if err := <-bombErr; !errors.Is(err, ErrMemory) {
		t.Fatalf("memory-heavy evaluation: got %v, want ErrMemory", err)
	}
}
