package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// TestSDKExamples runs the guest SDKs' example artifacts (a .js path is a
// script for the shared JS engine) through the real
// engine and runner: describe exactly as publish reads it, then HTTP calls
// through every auth mode. The examples are build outputs, so the test runs
// only when pointed at them:
//
//	make -C clients/fn-go build-hello
//	cargo build --release --target wasm32-unknown-unknown --manifest-path clients/fn-rust/examples/hello/Cargo.toml
//	FN_EXAMPLE_WASM=$PWD/clients/fn-go/examples/hello/hello.wasm,$PWD/clients/fn-rust/examples/hello/target/wasm32-unknown-unknown/release/hello.wasm \
//	  go test ./internal/functions/runner -run TestSDKExamples -v
func TestSDKExamples(t *testing.T) {
	paths := os.Getenv("FN_EXAMPLE_WASM")
	if paths == "" {
		t.Skip("FN_EXAMPLE_WASM not set")
	}
	for p := range strings.SplitSeq(paths, ",") {
		t.Run(p[strings.LastIndex(p, "/clients/")+1:], func(t *testing.T) { runExample(t, p) })
	}
}

func runExample(t *testing.T, path string) {
	wasm, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(wasm)
	digest := hex.EncodeToString(sum[:])

	// A .js artifact runs on the shared JS engine; anything else is a module.
	runtime := runtimes.Wasm
	if strings.HasSuffix(path, ".js") {
		runtime = runtimes.JS
	}

	// Describe, as publish will: an instance with no capabilities.
	b, _ := budget.New(512<<20, 0)
	e, err := engine.New(t.Context(), engine.Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	start := time.Now()
	doc, err := runtimes.NewLoader(e).Describe(t.Context(), runtime, wasm, 64<<20)
	if err != nil {
		t.Fatalf("describe: %v", err)
	}
	t.Logf("%s, %d bytes, prepared+described in %v, describe %s", runtime, len(wasm), time.Since(start), doc)
	d, err := abi.ParseDescribe(doc)
	if err != nil {
		t.Fatalf("the SDK's describe does not validate: %v", err)
	}

	cp := newFakeCP()
	cp.artifacts[digest] = wasm
	clt := "clt_1"
	cp.push(control.Desired{Pool: "default", Functions: []control.Function{{
		ID: "fnc_ex", Address: "hello.example", ApplicationID: "app_1", ClientID: &clt,
		Limits:        control.Limits{MemoryMB: 64, MaxConcurrency: 4, TimeoutMs: 5000},
		WebhookSecret: secret,
		Config:        map[string]string{"GREETING": "Kia ora"},
		Versions:      []control.Version{{Number: 1, Digest: digest, Runtime: runtime, ABI: 1, Describe: doc, Roles: []string{control.RoleLive}}},
	}}})
	r, err := New(t.Context(), Config{Pool: "default", ControlPlane: cp, Budget: b, Tokens: fakeTokens{}})
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, cp: cp, r: r}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	h.srv = newTestServer(t, r)
	h.waitServing("hello.example", 1)

	for _, ep := range d.Endpoints {
		t.Logf("endpoint %s auth=%s", ep.Pattern(), ep.Auth)
	}
	if resp, body := h.do("GET", "/fn/hello.example/healthz", nil, nil); resp.StatusCode != 200 {
		t.Errorf("healthz: %d %s", resp.StatusCode, body)
	}
	resp, body := h.do("GET", "/fn/hello.example/hello/Aroha", nil, map[string]string{"Authorization": "Bearer plain"})
	if resp.StatusCode != 200 || !strings.Contains(string(body), "Kia ora") || !strings.Contains(string(body), "Aroha") {
		t.Errorf("hello: %d %s", resp.StatusCode, body)
	}
	if resp, _ := h.do("GET", "/fn/hello.example/hello/Aroha", nil, nil); resp.StatusCode != 401 {
		t.Errorf("hello without a token: %d", resp.StatusCode)
	}
	payload := []byte(`{"id":"evt_1","type":"hello.example:greeting:greeting:requested","data":{"name":"Aroha"}}`)
	resp, body = h.do("POST", "/fn/hello.example/events/greeting", payload, signed(payload))
	if resp.StatusCode/100 != 2 {
		t.Errorf("webhook: %d %s", resp.StatusCode, body)
	}
	cp.mu.Lock()
	emits := len(cp.emits)
	cp.mu.Unlock()
	if len(d.Emits) > 0 && emits == 0 {
		t.Errorf("the webhook handler emitted nothing")
	}
	checkDispatch(t, h, cp, d)
	var lat []time.Duration
	for range 200 {
		s := time.Now()
		h.do("GET", "/fn/hello.example/healthz", nil, nil)
		lat = append(lat, time.Since(s))
	}
	t.Logf("healthz over HTTP: first %v, median of 200 %v", lat[0], median(lat))
	_ = json.Valid
}

// checkDispatch proves the SDK's dispatch capability through the runner: the
// example declares dispatches (describe validated above), POST /dispatch calls
// fn.dispatch, the platform is asked exactly once with the declared target and
// path, the payload as the body and the metadata as an ARRAY of {key,value}
// whatever shape the SDK took it in, and a platform refusal reaches the
// handler as an error (not a trap).
func checkDispatch(t *testing.T, h *harness, cp *fakeCP, d *abi.Describe) {
	t.Helper()
	if len(d.Dispatches) == 0 {
		t.Errorf("the example declares no dispatches")
		return
	}
	resp, body := h.do("POST", "/fn/hello.example/dispatch", nil, nil)
	if resp.StatusCode != 202 || !strings.Contains(string(body), "djob_1") {
		t.Errorf("dispatch: %d %s", resp.StatusCode, body)
	}
	cp.mu.Lock()
	got := append([]control.DispatchRequest(nil), cp.dispatches...)
	cp.mu.Unlock()
	if len(got) != 1 {
		t.Fatalf("the platform was asked to dispatch %d times, want 1: %+v", len(got), got)
	}
	g := got[0]
	if g.FunctionID != "fnc_ex" || g.Version != 1 || g.InvocationID == "" ||
		g.Dispatch.Target != "hello.logger" || g.Dispatch.Path != "/deliver" || g.Dispatch.MessageGroup != "hello" ||
		string(g.Payload) != `{"hello":"world"}` {
		t.Errorf("dispatch request = %+v payload %q", g, g.Payload)
	}
	if len(g.Dispatch.Metadata) != 1 || g.Dispatch.Metadata[0] != (abi.KV{Key: "from", Value: "hello"}) {
		t.Errorf("metadata = %+v, want [{from hello}]", g.Dispatch.Metadata)
	}

	// A refusal is an error the handler maps to a reject (422) or a retry
	// (429), never a trap (500).
	for _, c := range []struct {
		err  *abi.Error
		want int
	}{
		{abi.Errorf(abi.CodeNotAllowed, "no"), 422},
		{abi.Errorf(abi.CodeUnavailable, "down"), 429},
	} {
		cp.mu.Lock()
		cp.dispatchErr = c.err
		cp.mu.Unlock()
		if resp, body := h.do("POST", "/fn/hello.example/dispatch", nil, nil); resp.StatusCode != c.want {
			t.Errorf("dispatch refused with %s: %d %s, want %d", c.err.Code, resp.StatusCode, body, c.want)
		}
	}
	cp.mu.Lock()
	cp.dispatchErr = nil
	cp.mu.Unlock()
}

func median(d []time.Duration) time.Duration {
	s := append([]time.Duration(nil), d...)
	for i := range s {
		for j := i + 1; j < len(s); j++ {
			if s[j] < s[i] {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
	return s[len(s)/2]
}
