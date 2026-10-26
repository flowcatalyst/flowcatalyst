package engine

import (
	"context"
	_ "embed"
	"errors"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/jsengine"
)

// The JS engine runs on the cooperative runtime: wazero does not stop it, so
// these tests are what stands behind "a function cannot run past its deadline"
// (docs/function-runtime-deadlines.md). Each script is a way a function can
// try to keep the CPU.

func jsHandler(body string) []byte {
	return []byte(`globalThis.__fc = {
  describe() { return "{}"; },
  handle(meta, body) { ` + body + ` },
};`)
}

const jsOK = `return { meta: '{"status":200}', body: "ok" };`

func coopInstance(t *testing.T, script []byte) (*Engine, *Instance) {
	t.Helper()
	b, _ := budget.New(512<<20, 0)
	e, err := New(t.Context(), Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	m, err := e.CompileCooperative(t.Context(), jsengine.Wasm)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := m.Instantiate(t.Context(), InstanceConfig{
		MemoryCapBytes: 256 << 20,
		Preload:        &Preload{Export: jsengine.LoadExport, Data: script},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = inst.Close(context.Background()) })
	return e, inst
}

func jsRequest(t *testing.T) []byte {
	t.Helper()
	return request(t, "/x", "", nil)
}

// A call inside its deadline returns normally, and the instance is reusable.
func TestCooperativeCallWithinDeadline(t *testing.T) {
	_, inst := coopInstance(t, jsHandler(jsOK))
	for range 3 {
		ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
		out, err := inst.Handle(ctx, jsRequest(t), nil)
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if r, body := response(t, out); r.Status != 200 || string(body) != "ok" {
			t.Fatalf("response = %d %q", r.Status, body)
		}
	}
	if inst.Broken() {
		t.Error("a normal call broke the instance")
	}
}

// Every shape of runaway stops at its deadline, as a deadline outcome, close to
// on time, with the instance discarded.
func TestCooperativeRunawaysStopAtTheDeadline(t *testing.T) {
	const limit = 250 * time.Millisecond
	cases := map[string]string{
		"empty loop":        `for (;;) {}`,
		"busy arithmetic":   `let x = 0; for (;;) { x = (x * 31 + 7) % 1000003; }`,
		"swallowed by try":  `for (;;) { try { for (;;) {} } catch (e) {} }`,
		"swallowed finally": `for (;;) { try { for (;;) {} } finally {} }`,
		"regexp backtrack":  `return { meta: '{"status":200}', body: String(/(a+)+b/.test("a".repeat(48))) };`,
		"regexp in a loop":  `for (;;) { /(x+x+)+y/.test("x".repeat(40)); }`,
		"microtask loop":    `return (async () => { for (;;) { await null; } })();`,
		"promise chain":     `const spin = () => Promise.resolve().then(spin); return spin();`,
		"sort comparator":   `const a = Array.from({length: 1e5}, (_, i) => i); for (;;) { a.sort(() => Math.random() - 0.5); }`,
		"string building":   `let s = ""; for (;;) { s += "x"; if (s.length > 1e6) s = ""; }`,
		"json churn":        `const o = {a: [1,2,3], b: {c: "d"}}; for (;;) { JSON.parse(JSON.stringify(o)); }`,
		"bigint":            `let n = 3n; for (;;) { n = n * n; if (n.toString(2).length > 1e5) n = 3n; }`,
		// Fills that fit the 256 MiB cap, repeated: one of 2e7 runs out of memory
		// at about the limit, so the case raced the out-of-memory failure (a 500,
		// no error) against the deadline.
		"large array fill": `for (;;) { new Array(2e6).fill(1).reduce((a, b) => a + b); }`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e, inst := coopInstance(t, jsHandler(body))
			ctx, cancel := context.WithTimeout(t.Context(), limit)
			defer cancel()
			start := time.Now()
			_, err := inst.Handle(ctx, jsRequest(t), nil)
			elapsed := time.Since(start)
			if !errors.Is(err, ErrDeadline) {
				t.Fatalf("err = %v after %v, want a deadline outcome", err, elapsed)
			}
			t.Logf("stopped after %v (limit %v), stuck=%v", elapsed.Round(time.Millisecond), limit, errors.Is(err, ErrStuck))
			if elapsed > limit+stuckGrace+time.Second {
				t.Errorf("took %v", elapsed)
			}
			if !inst.Broken() {
				t.Error("an instance that overran its deadline was left reusable")
			}
			if e.Stuck() != 0 && !errors.Is(err, ErrStuck) {
				t.Errorf("stuck = %d without ErrStuck", e.Stuck())
			}
		})
	}
}

// Unbounded recursion does not wait for the deadline: the guest's stack
// overflows into a trap within milliseconds (QuickJS's own stack limit is not
// effective in this build, so the script cannot catch it; this is the same on
// the preemptible runtime). The call fails and the instance is discarded.
func TestCooperativeRecursionTraps(t *testing.T) {
	_, inst := coopInstance(t, jsHandler(`const f = (n) => f(n + 1) + 1; for (;;) { try { f(0); } catch (e) {} }`))
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	start := time.Now()
	_, err := inst.Handle(ctx, jsRequest(t), nil)
	if !errors.Is(err, ErrTrap) && !errors.Is(err, ErrDeadline) {
		t.Fatalf("err = %v, want a trap or a deadline", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("recursion ran for %v", el)
	}
	if !inst.Broken() {
		t.Error("the instance was left reusable")
	}
}

// A deadline that has already passed never starts the guest.
func TestCooperativeExpiredContextDoesNotRun(t *testing.T) {
	_, inst := coopInstance(t, jsHandler(jsOK))
	ctx, cancel := context.WithDeadline(t.Context(), time.Now().Add(-time.Second))
	defer cancel()
	if _, err := inst.Handle(ctx, jsRequest(t), nil); !errors.Is(err, ErrDeadline) {
		t.Fatalf("err = %v", err)
	}
}

// The preemptible runtime still preempts: Wasm functions are not affected by
// the cooperative runtime existing.
func TestWasmModulesStayOnThePreemptibleRuntime(t *testing.T) {
	b, _ := budget.New(64<<20, 0)
	e, err := New(t.Context(), Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	m, err := e.Compile(t.Context(), jsengine.Wasm)
	if err != nil {
		t.Fatal(err)
	}
	if m.coop || m.rt != e.rt {
		t.Fatal("a module compiled with Compile must be on the preemptible runtime")
	}
	defer func() { _ = m.Close(context.Background()) }()
	inst, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 256 << 20})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inst.Close(context.Background()) }()
	if inst.deadline != nil {
		t.Error("a Wasm-runtime instance has a cooperative deadline hook")
	}
}

// A module without fc_js_deadline cannot be loaded cooperatively: nothing else
// could stop it.
func TestCooperativeModuleMustExportTheDeadline(t *testing.T) {
	b, _ := budget.New(64<<20, 0)
	e, err := New(t.Context(), Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	// The fixture is a valid ABI v1 guest without the export.
	_, err = e.CompileCooperative(t.Context(), fixture)
	var le *LoadError
	if !errors.As(err, &le) || le.Code != LoadExportMissing {
		t.Fatalf("err = %v, want EXPORT_MISSING", err)
	}
}

//go:embed testdata/stuck.wasm
var stuckGuest []byte

// A guest that ignores its deadline is abandoned after the grace: the caller
// gets an answer (ErrStuck, which is an ErrDeadline), the engine counts it,
// the instance is not closed under the running call, and when the call finally
// returns the count drops and the instance is closed.
func TestCooperativeWatchdogAbandonsAGuestThatIgnoresItsDeadline(t *testing.T) {
	old := stuckGrace
	stuckGrace = 50 * time.Millisecond
	t.Cleanup(func() { stuckGrace = old })

	e := newEngine(t, 64*mib)
	m, err := e.CompileCooperative(t.Context(), stuckGuest)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 16 * mib})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err = inst.Handle(ctx, request(t, "/x", "", nil), nil)
	if !errors.Is(err, ErrStuck) || !errors.Is(err, ErrDeadline) {
		t.Fatalf("err = %v, want ErrStuck (and ErrDeadline)", err)
	}
	if el := time.Since(start); el < 140*time.Millisecond || el > time.Second {
		t.Errorf("abandoned after %v, want deadline + grace (~150ms)", el)
	}
	if e.Stuck() != 1 || !e.Healthy() {
		t.Errorf("stuck = %d, healthy = %v", e.Stuck(), e.Healthy())
	}
	if err := inst.Close(t.Context()); err != nil {
		t.Errorf("closing a stuck instance: %v", err)
	}
	// Release the guest; its goroutine must settle the count and close it.
	inst.mod.Memory().WriteByte(0, 1)
	deadline := time.Now().Add(5 * time.Second)
	for e.Stuck() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if e.Stuck() != 0 {
		t.Fatalf("stuck = %d after the guest returned", e.Stuck())
	}
}

func TestEngineIsUnhealthyAtTheStuckLimit(t *testing.T) {
	e := newEngine(t, 64*mib)
	e.stuck.Store(StuckLimit - 1)
	if !e.Healthy() {
		t.Error("unhealthy below the limit")
	}
	e.stuck.Store(StuckLimit)
	if e.Healthy() {
		t.Error("healthy at the limit")
	}
	e.stuck.Store(0)
}

// The two runtimes share one compilation cache, which is only safe because
// wazero keys it by a module ID that includes whether termination checks are
// compiled in. This pins that: the same bytes compiled cooperatively first
// (unchecked code on disk) must not be what the preemptible runtime runs, also
// after a restart on the same directory. If a wazero upgrade changes it, this
// fails (rather than hangs: the spin is guarded) and the runtimes need separate
// cache directories.
func TestSharedCacheNeverCrossesRuntimes(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		b, _ := budget.New(256<<20, 0)
		e, err := New(t.Context(), Config{Budget: b, CacheDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		// The same bytes on each runtime, the cooperative one first so its
		// unchecked code is what lands in a shared cache, if there were one.
		cm, err := e.CompileCooperative(t.Context(), jsengine.Wasm)
		if err != nil {
			t.Fatal(err)
		}
		pm, err := e.Compile(t.Context(), jsengine.Wasm)
		if err != nil {
			t.Fatal(err)
		}
		pi, err := pm.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 256 << 20, Preload: &Preload{Export: jsengine.LoadExport, Data: jsHandler(`for (;;) {}`)}})
		if err != nil {
			t.Fatal(err)
		}
		// The preemptible runtime must still stop it by itself.
		errc := make(chan error, 1)
		go func() {
			ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
			defer cancel()
			_, err := pi.Handle(ctx, jsRequest(t), nil)
			errc <- err
		}()
		select {
		case err := <-errc:
			if !errors.Is(err, ErrDeadline) {
				t.Fatalf("err = %v, want ErrDeadline", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("the preemptible runtime did not stop a spinning guest: it was served the cooperative runtime's compiled code")
		}
		_ = pi.Close(t.Context())
		_ = cm.Close(t.Context())
		_ = pm.Close(t.Context())
		if err := e.Close(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}
