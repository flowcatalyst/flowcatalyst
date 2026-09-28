package engine

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
)

var fixture = fnfixture.Wasm

const mib = 1 << 20

func newEngine(t *testing.T, budgetBytes int64) *Engine {
	t.Helper()
	b, err := budget.New(budgetBytes, 0)
	if err != nil {
		t.Fatal(err)
	}
	e, err := New(t.Context(), Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Close(context.Background()) })
	return e
}

func compile(t *testing.T, e *Engine) *Module {
	t.Helper()
	m, err := e.Compile(t.Context(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func instance(t *testing.T, m *Module, capBytes uint64) *Instance {
	t.Helper()
	i, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: capBytes})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i.Close(context.Background()) })
	return i
}

func request(t *testing.T, path, query string, body []byte) []byte {
	t.Helper()
	f, err := abi.MarshalFrame(abi.Request{ID: "inv-1", Method: "POST", Path: path, RawQuery: query, Caller: abi.Caller{Kind: abi.CallerAnonymous}}, body)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func response(t *testing.T, frame []byte) (abi.Response, []byte) {
	t.Helper()
	var r abi.Response
	body, err := abi.UnmarshalFrame(frame, &r)
	if err != nil {
		t.Fatal(err)
	}
	return r, body
}

func TestHandleEcho(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	out, err := i.Handle(t.Context(), request(t, "/echo", "", []byte("raw\x00bytes")), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, body := response(t, out)
	if r.Status != 200 || string(body) != "raw\x00bytes" || r.Headers["x-path"][0] != "/echo" {
		t.Fatalf("response %+v body %q", r, body)
	}
}

func TestDescribe(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	doc, err := i.Describe(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	d, err := abi.ParseDescribe(doc)
	if err != nil {
		t.Fatal(err)
	}
	if d.Config[0] != "GREETING" {
		t.Fatalf("describe = %s", doc)
	}
}

func TestStatePersistsAcrossCallsOnOneInstance(t *testing.T) {
	m := compile(t, newEngine(t, 256*mib))
	a, b := instance(t, m, 64*mib), instance(t, m, 64*mib)
	for want := 1; want <= 3; want++ {
		_, body := response(t, must(a.Handle(t.Context(), request(t, "/count", "", nil), nil)))
		if string(body) != strconv.Itoa(want) {
			t.Fatalf("call %d on instance a counted %s", want, body)
		}
	}
	_, body := response(t, must(b.Handle(t.Context(), request(t, "/count", "", nil), nil)))
	if string(body) != "1" {
		t.Fatalf("a second instance shares state: counted %s", body)
	}
	if a.Calls() != 3 {
		t.Fatalf("calls = %d", a.Calls())
	}
}

func TestDeadlinePreemptsSpinningGuest(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := i.Handle(ctx, request(t, "/spin", "", nil), nil)
	if !errors.Is(err, ErrDeadline) {
		t.Fatalf("err = %v, want ErrDeadline", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("spin stopped after %v", el)
	}
	if !i.Broken() {
		t.Fatal("instance not marked broken after a deadline")
	}
}

func TestTrap(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	_, err := i.Handle(t.Context(), request(t, "/trap", "", nil), nil)
	if !errors.Is(err, ErrTrap) || !i.Broken() {
		t.Fatalf("err = %v broken = %v", err, i.Broken())
	}
	if _, err := i.Handle(t.Context(), request(t, "/echo", "", nil), nil); err == nil {
		t.Fatal("a broken instance served another call")
	}
}

func TestMalformedResponse(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	if _, err := i.Handle(t.Context(), request(t, "/malformed", "", nil), nil); !errors.Is(err, ErrMalformed) {
		t.Fatalf("err = %v", err)
	}
}

func TestMemoryCapFailsOnlyTheCall(t *testing.T) {
	e := newEngine(t, 256*mib)
	i := instance(t, compile(t, e), 8*mib)
	if _, err := i.Handle(t.Context(), request(t, "/grow", "mb=2", nil), nil); err != nil {
		t.Fatalf("a grow within the cap failed: %v", err)
	}
	_, err := i.Handle(t.Context(), request(t, "/grow", "mb=16", nil), nil)
	if !errors.Is(err, ErrOutOfMemory) {
		t.Fatalf("err = %v, want ErrOutOfMemory", err)
	}
	used := e.Budget().Stats().UsedBytes
	if err := i.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if after := e.Budget().Stats().UsedBytes; after != 0 {
		t.Fatalf("budget not returned on close: %d → %d", used, after)
	}
}

func TestSharedBudgetRefusesGrowAndAdmission(t *testing.T) {
	e := newEngine(t, 24*mib)
	m := compile(t, e)
	a := instance(t, m, 64*mib)
	if _, err := a.Handle(t.Context(), request(t, "/grow", "mb=16", nil), nil); err != nil {
		t.Fatalf("first instance within budget: %v", err)
	}
	b := instance(t, m, 64*mib)
	if _, err := b.Handle(t.Context(), request(t, "/grow", "mb=16", nil), nil); !errors.Is(err, ErrOutOfMemory) {
		t.Fatalf("second instance over the shared budget: err = %v", err)
	}
	// a is unaffected by b's failure.
	if _, err := a.Handle(t.Context(), request(t, "/echo", "", nil), nil); err != nil {
		t.Fatalf("neighbour broken by another instance's OOM: %v", err)
	}
	// Exhaust admission: keep instantiating until the budget refuses.
	var held []*Instance
	defer func() {
		for _, h := range held {
			_ = h.Close(context.Background())
		}
	}()
	for range 100 {
		i, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 64 * mib})
		if errors.Is(err, ErrNoMemory) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, i)
	}
	t.Fatal("admission never refused an instance")
}

func TestMinimumOverCapRefused(t *testing.T) {
	m := compile(t, newEngine(t, 256*mib))
	_, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 64 << 10})
	var le *LoadError
	if !errors.As(err, &le) || le.Code != LoadMemoryOverCap {
		t.Fatalf("err = %v", err)
	}
}

func hostCallFrame(t *testing.T, meta any, body string) []byte {
	t.Helper()
	f, err := abi.MarshalFrame(meta, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestHostCall(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	var gotOp abi.Op
	var gotKey abi.Key
	host := HostFunc(func(ctx context.Context, op abi.Op, meta, body []byte) ([]byte, []byte, *abi.Error) {
		gotOp = op
		_ = json.Unmarshal(meta, &gotKey)
		if gotKey.Key == "MISSING" {
			return nil, nil, abi.Errorf(abi.CodeNotDeclared, "MISSING is not declared")
		}
		return []byte(`{"found":true}`), []byte("hello " + string(body)), nil
	})
	out, err := i.Handle(t.Context(), request(t, "/call", "op=2", hostCallFrame(t, abi.Key{Key: "GREETING"}, "x")), host)
	if err != nil {
		t.Fatal(err)
	}
	r, body := response(t, out)
	if gotOp != abi.OpConfigGet || gotKey.Key != "GREETING" {
		t.Fatalf("host saw op %v key %+v", gotOp, gotKey)
	}
	if r.Headers["x-code"][0] != "0" || r.Headers["x-meta"][0] != `{"found":true}` || string(body) != "hello x" {
		t.Fatalf("guest saw %+v body %q", r.Headers, body)
	}

	out, err = i.Handle(t.Context(), request(t, "/call", "op=2", hostCallFrame(t, abi.Key{Key: "MISSING"}, "")), host)
	if err != nil {
		t.Fatal(err)
	}
	r, _ = response(t, out)
	if r.Headers["x-code"][0] != "1" || !strings.Contains(r.Headers["x-meta"][0], abi.CodeNotDeclared) {
		t.Fatalf("error not delivered as an error frame: %+v", r.Headers)
	}
}

func TestHostCallWithoutCapabilities(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	out, err := i.Handle(t.Context(), request(t, "/call", "op=3", hostCallFrame(t, abi.Key{Key: "K"}, "")), nil)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := response(t, out)
	if r.Headers["x-code"][0] != "1" || !strings.Contains(r.Headers["x-meta"][0], abi.CodeCapabilityUnavailable) {
		t.Fatalf("headers %+v", r.Headers)
	}
}

func TestHostPanicIsUnavailableNotTrap(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	host := HostFunc(func(context.Context, abi.Op, []byte, []byte) ([]byte, []byte, *abi.Error) { panic("host bug") })
	out, err := i.Handle(t.Context(), request(t, "/call", "op=1", hostCallFrame(t, abi.Log{Level: "info", Msg: "m"}, "")), host)
	if err != nil {
		t.Fatalf("a host panic trapped the guest: %v", err)
	}
	r, _ := response(t, out)
	if !strings.Contains(r.Headers["x-meta"][0], abi.CodeUnavailable) {
		t.Fatalf("headers %+v", r.Headers)
	}
}

func TestHostCallBadFrame(t *testing.T) {
	i := instance(t, compile(t, newEngine(t, 256*mib)), 64*mib)
	host := HostFunc(func(context.Context, abi.Op, []byte, []byte) ([]byte, []byte, *abi.Error) {
		t.Error("host called with a malformed frame")
		return nil, nil, nil
	})
	out, err := i.Handle(t.Context(), request(t, "/call", "op=2", []byte{0xff, 0, 0, 0}), host)
	if err != nil {
		t.Fatal(err)
	}
	r, _ := response(t, out)
	if !strings.Contains(r.Headers["x-meta"][0], abi.CodeBadRequest) {
		t.Fatalf("headers %+v", r.Headers)
	}
}

func TestConcurrentInstances(t *testing.T) {
	m := compile(t, newEngine(t, 512*mib))
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			i, err := m.Instantiate(t.Context(), InstanceConfig{MemoryCapBytes: 64 * mib})
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = i.Close(context.Background()) }()
			for range 200 {
				if _, err := i.Handle(t.Context(), request(t, "/echo", "", []byte("x")), nil); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
}

func must(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
