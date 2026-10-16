package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// hbVersions returns every report ever sent for version n.
func (f *fakeCP) reportsFor(n int) []control.VersionReport {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []control.VersionReport
	for _, hb := range f.heartbeats {
		for _, v := range hb.Versions {
			if v.Number == n {
				out = append(out, v)
			}
		}
	}
	return out
}

func (l *lifecycle) retryPolicy(initial, max, budget time.Duration) {
	l.r.loadRetry = LoadRetry{Initial: initial, Max: max, Budget: budget}
}

func (l *lifecycle) oneVersion() func() *version {
	l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}})
	l.run()
	return func() *version { return l.version("app.hello", 1) }
}

func status(code int) error { return &control.ErrStatus{Status: code, Code: "X", Message: "boom"} }

// A control-plane 500 on the artifact fetch is retried; the version loads and
// FAILED is never reported (the platform would latch it).
func TestTransientFetchErrorIsRetriedAndNeverReportedFailed(t *testing.T) {
	l := newLifecycle(t, time.Second)
	l.retryPolicy(40*time.Millisecond, 40*time.Millisecond, time.Minute)
	var fetches atomic.Int32
	l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
		if fetches.Add(1) <= 2 {
			return nil, status(500)
		}
		return l.base.Artifact(ctx, digest)
	}
	get := l.oneVersion()
	l.waitState(get, stateReady)
	if n := fetches.Load(); n != 3 {
		t.Fatalf("fetches = %d, want 3 (two 500s then success)", n)
	}
	sawRetrying := false
	for _, rep := range l.base.reportsFor(1) {
		if rep.State == control.StateFailed {
			t.Fatalf("a transient failure was reported FAILED: %+v", rep)
		}
		if rep.State == control.StateRetrying && strings.Contains(rep.Reason, "HTTP 500") {
			sawRetrying = true
		}
	}
	if !sawRetrying {
		t.Fatal("the retrying detail never reached a heartbeat")
	}
}

// Persistent 500s: FAILED only after the budget, with the last error and the
// attempt count in the reason.
func TestPersistentTransientErrorFailsAfterBudgetWithAttempts(t *testing.T) {
	l := newLifecycle(t, time.Second)
	l.retryPolicy(5*time.Millisecond, 20*time.Millisecond, 300*time.Millisecond)
	var fetches atomic.Int32
	l.cp.artifact = func(context.Context, string) (io.ReadCloser, error) {
		fetches.Add(1)
		return nil, status(500)
	}
	began := time.Now()
	get := l.oneVersion()
	l.waitState(get, stateFailed)
	if el := time.Since(began); el < 150*time.Millisecond {
		t.Fatalf("FAILED after only %s: the budget was not honoured", el)
	}
	_, reason := get().currentState()
	n := fetches.Load()
	if n < 3 {
		t.Fatalf("only %d attempts inside the budget", n)
	}
	if !strings.Contains(reason, "HTTP 500") || !strings.Contains(reason, fmt.Sprintf("gave up after %d attempts", n)) {
		t.Fatalf("reason %q lacks the last error / attempt count (%d attempts)", reason, n)
	}
}

// A missing artifact (404) cannot get better: FAILED at once, one fetch, even
// with a huge budget.
func TestMissingArtifactFailsImmediately(t *testing.T) {
	l := newLifecycle(t, time.Second)
	l.retryPolicy(time.Hour, time.Hour, time.Hour)
	var fetches atomic.Int32
	l.cp.artifact = func(context.Context, string) (io.ReadCloser, error) {
		fetches.Add(1)
		return nil, status(404)
	}
	get := l.oneVersion()
	l.waitState(get, stateFailed)
	_, reason := get().currentState()
	if fetches.Load() != 1 || strings.Contains(reason, "gave up") || !strings.Contains(reason, "ARTIFACT") {
		t.Fatalf("fetches=%d reason=%q", fetches.Load(), reason)
	}
}

// A module the loader rejects fails at once; so does a digest mismatch.
func TestInvalidModuleAndDigestMismatchFailImmediately(t *testing.T) {
	garbage := []byte("this is not a wasm module")
	sum := sha256.Sum256(garbage)
	garbageDigest := hex.EncodeToString(sum[:])

	for name, tc := range map[string]struct {
		serve   []byte // what the control plane serves for the digest
		reasonK string
	}{
		"invalid module":  {serve: garbage, reasonK: ""},
		"digest mismatch": {serve: []byte("other bytes"), reasonK: "digest"},
	} {
		t.Run(name, func(t *testing.T) {
			l := newLifecycle(t, time.Second)
			l.retryPolicy(time.Hour, time.Hour, time.Hour)
			var fetches atomic.Int32
			l.cp.artifact = func(context.Context, string) (io.ReadCloser, error) {
				fetches.Add(1)
				return io.NopCloser(strings.NewReader(string(tc.serve))), nil
			}
			v := ver(1, control.RoleLive)
			v.Digest = garbageDigest
			l.base.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(v)}})
			l.run()
			get := func() *version { return l.version("app.hello", 1) }
			l.waitState(get, stateFailed)
			_, reason := get().currentState()
			if fetches.Load() != 1 || strings.Contains(reason, "gave up") {
				t.Fatalf("fetches=%d reason=%q: a permanent failure was retried", fetches.Load(), reason)
			}
			if !strings.Contains(reason, tc.reasonK) {
				t.Fatalf("reason %q", reason)
			}
		})
	}
}

// An exhausted transient failure is re-attempted when desired state is
// re-rendered, with no backoff to wait out beyond retryBase.
func TestExhaustedTransientFailureIsRetriedOnRerender(t *testing.T) {
	l := newLifecycle(t, time.Second)
	l.retryPolicy(time.Millisecond, time.Millisecond, time.Nanosecond) // one attempt, then FAILED
	var healthy atomic.Bool
	l.cp.artifact = func(ctx context.Context, digest string) (io.ReadCloser, error) {
		if !healthy.Load() {
			return nil, status(503)
		}
		return l.base.Artifact(ctx, digest)
	}
	doc := control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(1, control.RoleLive))}}
	l.base.push(doc)
	l.run()
	get := func() *version { return l.version("app.hello", 1) }
	l.waitState(get, stateFailed)
	_, reason := get().currentState()
	if !strings.Contains(reason, "gave up after 1 attempt ") {
		t.Fatalf("reason %q", reason)
	}
	healthy.Store(true)
	time.Sleep(30 * time.Millisecond) // past retryBase (10ms)
	l.base.push(doc)
	l.waitState(get, stateReady)
}

func TestIsTransientLoadError(t *testing.T) {
	fetch := func(err error) error { return &fetchError{err} }
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"500", fetch(status(500)), true},
		{"502", fetch(status(502)), true},
		{"503", fetch(status(503)), true},
		{"504", fetch(status(504)), true},
		{"429", fetch(status(429)), true},
		{"408", fetch(status(408)), true},
		{"401 token blip", fetch(status(401)), true},
		{"403 lapsed presign", fetch(status(403)), true},
		{"404", fetch(status(404)), false},
		{"410", fetch(status(410)), false},
		{"400", fetch(status(400)), false},
		{"422", fetch(status(422)), false},
		{"deadline", fetch(context.DeadlineExceeded), true},
		{"connection reset", fetch(&net.OpError{Op: "read", Err: syscall.ECONNRESET}), true},
		{"connection refused", fetch(&net.OpError{Op: "dial", Err: syscall.ECONNREFUSED}), true},
		{"truncated body", fetch(io.ErrUnexpectedEOF), true},
		{"dns", fetch(&net.DNSError{Err: "no such host", IsNotFound: true}), true},
		{"wrapped status", fetch(fmt.Errorf("get: %w", status(500))), true},
		{"digest mismatch", fetch(permanentError{errors.New("artifact does not match its digest")}), false},
		{"module compile error", errors.New("LOAD: bad magic"), false},
		{"validation error", errors.New("ABI: unsupported"), false},
	}
	for _, c := range cases {
		if got := isTransientLoadError(c.err); got != c.want {
			t.Errorf("%s: transient = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestLoadRetryDelay(t *testing.T) {
	p := LoadRetry{Initial: time.Second, Max: 30 * time.Second, Budget: time.Minute}.withDefaults()
	for attempt, hi := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 3: 4 * time.Second, 6: 30 * time.Second, 50: 30 * time.Second, 1000: 30 * time.Second} {
		for range 200 {
			d := p.delay(attempt)
			if d < hi/2 || d > hi {
				t.Fatalf("attempt %d: delay %s outside [%s, %s]", attempt, d, hi/2, hi)
			}
		}
	}
	z := LoadRetry{}.withDefaults()
	if z.Initial != DefaultLoadRetryInitial || z.Max != DefaultLoadRetryMax || z.Budget != DefaultLoadRetryBudget {
		t.Fatalf("defaults: %+v", z)
	}
}
