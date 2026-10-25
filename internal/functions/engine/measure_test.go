package engine

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
)

// TestMeasureGuests measures guest artifacts on the real engine: size, compile
// (cold and from the disk cache), instantiate, first call, warm call and
// memory. Opt-in; the numbers are for comparing guest toolchains on one
// machine, not absolute capacity (docs/function-examples-go-rust.md):
//
//	FN_MEASURE="label=scenario=/path/a.wasm,label=scenario=/path/b.wasm" \
//	  go test ./internal/functions/engine -run TestMeasureGuests -v
//
// Scenarios: health (GET /healthz), ingest (POST /ingest, HMAC-signed JSON,
// then a dispatch host call), creator (POST /dispatch as a principal holding
// dispatchdemo:dispatch:job:create, then a dispatch host call). Every scenario
// asserts the expected status, so a guest that misbehaves fails, not just slows.
func TestMeasureGuests(t *testing.T) {
	spec := os.Getenv("FN_MEASURE")
	if spec == "" {
		t.Skip("FN_MEASURE not set")
	}
	fmt.Println("| guest | scenario | size | compile cold | compile cached | instantiate p50 / p99 | first call | warm p50 / p99 | min mem | mem after 5000 calls |")
	fmt.Println("|---|---|---|---|---|---|---|---|---|---|")
	for item := range strings.SplitSeq(spec, ",") {
		parts := strings.SplitN(item, "=", 3)
		if len(parts) != 3 {
			t.Fatalf("bad FN_MEASURE item %q", item)
		}
		measure(t, parts[0], parts[1], parts[2])
	}
}

const measureSecret = "s3cr3t"

type scenario struct {
	req    abi.Request
	body   []byte
	status int
}

func scenarioFor(t *testing.T, name string) scenario {
	switch name {
	case "health":
		return scenario{req: abi.Request{Method: "GET", Path: "/healthz"}, status: 200}
	case "ingest":
		body := []byte(`{"order":42,"note":"hello","lines":[{"sku":"a1","qty":2},{"sku":"b7","qty":1}]}`)
		mac := hmac.New(sha256.New, []byte(measureSecret))
		mac.Write(body)
		return scenario{req: abi.Request{
			Method: "POST", Path: "/ingest",
			Headers: map[string][]string{"X-Signature": {"sha256=" + hex.EncodeToString(mac.Sum(nil))}},
		}, body: body, status: 202}
	case "creator":
		return scenario{
			req: abi.Request{
				Method: "POST", Path: "/dispatch",
				Caller: abi.Caller{Kind: abi.CallerPrincipal, ID: "prn_1", Permissions: []string{"dispatchdemo:dispatch:job:create"}},
			},
			body: []byte(`{"order":42,"note":"hello"}`), status: 202,
		}
	}
	t.Fatalf("unknown scenario %q", name)
	return scenario{}
}

type fakeHost struct{}

func (fakeHost) Call(_ context.Context, op abi.Op, _, _ []byte) ([]byte, []byte, *abi.Error) {
	switch op {
	case abi.OpSecretGet:
		return []byte(`{"found":true}`), []byte(measureSecret), nil
	case abi.OpDispatch:
		return []byte(`{"jobId":"djob_1"}`), nil, nil
	case abi.OpLog, abi.OpConfigGet, abi.OpHTTPFetch, abi.OpEventEmit,
		abi.OpDBQuery, abi.OpDBExec, abi.OpDBBegin, abi.OpDBCommit, abi.OpDBRollback,
		abi.OpTrace, abi.OpTracing, abi.OpTraceReference:
		// The measured fixtures do not need these answered.
	}
	return nil, nil, nil
}

func pct(d []time.Duration, p float64) time.Duration {
	s := slices.Clone(d)
	slices.Sort(s)
	return s[int(float64(len(s)-1)*p)]
}

func us(d time.Duration) string {
	if d >= 10*time.Millisecond {
		return fmt.Sprintf("%.0f ms", float64(d)/1e6)
	}
	if d >= time.Millisecond {
		return fmt.Sprintf("%.1f ms", float64(d)/1e6)
	}
	return fmt.Sprintf("%.0f µs", float64(d)/1e3)
}

func measure(t *testing.T, label, scen, path string) {
	t.Helper()
	ctx := context.Background()
	wasm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	newEng := func(cacheDir string) *Engine {
		b, _ := budget.New(1<<30, 0)
		e, err := New(ctx, Config{Budget: b, CacheDir: cacheDir})
		if err != nil {
			t.Fatal(err)
		}
		return e
	}

	var cold []time.Duration
	for range 3 {
		e := newEng("")
		s := time.Now()
		m, err := e.Compile(ctx, wasm)
		if err != nil {
			t.Fatalf("%s: compile: %v", label, err)
		}
		cold = append(cold, time.Since(s))
		_ = m.Close(ctx)
		_ = e.Close(ctx)
	}

	dir := t.TempDir()
	e0 := newEng(dir)
	m0, err := e0.Compile(ctx, wasm)
	if err != nil {
		t.Fatal(err)
	}
	_ = m0.Close(ctx)
	_ = e0.Close(ctx)
	var cached []time.Duration
	var e *Engine
	var m *Module
	for range 3 {
		if e != nil {
			_ = m.Close(ctx)
			_ = e.Close(ctx)
		}
		e = newEng(dir)
		s := time.Now()
		if m, err = e.Compile(ctx, wasm); err != nil {
			t.Fatal(err)
		}
		cached = append(cached, time.Since(s))
	}
	defer func() { _ = e.Close(ctx) }()

	cfg := InstanceConfig{MemoryCapBytes: 256 << 20}
	var inst []time.Duration
	for range 200 {
		s := time.Now()
		i, err := m.Instantiate(ctx, cfg)
		if err != nil {
			t.Fatalf("%s: instantiate: %v", label, err)
		}
		inst = append(inst, time.Since(s))
		_ = i.Close(ctx)
	}

	sc := scenarioFor(t, scen)
	call := func(i *Instance) {
		r := sc.req
		r.ID, r.Address, r.Version = "inv_1", "x.y", 1
		r.DeadlineUnixMs = time.Now().Add(time.Minute).UnixMilli()
		if r.Caller.Kind == "" {
			r.Caller.Kind = abi.CallerAnonymous
		}
		frame, err := abi.MarshalFrame(r, sc.body)
		if err != nil {
			t.Fatal(err)
		}
		out, err := i.Handle(ctx, frame, fakeHost{})
		if err != nil {
			t.Fatalf("%s: handle: %v", label, err)
		}
		var rm abi.Response
		if _, err := abi.UnmarshalFrame(out, &rm); err != nil || rm.Status != sc.status {
			t.Fatalf("%s: status %d (want %d) err %v frame %q", label, rm.Status, sc.status, err, out)
		}
	}
	i, err := m.Instantiate(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = i.Close(ctx) }()
	s := time.Now()
	call(i)
	first := time.Since(s)
	for range 500 {
		call(i)
	}
	var warm []time.Duration
	for range 5000 {
		s := time.Now()
		call(i)
		warm = append(warm, time.Since(s))
	}
	fmt.Printf("| %s | %s | %.2f MB | %s | %s | %s / %s | %s | %s / %s | %.1f MB | %.1f MB |\n",
		label, scen, float64(len(wasm))/1e6, us(pct(cold, .5)), us(pct(cached, .5)),
		us(pct(inst, .5)), us(pct(inst, .99)), us(first), us(pct(warm, .5)), us(pct(warm, .99)),
		float64(m.MinMemoryBytes())/(1<<20), float64(i.MemoryBytes())/(1<<20))
}
