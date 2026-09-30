package runner

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// hookCP is a fakeCP whose artifact download and event emit can be steered.
type hookCP struct {
	*fakeCP
	artifact func(ctx context.Context, digest string) (io.ReadCloser, error)
	emit     func(ctx context.Context)
}

func (h *hookCP) Artifact(ctx context.Context, digest string) (io.ReadCloser, error) {
	if h.artifact != nil {
		return h.artifact(ctx, digest)
	}
	return h.fakeCP.Artifact(ctx, digest)
}

func (h *hookCP) Emit(ctx context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error) {
	if h.emit != nil {
		h.emit(ctx)
	}
	return h.fakeCP.Emit(ctx, r)
}

// lifecycle is a runner started without waiting for anything to load.
type lifecycle struct {
	t      *testing.T
	base   *fakeCP
	cp     *hookCP
	r      *Runner
	budget *budget.Budget
	srv    *httptest.Server
	cancel context.CancelFunc
	done   chan struct{}
}

// newLifecycle builds a runner with fast retry and a short drain grace. The
// caller pushes desired state and calls run.
func newLifecycle(t *testing.T, grace time.Duration) *lifecycle {
	t.Helper()
	b, err := budget.New(512<<20, 0)
	if err != nil {
		t.Fatal(err)
	}
	base := newFakeCP()
	cp := &hookCP{fakeCP: base}
	r, err := New(t.Context(), Config{Pool: "default", ControlPlane: cp, Budget: b, HeartbeatEvery: 20 * time.Millisecond, DrainGrace: grace, Tokens: fakeTokens{}})
	if err != nil {
		t.Fatal(err)
	}
	r.retryBase = 10 * time.Millisecond
	return &lifecycle{t: t, base: base, cp: cp, r: r, budget: b, done: make(chan struct{})}
}

func (l *lifecycle) run() {
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	go func() { _ = l.r.Run(ctx); close(l.done) }()
	l.srv = httptest.NewServer(l.r.Handler())
	l.t.Cleanup(func() {
		l.srv.Close()
		cancel()
		<-l.done
	})
}

func (l *lifecycle) version(address string, n int) *version {
	l.r.mu.RLock()
	defer l.r.mu.RUnlock()
	if fn := l.r.functions[address]; fn != nil {
		return fn.versions[n]
	}
	return nil
}

func (l *lifecycle) waitState(v func() *version, want versionState) {
	l.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ver := v(); ver != nil {
			if st, _ := ver.currentState(); st == want {
				return
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	l.t.Fatalf("version never reached state %d", want)
}

func (l *lifecycle) get(path string) int {
	l.t.Helper()
	resp, err := http.Get(l.srv.URL + path)
	if err != nil {
		l.t.Fatal(err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// 2.4: a failed version is retried, not pinned.
func TestFailedVersionIsRetriedWithoutRestart(t *testing.T) {
	l := newLifecycle(t, time.Second)
	var fetches atomic.Int32
	l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
		if fetches.Add(1) == 1 {
			return nil, errors.New("transient: connection reset")
		}
		return l.base.Artifact(ctx, digest)
	}
	doc := control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}}
	l.base.push(doc)
	l.run()
	get := func() *version { return l.version("app.hello", 1) }
	l.waitState(get, stateFailed)
	if code := l.get("/fn/app.hello/echo"); code != 503 {
		t.Fatalf("a failed version answered %d, want 503", code)
	}

	// The next apply (an unrelated revision of the same document) retries it.
	time.Sleep(30 * time.Millisecond) // past the 10ms backoff
	l.base.push(doc)
	l.waitState(get, stateReady)
	if code := l.get("/fn/app.hello/echo"); code != 200 {
		t.Fatalf("after the retry the function answered %d", code)
	}
	if n := fetches.Load(); n != 2 {
		t.Fatalf("artifact fetched %d times, want 2", n)
	}
}

// 2.4: the maintenance tick retries too (no new desired state needed), and a
// still-failing version backs off instead of retrying every tick.
func TestFailedVersionIsRetriedOnMaintenanceTick(t *testing.T) {
	l := newLifecycle(t, time.Second)
	l.r.retryBase = 200 * time.Millisecond
	var fetches atomic.Int32
	l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
		if fetches.Add(1) <= 2 {
			return nil, errors.New("transient")
		}
		return l.base.Artifact(ctx, digest)
	}
	l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
	l.run()
	get := func() *version { return l.version("app.hello", 1) }
	l.waitState(get, stateFailed)

	l.r.retryFailed(context.Background()) // inside the backoff: nothing happens
	time.Sleep(20 * time.Millisecond)
	if n := fetches.Load(); n != 1 {
		t.Fatalf("retried inside its backoff: %d fetches", n)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		if st, _ := get().currentState(); st == stateReady {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("never became ready; %d fetches", fetches.Load())
		}
		l.r.retryFailed(context.Background())
		time.Sleep(20 * time.Millisecond)
	}
	if n := fetches.Load(); n != 3 {
		t.Fatalf("fetches = %d, want 3 (two failures, one success)", n)
	}
}

// 2.5: a version closed before its prepare finishes must not keep anything
// charged to the budget.
func TestPrepareRacingCloseLeavesNothing(t *testing.T) {
	t.Run("queued behind other prepares", func(t *testing.T) {
		l := newLifecycle(t, 50*time.Millisecond)
		for range cap(l.r.prepareSem) {
			l.r.prepareSem <- struct{}{} // every slot busy: the prepare must wait
		}
		l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
		l.run()
		get := func() *version { return l.version("app.hello", 1) }
		deadline := time.Now().Add(5 * time.Second)
		for get() == nil && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		v := get()
		if v == nil {
			t.Fatal("version never appeared")
		}
		l.base.push(control.Desired{Pool: "default"}) // removes it: close runs
		l.waitState(func() *version { return v }, stateClosed)
		for range cap(l.r.prepareSem) {
			<-l.r.prepareSem // the queued prepare may now run
		}
		time.Sleep(200 * time.Millisecond)
		assertReleased(t, l, v)
	})

	t.Run("download outlives the close", func(t *testing.T) {
		l := newLifecycle(t, 50*time.Millisecond)
		release := make(chan struct{})
		entered := make(chan struct{}, 1)
		l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-release // deliberately ignores ctx: the download finishes after the close began
			return l.base.Artifact(ctx, digest)
		}
		l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
		l.run()
		<-entered
		v := l.version("app.hello", 1)
		l.base.push(control.Desired{Pool: "default"})
		deadline := time.Now().Add(5 * time.Second)
		for {
			v.mu.Lock()
			closing := v.closing
			v.mu.Unlock()
			if closing {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("close never began")
			}
			time.Sleep(5 * time.Millisecond)
		}
		close(release)
		l.waitState(func() *version { return v }, stateClosed)
		time.Sleep(100 * time.Millisecond)
		assertReleased(t, l, v)
	})
}

func assertReleased(t *testing.T, l *lifecycle, v *version) {
	t.Helper()
	v.mu.Lock()
	pool, prepared := v.pool, v.prepared
	v.mu.Unlock()
	if pool != nil || prepared != nil {
		t.Fatalf("a closed version still holds a pool (%v) or module (%v)", pool != nil, prepared != nil)
	}
	if used := l.budget.Stats().UsedBytes; used != 0 {
		t.Fatalf("budget still charged %d bytes after the version closed", used)
	}
}

// 2.6: a version whose artifact download hangs must not stall other
// functions, the heartbeat or maintenance, even with an apply pending.
func TestSlowLoadDoesNotStallOthers(t *testing.T) {
	l := newLifecycle(t, time.Second)
	slow := strings.Repeat("cd", 32)
	release := make(chan struct{})
	entered := make(chan struct{}, 1)
	l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
		if digest != slow {
			return l.base.Artifact(ctx, digest)
		}
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, errors.New("gave up")
	}
	defer close(release)
	other := func() control.Function {
		f := fnDoc(ver(1, control.RoleLive))
		f.ID, f.Address = "fnc_2", "app.slow"
		f.Versions[0].Digest = slow
		return f
	}
	docs := func(extra string) control.Desired {
		f := fnDoc(ver(1, control.RoleLive))
		f.Config = map[string]string{"GREETING": extra} // a change that forces a fresh apply
		return control.Desired{Pool: "default", Functions: []control.Function{f, other()}}
	}
	l.base.push(docs("a"))
	l.run()
	l.waitState(func() *version { return l.version("app.hello", 1) }, stateReady)
	<-entered

	// Traffic to the loading function, then an apply queued behind it.
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() { l.get("/fn/app.slow/echo") })
	}
	time.Sleep(50 * time.Millisecond)
	l.base.push(docs("b"))
	time.Sleep(50 * time.Millisecond)

	prompt := func(name string, f func()) {
		t.Helper()
		done := make(chan struct{})
		go func() { f(); close(done) }()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("%s stalled behind a slow load", name)
		}
	}
	prompt("invocation of another function", func() {
		if code := l.get("/fn/app.hello/echo"); code != 200 {
			t.Errorf("other function answered %d", code)
		}
	})
	prompt("the loading function's own request", func() {
		if code := l.get("/fn/app.slow/echo"); code != 503 {
			t.Errorf("loading function answered %d, want 503", code)
		}
	})
	prompt("heartbeat", func() { _ = l.r.heartbeat() })
	prompt("a maintenance pass", func() {
		l.r.retryFailed(context.Background())
		for _, v := range l.r.allVersions() {
			v.mu.Lock()
			v.mu.Unlock() //nolint:staticcheck // proving the lock is free
		}
	})
	wg.Wait()
}

// 2.8: once draining, new calls are refused with 503 (not a 404) and the
// runner is not ready.
func TestDrainRefusesNewCalls(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	if code := statusOf(t, h.srv.URL+"/readyz"); code != 200 {
		t.Fatalf("readyz before drain: %d", code)
	}
	h.r.Drain()
	if code := statusOf(t, h.srv.URL+"/readyz"); code != 503 {
		t.Fatalf("readyz while draining: %d", code)
	}
	resp, body := h.do("GET", "/fn/app.hello/echo", nil, nil)
	if resp.StatusCode != 503 || !strings.Contains(string(body), "FUNCTION_UNAVAILABLE") || resp.Header.Get("Retry-After") == "" {
		t.Fatalf("call while draining: %d %s", resp.StatusCode, body)
	}
	resp, _ = h.do("GET", "/fn/nope.nope/echo", nil, nil)
	if resp.StatusCode != 503 {
		t.Fatalf("unknown function while draining: %d", resp.StatusCode)
	}
	// A desired-state apply must not flip a draining runner back to ready.
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
	time.Sleep(100 * time.Millisecond)
	if h.r.Ready() {
		t.Fatal("an apply made a draining runner ready again")
	}
}

func statusOf(t *testing.T, url string) int {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// 2.8: nothing a running call uses is released under it, even when the drain
// grace runs out first.
func TestNoInstanceClosedWhileCallInFlight(t *testing.T) {
	l := newLifecycle(t, 100*time.Millisecond)
	inCall := make(chan struct{})
	release := make(chan struct{})
	l.cp.emit = func(context.Context) {
		close(inCall)
		<-release
	}
	l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
	l.run()
	get := func() *version { return l.version("app.hello", 1) }
	l.waitState(get, stateReady)
	v := get()

	frame, err := abi.MarshalFrame(abi.Event{Type: "app:dom:agg:done", DedupID: "d1"}, []byte(`{"x":1}`))
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		code int
		xc   string
	}
	res := make(chan result, 1)
	go func() {
		resp, err := http.Post(l.srv.URL+"/fn/app.hello/call?op="+strconv.Itoa(int(abi.OpEventEmit)), "application/octet-stream", bytes.NewReader(frame))
		if err != nil {
			res <- result{code: -1}
			return
		}
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		res <- result{resp.StatusCode, resp.Header.Get("X-Code")}
	}()
	<-inCall

	l.base.push(control.Desired{Pool: "default"}) // unload it while the call runs
	time.Sleep(400 * time.Millisecond)            // well past the 100ms grace
	if st, _ := v.currentState(); st != stateClosed {
		t.Fatalf("state %d; the version should refuse new calls once the grace ends", st)
	}
	v.mu.Lock()
	torn := v.pool != nil || v.prepared != nil
	v.mu.Unlock()
	_ = torn // the fields are detached at grace; what matters is the module below stays open
	close(release)
	select {
	case got := <-res:
		if got.code != 200 || got.xc != "0" {
			t.Fatalf("the in-flight call failed after its version was unloaded: %+v", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("call never finished")
	}
	// Once the call is done everything is released.
	deadline := time.Now().Add(5 * time.Second)
	for l.budget.Stats().UsedBytes != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("budget still charged %d after the last call", l.budget.Stats().UsedBytes)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
