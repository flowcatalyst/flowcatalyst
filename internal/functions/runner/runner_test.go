package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/webhook"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
)

// fakeCP is an in-memory control plane: tests push desired documents and read
// heartbeats and emits.
type fakeCP struct {
	mu         sync.Mutex
	desired    *control.Desired
	rev        int64
	changed    chan struct{}
	artifacts  map[string][]byte
	heartbeats []control.Heartbeat
	emits      []control.EmitRequest
	emitPanics bool // Emit panics (a host-side fault inside a call)
}

func newFakeCP() *fakeCP {
	return &fakeCP{changed: make(chan struct{}, 1), artifacts: map[string][]byte{fnfixture.Digest(): fnfixture.Wasm}}
}

func (f *fakeCP) push(d control.Desired) {
	f.mu.Lock()
	f.rev++
	d.Revision = f.rev
	f.desired = &d
	f.mu.Unlock()
	select {
	case f.changed <- struct{}{}:
	default:
	}
}

func (f *fakeCP) Desired(ctx context.Context, pool, etag string, wait time.Duration) (*control.Desired, string, bool, error) {
	for {
		f.mu.Lock()
		d, rev := f.desired, f.rev
		f.mu.Unlock()
		if d != nil && strconv.FormatInt(rev, 10) != etag {
			return d, strconv.FormatInt(rev, 10), true, nil
		}
		select {
		case <-ctx.Done():
			return nil, "", false, ctx.Err()
		case <-f.changed:
		case <-time.After(wait):
			return nil, etag, false, nil
		}
	}
}

func (f *fakeCP) Heartbeat(_ context.Context, hb control.Heartbeat) error {
	f.mu.Lock()
	f.heartbeats = append(f.heartbeats, hb)
	f.mu.Unlock()
	return nil
}

func (f *fakeCP) Artifact(_ context.Context, digest string) (io.ReadCloser, error) {
	f.mu.Lock()
	b, ok := f.artifacts[digest]
	f.mu.Unlock()
	if !ok {
		return nil, errors.New("no such artifact")
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

func (f *fakeCP) Emit(_ context.Context, r control.EmitRequest) (*control.EmitResponse, *abi.Error) {
	if f.emitPanics {
		panic("emit exploded")
	}
	f.mu.Lock()
	f.emits = append(f.emits, r)
	f.mu.Unlock()
	return &control.EmitResponse{EventID: "evt_1"}, nil
}

func (f *fakeCP) lastHeartbeat() (control.Heartbeat, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.heartbeats) == 0 {
		return control.Heartbeat{}, false
	}
	return f.heartbeats[len(f.heartbeats)-1], true
}

// fakeTokens accepts "Bearer good" (holding version:invoke) and "Bearer plain".
type fakeTokens struct{}

func (fakeTokens) Verify(_ context.Context, bearer string) (*abi.Caller, error) {
	switch bearer {
	case "Bearer good":
		return &abi.Caller{Kind: abi.CallerPrincipal, ID: "prn_1", Tier: "CLIENT", Clients: []string{"clt_1"}, Permissions: []string{"platform:function:*:invoke"}}, nil
	case "Bearer plain":
		return &abi.Caller{Kind: abi.CallerPrincipal, ID: "prn_2", Tier: "CLIENT", Clients: []string{"clt_1"}}, nil
	}
	return nil, errUnauthenticated
}

const describeDoc = `{"abi":1,"endpoints":[
 {"path":"/echo","auth":"none"},
 {"path":"/meta","auth":"none"},
 {"path":"/count","auth":"none"},
 {"method":"POST","path":"/hook","auth":"webhook"},
 {"method":"GET","path":"/secure","auth":"platform"},
 {"path":"/spin","auth":"none","timeoutMs":150},
 {"path":"/grow","auth":"none"},
 {"path":"/trap","auth":"none"},
 {"path":"/call","auth":"none"},
 {"method":"GET","path":"/cors","auth":"none","cors":{"origins":["https://app.acme.com"]}},
 {"method":"POST","path":"/small","auth":"none","maxBodyBytes":4},
 {"method":"POST","path":"/huge","auth":"none","maxBodyBytes":9000000000000},
 {"method":"POST","path":"/secure-post","auth":"platform"}
],"config":["GREETING"],"secrets":["KEY"],"emits":["app:dom:agg:done"],"httpAllow":["example.com"]}`

// The fixture routes by path, so /hook and /secure need fixture paths too:
// they fall through to its 404, which is still a guest answer (not a runner
// refusal) — enough to prove auth let the call through.

const secret = "whsec-test"

func fnDoc(versions ...control.Version) control.Function {
	clt := "clt_1"
	return control.Function{
		ID: "fnc_1", Address: "app.hello", ApplicationID: "app_1", ClientID: &clt,
		Limits:        control.Limits{MemoryMB: 64, MaxConcurrency: 4, TimeoutMs: 5000},
		WebhookSecret: secret,
		Config:        map[string]string{"GREETING": "hi"},
		Secrets:       map[string]string{"KEY": "s3cr3t"},
		Versions:      versions,
	}
}

func ver(n int, roles ...string) control.Version {
	return control.Version{Number: n, Digest: fnfixture.Digest(), ABI: 1, Describe: json.RawMessage(describeDoc), Roles: roles}
}

type harness struct {
	t   *testing.T
	cp  *fakeCP
	r   *Runner
	srv *httptest.Server
}

func start(t *testing.T, fns ...control.Function) *harness {
	t.Helper()
	return startCfg(t, nil, fns...)
}

// startCfg is start with a hook to adjust the runner's Config.
func startCfg(t *testing.T, adjust func(*Config), fns ...control.Function) *harness {
	t.Helper()
	b, err := budget.New(512<<20, 0)
	if err != nil {
		t.Fatal(err)
	}
	cp := newFakeCP()
	cp.push(control.Desired{Pool: "default", Functions: fns})
	cfg := Config{Pool: "default", ControlPlane: cp, Budget: b, HeartbeatEvery: 50 * time.Millisecond, DrainGrace: time.Second, Tokens: fakeTokens{}}
	if adjust != nil {
		adjust(&cfg)
	}
	r, err := New(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	srv := httptest.NewServer(r.Handler())
	t.Cleanup(func() {
		srv.Close()
		cancel()
		<-done
	})
	h := &harness{t: t, cp: cp, r: r, srv: srv}
	h.waitServing("app.hello", 1)
	return h
}

// waitServing waits until address serves version n (or fails the test).
func (h *harness) waitServing(address string, n int) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.r.mu.RLock()
		fn := h.r.functions[address]
		ok := false
		if fn != nil && fn.serving == n {
			if v := fn.versions[n]; v != nil {
				st, _ := v.currentState()
				ok = st == stateReady || st == stateEvicted
			}
		}
		h.r.mu.RUnlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("%s never served version %d", address, n)
}

func (h *harness) do(method, path string, body []byte, hdr map[string]string) (*http.Response, []byte) {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.srv.URL+path, bytes.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp, b
}

func TestInvokeLive(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	resp, body := h.do("POST", "/fn/app.hello/echo", []byte("ping"), nil)
	if resp.StatusCode != 200 || string(body) != "ping" || resp.Header.Get("X-FlowCatalyst-Invocation") == "" {
		t.Fatalf("status %d body %q headers %v", resp.StatusCode, body, resp.Header)
	}
}

func TestRequestMetaReachesGuest(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	resp, body := h.do("GET", "/fn/app.hello/meta?x=1", nil, map[string]string{"X-Custom": "v"})
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	var m abi.Request
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m.Address != "app.hello" || m.Version != 1 || m.Path != "/meta" || m.RawQuery != "x=1" || m.Route != "/meta" ||
		m.Caller.Kind != abi.CallerAnonymous || m.Headers["X-Custom"][0] != "v" || m.DeadlineUnixMs == 0 || m.ID == "" {
		t.Fatalf("meta = %+v", m)
	}
}

func TestMisses(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	cases := []struct {
		method, path string
		want         int
		code         string
	}{
		{"GET", "/fn/nope.nope/echo", 404, "FUNCTION_NOT_FOUND"},
		{"GET", "/fn/app.hello/nowhere", 404, "ROUTE_NOT_FOUND"},
		{"GET", "/fn/app.hello/hook", 405, "METHOD_NOT_ALLOWED"},
		{"GET", "/fn/app.hello@qa/echo", 404, "ALIAS_NOT_FOUND"},
	}
	for _, c := range cases {
		resp, body := h.do(c.method, c.path, nil, nil)
		if resp.StatusCode != c.want || !strings.Contains(string(body), c.code) {
			t.Errorf("%s %s: %d %s, want %d %s", c.method, c.path, resp.StatusCode, body, c.want, c.code)
		}
	}
}

func signed(body []byte) map[string]string {
	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	return map[string]string{
		"X-FlowCatalyst-Signature": webhook.NewValidator(secret).ComputeSignature(ts, body),
		"X-FlowCatalyst-Timestamp": ts,
	}
}

func TestWebhookAuth(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	body := []byte(`{"event":1}`)
	if resp, b := h.do("POST", "/fn/app.hello/hook", body, nil); resp.StatusCode != 401 {
		t.Fatalf("unsigned delivery: %d %s", resp.StatusCode, b)
	}
	bad := signed(body)
	if resp, _ := h.do("POST", "/fn/app.hello/hook", []byte(`{"event":2}`), bad); resp.StatusCode != 401 {
		t.Fatalf("signature over a different body accepted: %d", resp.StatusCode)
	}
	// Signed: the runner lets it through; the fixture has no /hook path, so the
	// guest's own 404 comes back — a guest answer, not a runner refusal.
	resp, b := h.do("POST", "/fn/app.hello/hook", body, signed(body))
	if resp.StatusCode != 404 || string(b) != "no such fixture path" {
		t.Fatalf("signed delivery: %d %s", resp.StatusCode, b)
	}
}

func TestPlatformAuthStripsToken(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	if resp, _ := h.do("GET", "/fn/app.hello/secure", nil, nil); resp.StatusCode != 401 {
		t.Fatalf("no token: %d", resp.StatusCode)
	}
	if resp, _ := h.do("GET", "/fn/app.hello/secure", nil, map[string]string{"Authorization": "Bearer forged"}); resp.StatusCode != 401 {
		t.Fatalf("bad token: %d", resp.StatusCode)
	}
	resp, b := h.do("GET", "/fn/app.hello/secure", nil, map[string]string{"Authorization": "Bearer plain"})
	if resp.StatusCode != 404 || string(b) != "no such fixture path" {
		t.Fatalf("valid token: %d %s", resp.StatusCode, b)
	}
}

func TestExplicitVersionNeedsPermission(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive), ver(2, control.RoleCandidate)))
	if resp, _ := h.do("GET", "/fn/app.hello@v2/meta", nil, nil); resp.StatusCode != 401 {
		t.Fatalf("anonymous versioned call: %d", resp.StatusCode)
	}
	if resp, _ := h.do("GET", "/fn/app.hello@v2/meta", nil, map[string]string{"Authorization": "Bearer plain"}); resp.StatusCode != 403 {
		t.Fatalf("versioned call without version:invoke: %d", resp.StatusCode)
	}
	resp, body := h.do("GET", "/fn/app.hello@v2/meta", nil, map[string]string{"Authorization": "Bearer good"})
	var m abi.Request
	_ = json.Unmarshal(body, &m)
	if resp.StatusCode != 200 || m.Version != 2 || m.Caller.Kind != abi.CallerPrincipal || len(m.Headers["Authorization"]) != 0 {
		t.Fatalf("versioned call: %d %+v", resp.StatusCode, m)
	}
	// The live route still serves v1.
	_, body = h.do("GET", "/fn/app.hello/meta", nil, nil)
	_ = json.Unmarshal(body, &m)
	if m.Version != 1 {
		t.Fatalf("live serves v%d", m.Version)
	}
}

func TestTimeoutAnd504(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	start := time.Now()
	resp, body := h.do("GET", "/fn/app.hello/spin", nil, nil)
	if resp.StatusCode != 504 || !strings.Contains(string(body), "FUNCTION_TIMEOUT") {
		t.Fatalf("spin: %d %s", resp.StatusCode, body)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("504 took %v", el)
	}
	// The runner still serves after a runaway guest.
	if resp, _ := h.do("GET", "/fn/app.hello/echo", nil, nil); resp.StatusCode != 200 {
		t.Fatalf("after timeout: %d", resp.StatusCode)
	}
}

func TestTrapIs500AndDoesNotLeak(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	resp, body := h.do("GET", "/fn/app.hello/trap", nil, nil)
	if resp.StatusCode != 500 || !strings.Contains(string(body), "FUNCTION_FAILED") || strings.Contains(string(body), "fixture trap") {
		t.Fatalf("trap: %d %s", resp.StatusCode, body)
	}
}

func TestConcurrencyLimit(t *testing.T) {
	fn := fnDoc(ver(1, control.RoleLive))
	fn.Limits.MaxConcurrency = 1
	h := start(t, fn)
	var wg sync.WaitGroup
	wg.Go(func() { h.do("GET", "/fn/app.hello/spin", nil, nil) }) // holds the only permit ~150ms
	time.Sleep(50 * time.Millisecond)
	resp, body := h.do("GET", "/fn/app.hello/echo", nil, nil)
	wg.Wait()
	if resp.StatusCode != 429 || resp.Header.Get("Retry-After") == "" || !strings.Contains(string(body), "FUNCTION_BUSY") {
		t.Fatalf("second call at the limit: %d %s", resp.StatusCode, body)
	}
}

func TestBodyLimit(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	if resp, _ := h.do("POST", "/fn/app.hello/small", []byte("12345"), nil); resp.StatusCode != 413 {
		t.Fatalf("over the endpoint body limit: %d", resp.StatusCode)
	}
}

func TestCORSPreflight(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	resp, _ := h.do("OPTIONS", "/fn/app.hello/cors", nil, map[string]string{"Origin": "https://app.acme.com", "Access-Control-Request-Method": "GET"})
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "https://app.acme.com" {
		t.Fatalf("preflight: %d %v", resp.StatusCode, resp.Header)
	}
	resp, _ = h.do("OPTIONS", "/fn/app.hello/cors", nil, map[string]string{"Origin": "https://evil.com", "Access-Control-Request-Method": "GET"})
	if resp.StatusCode != 403 {
		t.Fatalf("preflight from a foreign origin: %d", resp.StatusCode)
	}
}

func hostCall(t *testing.T, h *harness, op abi.Op, meta any, body string) (code string, respMeta string, respBody string) {
	t.Helper()
	frame, err := abi.MarshalFrame(meta, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, b := h.do("POST", "/fn/app.hello/call?op="+strconv.Itoa(int(op)), frame, nil)
	if resp.StatusCode != 200 {
		t.Fatalf("host call %s: %d %s", op, resp.StatusCode, b)
	}
	return resp.Header.Get("X-Code"), resp.Header.Get("X-Meta"), string(b)
}

func TestCapabilities(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	if code, meta, body := hostCall(t, h, abi.OpConfigGet, abi.Key{Key: "GREETING"}, ""); code != "0" || meta != `{"found":true}` || body != "hi" {
		t.Errorf("config: %s %s %q", code, meta, body)
	}
	if code, meta, _ := hostCall(t, h, abi.OpConfigGet, abi.Key{Key: "OTHER"}, ""); code != "1" || !strings.Contains(meta, abi.CodeNotDeclared) {
		t.Errorf("undeclared config: %s %s", code, meta)
	}
	if code, _, body := hostCall(t, h, abi.OpSecretGet, abi.Key{Key: "KEY"}, ""); code != "0" || body != "s3cr3t" {
		t.Errorf("secret: %s %q", code, body)
	}
	if code, meta, _ := hostCall(t, h, abi.OpEventEmit, abi.Event{Type: "app:dom:agg:done", DedupID: "d1"}, `{"x":1}`); code != "0" || meta != `{"eventId":"evt_1"}` {
		t.Errorf("emit: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpEventEmit, abi.Event{Type: "app:dom:agg:other", DedupID: "d2"}, ""); code != "1" || !strings.Contains(meta, abi.CodeNotDeclared) {
		t.Errorf("undeclared emit: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpHTTPFetch, abi.HTTPRequest{URL: "https://evil.com/x"}, ""); code != "1" || !strings.Contains(meta, abi.CodeNotAllowed) {
		t.Errorf("http outside httpAllow: %s %s", code, meta)
	}
	if code, meta, _ := hostCall(t, h, abi.OpDBQuery, abi.DBStatement{DB: "main", SQL: "select 1"}, ""); code != "1" || !strings.Contains(meta, abi.CodeNotDeclared) {
		t.Errorf("undeclared db: %s %s", code, meta)
	}
	h.cp.mu.Lock()
	defer h.cp.mu.Unlock()
	if len(h.cp.emits) != 1 || h.cp.emits[0].FunctionID != "fnc_1" || h.cp.emits[0].Event.Source != "function:app.hello" || string(h.cp.emits[0].Data) != `{"x":1}` {
		t.Fatalf("emits = %+v", h.cp.emits)
	}
}

func TestPromoteSwapsWithoutFailedCalls(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	stop := make(chan struct{})
	var failures, calls int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				resp, _ := h.do("GET", "/fn/app.hello/echo", nil, nil)
				mu.Lock()
				calls++
				if resp.StatusCode != 200 {
					failures++
				}
				mu.Unlock()
			}
		})
	}
	time.Sleep(50 * time.Millisecond)
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(ver(2, control.RoleLive))}})
	h.waitServing("app.hello", 2)
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	if failures > 0 {
		t.Fatalf("%d of %d calls failed across the swap", failures, calls)
	}
	h.r.mu.RLock()
	_, oldStill := h.r.functions["app.hello"].versions[1]
	h.r.mu.RUnlock()
	if oldStill {
		t.Fatal("the replaced version was not unloaded")
	}
}

func TestFailedVersionKeepsOldServingAndReports(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	bad := ver(2, control.RoleLive)
	bad.Digest = strings.Repeat("ab", 32) // no such artifact
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{fnDoc(bad)}})
	deadline := time.Now().Add(5 * time.Second)
	for {
		hb, ok := h.cp.lastHeartbeat()
		failed := false
		for _, v := range hb.Versions {
			if v.Number == 2 && v.State == control.StateFailed && strings.Contains(v.Reason, "ARTIFACT") {
				failed = true
			}
		}
		if ok && failed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no FAILED heartbeat for v2; last %+v", hb)
		}
		time.Sleep(20 * time.Millisecond)
	}
	resp, body := h.do("GET", "/fn/app.hello/meta", nil, nil)
	var m abi.Request
	_ = json.Unmarshal(body, &m)
	if resp.StatusCode != 200 || m.Version != 1 {
		t.Fatalf("old version stopped serving: %d v%d", resp.StatusCode, m.Version)
	}
}

func TestHeartbeatReportsLoaded(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive), ver(2, control.RoleCandidate)))
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		hb, _ := h.cp.lastHeartbeat()
		loaded := 0
		for _, v := range hb.Versions {
			if v.State == control.StateLoaded {
				loaded++
			}
		}
		if loaded == 2 && hb.Revision > 0 && hb.Budget.BudgetBytes == 512<<20 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	hb, _ := h.cp.lastHeartbeat()
	t.Fatalf("heartbeat never reported both versions loaded: %+v", hb)
}

func TestRemovedFunctionIsUnloaded(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	h.cp.push(control.Desired{Pool: "default"})
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if resp, _ := h.do("GET", "/fn/app.hello/echo", nil, nil); resp.StatusCode == 404 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a function removed from desired state still answers")
}

func TestHasPermission(t *testing.T) {
	cases := []struct {
		held []string
		req  string
		want bool
	}{
		{[]string{"platform:function:version:invoke"}, "platform:function:version:invoke", true},
		{[]string{"platform:function:*:invoke"}, "platform:function:version:invoke", true},
		{[]string{"platform:*"}, "platform:function:version:invoke", false},
		{[]string{"platform:function:version:view"}, "platform:function:version:invoke", false},
	}
	for _, c := range cases {
		if got := hasPermission(c.held, c.req); got != c.want {
			t.Errorf("hasPermission(%v, %s) = %v", c.held, c.req, got)
		}
	}
}

func TestParseTarget(t *testing.T) {
	for in, want := range map[string]target{
		"app.fn":     {address: "app.fn"},
		"app.fn@qa":  {address: "app.fn", alias: "qa"},
		"app.fn@v12": {address: "app.fn", version: 12},
		"app.fn@vx":  {address: "app.fn", alias: "vx"},
	} {
		got, ok := parseTarget(in)
		if !ok || got != want {
			t.Errorf("parseTarget(%q) = %+v, %v", in, got, ok)
		}
	}
	if _, ok := parseTarget("app.fn@"); ok {
		t.Error("empty selector accepted")
	}
}

func TestHostAllowed(t *testing.T) {
	allow := []string{"api.stripe.com", "*.acme.com", "localhost:8080"}
	for u, want := range map[string]bool{
		"https://api.stripe.com/v1":        true,
		"https://API.STRIPE.COM/v1":        true,
		"https://x.acme.com/":              true,
		"https://acme.com/":                false,
		"https://evil.com/?api.stripe.com": false,
		"http://localhost:8080/":           true,
		"http://localhost:9090/":           false,
		"https://api.stripe.com.evil.com/": false,
	} {
		pu, _ := parseURL(u)
		if got := hostAllowed(allow, pu); got != want {
			t.Errorf("%s: %v, want %v", u, got, want)
		}
	}
}

func TestRewritePlaceholders(t *testing.T) {
	for in, want := range map[string]string{
		"select ? , ?":                     "select $1 , $2",
		"select '?' , ?":                   "select '?' , $1",
		`select "a?b" from t where x = ?`:  `select "a?b" from t where x = $1`,
		"select 'it''s ?', ?":              "select 'it''s ?', $1",
		"select ? -- what?\n, ?":           "select $1 -- what?\n, $2",
		"select /* ? */ ?":                 "select /* ? */ $1",
		"select $$ ? $$, ?":                "select $$ ? $$, $1",
		"select $tag$ ? $tag$, ?, $1::int": "select $tag$ ? $tag$, $1, $1::int",
	} {
		if got := rewritePlaceholders(in); got != want {
			t.Errorf("rewrite(%q) = %q, want %q", in, got, want)
		}
	}
}

func parseURL(s string) (*url.URL, error) { return url.Parse(s) }

func newTestServer(t *testing.T, r *Runner) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(r.Handler())
	t.Cleanup(srv.Close)
	return srv
}

// waitReady waits until version n of address is prepared (any role).
func (h *harness) waitReady(address string, n int) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.r.mu.RLock()
		var st versionState = -1
		if fn := h.r.functions[address]; fn != nil {
			if v := fn.versions[n]; v != nil {
				st, _ = v.currentState()
			}
		}
		h.r.mu.RUnlock()
		if st == stateReady || st == stateEvicted {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	h.t.Fatalf("%s version %d never became ready", address, n)
}

// TestHeartbeatVersionsIsAListWhenEmpty: the platform validates the
// heartbeat body, and a runner holding nothing must still send [] — a null
// list was rejected with 400 VALIDATION until the first version loaded.
func TestHeartbeatVersionsIsAListWhenEmpty(t *testing.T) {
	b, _ := budget.New(64<<20, 0)
	r := &Runner{cfg: Config{Pool: "p"}, budget: b, functions: map[string]*function{}}
	raw, err := json.Marshal(r.heartbeat())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"versions":[]`) {
		t.Fatalf("heartbeat = %s", raw)
	}
}
