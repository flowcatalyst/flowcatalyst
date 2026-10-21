package runner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// fakeTraceSink is the control plane's trace-log endpoint: it records the
// batches it accepted and can be told to fail.
type fakeTraceSink struct {
	mu        sync.Mutex
	batches   [][]control.TraceRecord
	calls     int
	err       error // every call fails with this
	failFirst int   // the first n calls fail with a 500
}

func (s *fakeTraceSink) ship(b control.TraceBatch) (*control.TraceAck, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	if s.failFirst > 0 {
		s.failFirst--
		return nil, &control.ErrStatus{Status: 500, Code: "BOOM", Message: "try later"}
	}
	s.batches = append(s.batches, b.Records)
	return &control.TraceAck{Accepted: len(b.Records)}, nil
}

func (s *fakeTraceSink) TraceLog(_ context.Context, b control.TraceBatch) (*control.TraceAck, error) {
	return s.ship(b)
}

func (s *fakeTraceSink) sizes() []int {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []int
	for _, b := range s.batches {
		out = append(out, len(b))
	}
	return out
}

func (s *fakeTraceSink) all() []control.TraceRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []control.TraceRecord
	for _, b := range s.batches {
		out = append(out, b...)
	}
	return out
}

func (f *fakeCP) TraceLog(_ context.Context, b control.TraceBatch) (*control.TraceAck, error) {
	return f.traces.ship(b)
}

// clock is a controllable runner clock.
type clock struct{ ms atomic.Int64 }

func (c *clock) Now() time.Time          { return time.UnixMilli(c.ms.Load()) }
func (c *clock) advance(d time.Duration) { c.ms.Add(d.Milliseconds()) }

// startTrace runs a runner whose batcher is parked (it neither ticks nor wakes
// on size) so tests read the queue with drainTrace; tune adjusts the tracer
// before Run. stop shuts the runner down and waits for it.
func startTrace(t *testing.T, adjust func(*Config), tune func(*tracer), fns ...control.Function) (h *harness, clk *clock, stop func()) {
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
	clk = &clock{}
	clk.ms.Store(time.Now().UnixMilli())
	r.now = clk.Now
	r.tracer.flushEvery, r.tracer.batchAt = time.Hour, 1<<30
	if tune != nil {
		tune(r.tracer)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	var once sync.Once
	stop = func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(stop)
	h = &harness{t: t, cp: cp, r: r}
	h.waitServing("app.hello", 1)
	return h, clk, stop
}

// traceFn is the fixture function with trace windows.
func traceFn(ws ...control.TraceState) control.Function {
	fn := fnDoc(ver(1, control.RoleLive))
	fn.Trace = ws
	return fn
}

func (c *clock) window(context string, d time.Duration, rate int) control.TraceState {
	return control.TraceState{Context: context, EnabledUntilMs: c.ms.Load() + d.Milliseconds(), SampleRate: rate}
}

// httpWindow is traceFn's window helper before the clock exists: the real time.
func httpWindow(d time.Duration, rate int) control.TraceState {
	return control.TraceState{Context: "http", EnabledUntilMs: time.Now().Add(d).UnixMilli(), SampleRate: rate}
}

// call runs one request synchronously through the handler, so the trace record
// of the call is queued when it returns.
func call(h *harness, method, path string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	return serve(h, req)
}

func opCall(h *harness, op abi.Op, meta any, body string) (code, respMeta string) {
	h.t.Helper()
	frame, err := abi.MarshalFrame(meta, []byte(body))
	if err != nil {
		h.t.Fatal(err)
	}
	rec := call(h, "POST", "/fn/app.hello/call?op="+strconv.Itoa(int(op)), frame, nil)
	if rec.Code != 200 {
		h.t.Fatalf("op %s: %d %s", op, rec.Code, rec.Body.String())
	}
	return rec.Header().Get("X-Code"), rec.Header().Get("X-Meta")
}

func (h *harness) drainTrace() []control.TraceRecord {
	var all []control.TraceRecord
	for {
		b := h.r.tracer.take()
		if len(b) == 0 {
			return all
		}
		all = append(all, b...)
	}
}

func ofKind(recs []control.TraceRecord, kind string) []control.TraceRecord {
	var out []control.TraceRecord
	for _, r := range recs {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func httpData(t *testing.T, rec control.TraceRecord) control.TraceHTTPData {
	t.Helper()
	var d control.TraceHTTPData
	if err := json.Unmarshal(rec.Data, &d); err != nil {
		t.Fatalf("http data: %v (%s)", err, rec.Data)
	}
	return d
}

func counterVal(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

func TestTraceOffRecordsNothing(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn())
	if rec := call(h, "POST", "/fn/app.hello/echo", []byte("x"), nil); rec.Code != 200 {
		t.Fatalf("echo: %d", rec.Code)
	}
	if recs := h.drainTrace(); len(recs) != 0 {
		t.Fatalf("records with no window: %+v", recs)
	}
}

func TestTraceGateAndExpiry(t *testing.T) {
	h, clk, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Minute, 1)))
	call(h, "POST", "/fn/app.hello/echo", []byte("hi"), map[string]string{"X-Correlation-Id": "corr-1"})
	recs := h.drainTrace()
	if len(recs) != 1 || recs[0].Kind != control.TraceKindHTTP || recs[0].Context != "http" {
		t.Fatalf("live window: %+v", recs)
	}
	r := recs[0]
	if r.Address != "app.hello" || r.Version != 1 || r.InvocationID == "" || r.CorrelationID != "corr-1" || r.At != clk.ms.Load() {
		t.Fatalf("record identity: %+v", r)
	}
	// Expiry is judged against the runner's clock when the call starts.
	clk.advance(2 * time.Minute)
	call(h, "POST", "/fn/app.hello/echo", []byte("hi"), nil)
	if recs := h.drainTrace(); len(recs) != 0 {
		t.Fatalf("expired window recorded: %+v", recs)
	}
}

func TestTraceSamplingExact(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 5)))
	var sampled []int
	for i := range 20 {
		call(h, "GET", "/fn/app.hello/echo", nil, nil)
		if recs := h.drainTrace(); len(recs) == 1 {
			sampled = append(sampled, i)
		} else if len(recs) != 0 {
			t.Fatalf("call %d: %d records", i, len(recs))
		}
	}
	if want := []int{0, 5, 10, 15}; !equalInts(sampled, want) {
		t.Fatalf("sampled calls = %v, want %v", sampled, want)
	}
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestNewTraceWindowsKeepsCounter(t *testing.T) {
	first := newTraceWindows(nil, []control.TraceState{{Context: "http", EnabledUntilMs: 10, SampleRate: 3}, {Context: "orders", EnabledUntilMs: 10}})
	(*first)[0].seq.Add(2)
	next := newTraceWindows(first, []control.TraceState{{Context: "http", EnabledUntilMs: 99, SampleRate: 3}})
	if len(*next) != 1 || (*next)[0].seq.Load() != 2 || (*next)[0].untilMs != 99 {
		t.Fatalf("re-render lost the cycle: %+v", *next)
	}
	if (*first)[1].rate != 1 {
		t.Fatalf("rate 0 should mean every call, got %d", (*first)[1].rate)
	}
	if newTraceWindows(first, nil) != nil {
		t.Fatal("no windows should be nil")
	}
}

func TestTraceWindowsReplacedOnApply(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 1)))
	call(h, "GET", "/fn/app.hello/echo", nil, nil)
	if recs := h.drainTrace(); len(recs) != 1 {
		t.Fatalf("before: %d records", len(recs))
	}
	// A new document without the window switches it off.
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{traceFn()}})
	deadline := time.Now().Add(5 * time.Second)
	for h.r.functions["app.hello"].trace.Load() != nil {
		if time.Now().After(deadline) {
			t.Fatal("window never cleared")
		}
		time.Sleep(10 * time.Millisecond)
	}
	call(h, "GET", "/fn/app.hello/echo", nil, nil)
	if recs := h.drainTrace(); len(recs) != 0 {
		t.Fatalf("after removal: %+v", recs)
	}
}

func TestTraceOps(t *testing.T) {
	h, clk, _ := startTrace(t, nil, nil, traceFn())
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{traceFn(clk.window("orders", time.Hour, 1))}})
	waitTrace(t, h)

	// A live, declared context records and answers an empty success.
	code, meta := opCall(h, abi.OpTrace, abi.TraceCall{Context: "orders", Msg: "resolved", Data: map[string]any{"lines": 3}, Reference: "TRIP-1"}, "")
	if code != "0" || meta != "{}" {
		t.Fatalf("trace: %s %s", code, meta)
	}
	recs := ofKind(h.drainTrace(), control.TraceKindTrace)
	if len(recs) != 1 {
		t.Fatalf("trace records: %+v", recs)
	}
	r := recs[0]
	if r.Context != "orders" || r.Level != "info" || r.Message != "resolved" || r.Reference != "TRIP-1" || string(r.Data) != `{"lines":3}` {
		t.Fatalf("trace record: %+v (%s)", r, r.Data)
	}

	errs := []struct {
		name string
		meta any
		code string
	}{
		{"undeclared", abi.TraceCall{Context: "nope", Msg: "x"}, abi.CodeNotDeclared},
		{"http is not writable", abi.TraceCall{Context: "http", Msg: "x"}, abi.CodeBadRequest},
		{"no context", abi.TraceCall{Msg: "x"}, abi.CodeBadRequest},
		{"bad level", abi.TraceCall{Context: "orders", Level: "loud", Msg: "x"}, abi.CodeBadRequest},
		{"message too large", abi.TraceCall{Context: "orders", Msg: strings.Repeat("a", abi.TraceMaxMessage+1)}, abi.CodeTooLarge},
		{"data too large", abi.TraceCall{Context: "orders", Msg: "x", Data: map[string]any{"k": strings.Repeat("a", abi.TraceMaxData)}}, abi.CodeTooLarge},
		{"reference too long", abi.TraceCall{Context: "orders", Msg: "x", Reference: strings.Repeat("a", abi.TraceMaxReference+1)}, abi.CodeTooLarge},
	}
	for _, c := range errs {
		code, meta := opCall(h, abi.OpTrace, c.meta, "")
		if code != "1" || !strings.Contains(meta, c.code) {
			t.Errorf("%s: %s %s, want %s", c.name, code, meta, c.code)
		}
	}
	if code, meta := opCall(h, abi.OpTrace, map[string]any{"context": "orders", "msg": "x", "data": []int{1}}, ""); code != "1" || !strings.Contains(meta, abi.CodeBadRequest) {
		t.Errorf("array data: %s %s", code, meta)
	}
	if recs := ofKind(h.drainTrace(), control.TraceKindTrace); len(recs) != 0 {
		t.Fatalf("rejected calls recorded: %+v", recs)
	}

	// tracing: live for the live context, not for the http window nobody enabled.
	if code, meta := opCall(h, abi.OpTracing, abi.TracingCall{Context: "orders"}, ""); code != "0" || meta != `{"live":true}` {
		t.Errorf("tracing orders: %s %s", code, meta)
	}
	if code, meta := opCall(h, abi.OpTracing, abi.TracingCall{Context: "http"}, ""); code != "0" || meta != `{"live":false}` {
		t.Errorf("tracing http: %s %s", code, meta)
	}
	if code, meta := opCall(h, abi.OpTracing, abi.TracingCall{Context: "nope"}, ""); code != "1" || !strings.Contains(meta, abi.CodeNotDeclared) {
		t.Errorf("tracing undeclared: %s %s", code, meta)
	}
	h.drainTrace()

	// Window off: trace and tracing still succeed, silently.
	clk.advance(2 * time.Hour)
	if code, meta := opCall(h, abi.OpTrace, abi.TraceCall{Context: "orders", Msg: "x"}, ""); code != "0" || meta != "{}" {
		t.Errorf("trace with the window expired: %s %s", code, meta)
	}
	if code, meta := opCall(h, abi.OpTracing, abi.TracingCall{Context: "orders"}, ""); code != "0" || meta != `{"live":false}` {
		t.Errorf("tracing with the window expired: %s %s", code, meta)
	}
	if recs := h.drainTrace(); len(recs) != 0 {
		t.Fatalf("expired window recorded: %+v", recs)
	}
}

// waitTrace waits until the runner has applied a document with trace windows.
func waitTrace(t *testing.T, h *harness) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for h.r.functions["app.hello"].trace.Load() == nil {
		if time.Now().After(deadline) {
			t.Fatal("trace windows never applied")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestTracingHTTPLiveAndSampled(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 2)))
	var live []string
	for range 4 {
		_, meta := opCall(h, abi.OpTracing, abi.TracingCall{Context: "http"}, "")
		live = append(live, meta)
	}
	want := []string{`{"live":true}`, `{"live":false}`, `{"live":true}`, `{"live":false}`}
	if strings.Join(live, ",") != strings.Join(want, ",") {
		t.Fatalf("live = %v, want %v", live, want)
	}
}

// A reference given on a trace call is stamped on the invocation's later
// records (driven at the capabilities level: one op per HTTP call in the
// fixture).
func TestTraceReferenceStamped(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 100, 1024)
	inv := &invTrace{t: tr, address: "app.hello", version: 1, invID: "inv1", live: []string{"http", "orders"}, httpLive: true}
	c := &capabilities{trace: inv, ver: &version{describe: &abi.Describe{TraceContexts: []abi.TraceContext{{Name: "orders"}}}}, log: slog.New(slog.NewTextHandler(io.Discard, nil)), invID: "inv1"}
	if _, _, aerr := c.Call(t.Context(), abi.OpTrace, encode(abi.TraceCall{Context: "orders", Msg: "a", Reference: "TRIP-9"}), nil); aerr != nil {
		t.Fatal(aerr)
	}
	if _, _, aerr := c.Call(t.Context(), abi.OpTrace, encode(abi.TraceCall{Context: "orders", Msg: "b"}), nil); aerr != nil {
		t.Fatal(aerr)
	}
	if _, _, aerr := c.Call(t.Context(), abi.OpLog, encode(abi.Log{Level: "warn", Msg: "c", Attrs: map[string]any{"k": "v"}}), nil); aerr != nil {
		t.Fatal(aerr)
	}
	inv.captureHTTP(httptest.NewRequest("GET", "/x", nil), "/x", nil, &abi.Caller{Kind: abi.CallerAnonymous}, &captureWriter{limit: 10}, "ok", time.Now())
	var got []control.TraceRecord
	for len(got) < 4 {
		b := tr.take()
		if len(b) == 0 {
			break
		}
		got = append(got, b...)
	}
	if len(got) != 4 {
		t.Fatalf("records = %d", len(got))
	}
	for _, r := range got {
		if r.Reference != "TRIP-9" {
			t.Errorf("%s/%s %q has reference %q", r.Kind, r.Context, r.Message, r.Reference)
		}
	}
	if got[2].Kind != control.TraceKindLog || got[2].Level != "warn" || got[2].Message != "c" || string(got[2].Data) != `{"k":"v"}` || got[2].Context != "http" {
		t.Errorf("log record: %+v", got[2])
	}
}

func TestTraceHTTPOutcomes(t *testing.T) {
	fn := traceFn(httpWindow(time.Hour, 1))
	fn.Limits.MaxConcurrency = 1
	h, _, _ := startTrace(t, nil, nil, fn)
	body := []byte(`{"event":1}`)
	cases := []struct {
		name, method, path string
		hdr                map[string]string
		status             int
		outcome            string
	}{
		{"ok", "POST", "/fn/app.hello/echo", nil, 200, control.TraceOutcomeOK},
		{"guest 404", "POST", "/fn/app.hello/hook", signed(body), 404, control.TraceOutcomeOK},
		{"timeout", "GET", "/fn/app.hello/spin", nil, 504, control.TraceOutcomeTimeout},
		{"trap", "GET", "/fn/app.hello/trap", nil, 500, control.TraceOutcomeTrap},
		{"malformed frame", "GET", "/fn/app.hello/malformed", nil, 500, control.TraceOutcomeTrap},
	}
	for _, c := range cases {
		b := []byte("ping")
		if c.path == "/fn/app.hello/hook" {
			b = body
		}
		rec := call(h, c.method, c.path, b, c.hdr)
		recs := h.drainTrace()
		if len(recs) != 1 || recs[0].Kind != control.TraceKindHTTP {
			t.Errorf("%s: records %+v", c.name, recs)
			continue
		}
		d := httpData(t, recs[0])
		if rec.Code != c.status || d.Response.Status != c.status {
			t.Errorf("%s: answered %d, captured %d, want %d", c.name, rec.Code, d.Response.Status, c.status)
		}
		if d.Request.Method != c.method || d.Request.Path != strings.TrimPrefix(c.path, "/fn/app.hello") || d.Outcome == "" {
			t.Errorf("%s: data %+v", c.name, d)
		}
		if d.Outcome != c.outcome {
			t.Errorf("%s: outcome %q, want %q", c.name, d.Outcome, c.outcome)
		}
		if c.name == "ok" && (d.Request.Caller["kind"] != abi.CallerAnonymous || d.Response.Body == nil || *d.Response.Body != "ping" || d.Request.BodyBytes != 4) {
			t.Errorf("ok capture: %+v", d)
		}
		if c.name == "timeout" && d.DurationMs < 100 {
			t.Errorf("timeout duration %d ms", d.DurationMs)
		}
	}

	// busy: the function's only permit is held by a spinning call.
	var wg sync.WaitGroup
	var stopSpin atomic.Bool
	wg.Go(func() {
		for !stopSpin.Load() {
			call(h, "GET", "/fn/app.hello/spin", nil, nil)
		}
	})
	deadline := time.Now().Add(5 * time.Second)
	var busy *httptest.ResponseRecorder
	for time.Now().Before(deadline) {
		busy = call(h, "GET", "/fn/app.hello/echo", nil, nil)
		if busy.Code == 429 {
			break
		}
		h.drainTrace()
		time.Sleep(5 * time.Millisecond)
	}
	stopSpin.Store(true)
	wg.Wait()
	if busy.Code != 429 {
		t.Fatalf("never saw a busy refusal: %d", busy.Code)
	}
	var sawBusy bool
	for _, r := range h.drainTrace() {
		if d := httpData(t, r); d.Outcome == control.TraceOutcomeBusy {
			sawBusy = d.Response.Status == 429 && r.Level == "error"
		}
	}
	if !sawBusy {
		t.Error("no busy http record")
	}
}

// A guest that blows its memory cap is a record too, whichever way the engine
// classifies the failure.
func TestTraceHTTPOutOfMemory(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 1)))
	rec := call(h, "GET", "/fn/app.hello/grow?mb=400", nil, nil)
	recs := h.drainTrace()
	if rec.Code != 500 || len(recs) != 1 {
		t.Fatalf("grow: %d, %d records", rec.Code, len(recs))
	}
	if o := httpData(t, recs[0]).Outcome; o != control.TraceOutcomeOutOfMemory && o != control.TraceOutcomeTrap {
		t.Fatalf("outcome %q", o)
	}
}

func TestTraceOutcomeMapping(t *testing.T) {
	for in, want := range map[string]string{
		"2xx": control.TraceOutcomeOK, "5xx": control.TraceOutcomeOK, "timeout": control.TraceOutcomeTimeout,
		"failed": control.TraceOutcomeTrap, "out_of_memory": control.TraceOutcomeOutOfMemory,
		"malformed": control.TraceOutcomeBadResponse, "busy": control.TraceOutcomeBusy,
		"unavailable": control.TraceOutcomeError, "client_closed": control.TraceOutcomeError,
	} {
		if got := traceOutcomeOf(in); got != want {
			t.Errorf("%s -> %s, want %s", in, got, want)
		}
	}
}

func TestTraceHTTPRedactionAndBodies(t *testing.T) {
	h, _, _ := startTrace(t, func(c *Config) { c.TraceBodyBytes = 10 }, nil, traceFn(httpWindow(time.Hour, 1)))
	call(h, "POST", "/fn/app.hello/echo?a=1", []byte(strings.Repeat("é", 10)), map[string]string{
		"Authorization": "Bearer abc", "Cookie": "s=1", "X-Api-Key": "k", "X-My-Token": "t", "X-Client-Secret": "s",
		"X-Db-Password": "p", "Content-Type": "text/plain",
	})
	recs := h.drainTrace()
	if len(recs) != 1 {
		t.Fatalf("records: %+v", recs)
	}
	d := httpData(t, recs[0])
	for _, name := range []string{"authorization", "cookie", "x-api-key", "x-my-token", "x-client-secret", "x-db-password"} {
		if v := d.Request.Headers[name]; len(v) != 1 || v[0] != redacted {
			t.Errorf("header %s = %v", name, v)
		}
	}
	if d.Request.Headers["content-type"][0] != "text/plain" || d.Request.RawQuery != "a=1" {
		t.Errorf("request: %+v", d.Request)
	}
	// 20 bytes cut at 10: five whole characters, the text stays utf8.
	if d.Request.BodyBytes != 20 || !d.Request.BodyTruncated || d.Request.BodyEncoding != "utf8" || *d.Request.Body != strings.Repeat("é", 5) {
		t.Errorf("request body: %+v %q", d.Request, *d.Request.Body)
	}
	if d.Response.BodyBytes != 20 || !d.Response.BodyTruncated || *d.Response.Body != strings.Repeat("é", 5) {
		t.Errorf("response body: %+v", d.Response)
	}

	// Not UTF-8: base64 of the captured bytes.
	raw := []byte{0xff, 0xfe, 0x00, 0x01}
	call(h, "POST", "/fn/app.hello/echo", raw, nil)
	d = httpData(t, h.drainTrace()[0])
	if d.Request.BodyEncoding != "base64" || *d.Request.Body != base64.StdEncoding.EncodeToString(raw) || d.Request.BodyTruncated {
		t.Errorf("binary request: %+v", d.Request)
	}
	if d.Response.BodyEncoding != "base64" {
		t.Errorf("binary response: %+v", d.Response)
	}
}

// Two 60 KB bodies would make a record the platform throws away; they are
// shrunk until the record fits.
func TestTraceBodiesShrinkToFit(t *testing.T) {
	h, _, _ := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 1)))
	call(h, "POST", "/fn/app.hello/echo", bytes.Repeat([]byte("a"), 60000), nil)
	recs := h.drainTrace()
	if len(recs) != 1 || len(recs[0].Data) > control.MaxTraceHTTPData {
		t.Fatalf("record data %d bytes", len(recs[0].Data))
	}
	d := httpData(t, recs[0])
	if !d.Request.BodyTruncated || d.Request.BodyBytes != 60000 || len(*d.Request.Body) == 0 {
		t.Fatalf("request: truncated=%v bytes=%d", d.Request.BodyTruncated, d.Request.BodyBytes)
	}
}

func TestTraceLogCapture(t *testing.T) {
	h, clk, _ := startTrace(t, nil, nil, traceFn())
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{traceFn(clk.window("http", time.Hour, 1))}})
	waitTrace(t, h)
	opCall(h, abi.OpLog, abi.Log{Level: "error", Msg: "boom", Attrs: map[string]any{"n": 1}}, "")
	recs := h.drainTrace()
	logs := ofKind(recs, control.TraceKindLog)
	if len(logs) != 1 || logs[0].Message != "boom" || logs[0].Level != "error" || logs[0].Context != "http" || logs[0].InvocationID == "" {
		t.Fatalf("log records: %+v", logs)
	}
	if h := ofKind(recs, control.TraceKindHTTP); len(h) != 1 || h[0].InvocationID != logs[0].InvocationID {
		t.Fatalf("log and http records should share the invocation: %+v", recs)
	}

	// Only a declared context live: log lines are not captured.
	h.cp.push(control.Desired{Pool: "default", Functions: []control.Function{traceFn(clk.window("orders", time.Hour, 1))}})
	deadline := time.Now().Add(5 * time.Second)
	for h.r.functions["app.hello"].trace.Load() == nil || (*h.r.functions["app.hello"].trace.Load())[0].context != "orders" {
		if time.Now().After(deadline) {
			t.Fatal("window never replaced")
		}
		time.Sleep(10 * time.Millisecond)
	}
	opCall(h, abi.OpLog, abi.Log{Msg: "quiet"}, "")
	if recs := h.drainTrace(); len(recs) != 0 {
		t.Fatalf("records without http live: %+v", recs)
	}
}

func TestGuestOutputCapture(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 1000, 1024)
	inv := &invTrace{t: tr, address: "a", version: 1, invID: "i", live: []string{"http"}, httpLive: true}
	sink := &invSink{}
	w := &guestWriter{log: slog.New(slog.NewTextHandler(io.Discard, nil)), level: slog.LevelWarn, sink: sink}

	// Nothing while no invocation is running on the instance.
	_, _ = w.Write([]byte("before\n"))
	if tr.queued() != 0 {
		t.Fatal("captured output outside an invocation")
	}
	sink.cur.Store(inv)
	_, _ = w.Write([]byte("line one\r\n\nline two\n"))
	sink.cur.Store(nil)
	_, _ = w.Write([]byte("after\n"))
	recs := tr.take()
	if len(recs) != 2 || recs[0].Message != "line one" || recs[1].Message != "line two" || recs[0].Level != "warn" || recs[0].Kind != control.TraceKindLog || recs[0].InvocationID != "i" {
		t.Fatalf("records: %+v", recs)
	}

	// A chatty guest is capped per invocation, and the excess is counted.
	inv2 := &invTrace{t: tr, address: "a", version: 1, invID: "j", live: []string{"http"}, httpLive: true}
	sink.cur.Store(inv2)
	for range maxInvocationLogRecords + 25 {
		_, _ = w.Write([]byte("spam\n"))
	}
	if tr.queued() != maxInvocationLogRecords || counterVal(tr.m.traceDropped) != 25 {
		t.Fatalf("queued %d dropped %v", tr.queued(), counterVal(tr.m.traceDropped))
	}
}

func newTestTracer(ship TraceShipper, queue, bodyBytes int) *tracer {
	return newTracer(ship, newMetrics(), slog.New(slog.NewTextHandler(io.Discard, nil)), "run1", "default", queue, bodyBytes, time.Now)
}

func rec(i int) control.TraceRecord {
	return control.TraceRecord{At: int64(i), Address: "a", Version: 1, InvocationID: "i", Context: "orders", Kind: control.TraceKindTrace, Level: "info", Message: strconv.Itoa(i)}
}

func TestTraceQueueDropsOldest(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 3, 10)
	for i := range 5 {
		tr.add(rec(i))
	}
	if d := counterVal(tr.m.traceDropped); d != 2 {
		t.Fatalf("dropped = %v, want 2", d)
	}
	got := tr.take()
	if len(got) != 3 || got[0].Message != "2" || got[2].Message != "4" {
		t.Fatalf("kept %+v", got)
	}
	if tr.queued() != 0 || len(tr.take()) != 0 {
		t.Fatal("queue should be empty")
	}
	// The ring keeps working after wrapping.
	tr.add(rec(7))
	if got := tr.take(); len(got) != 1 || got[0].Message != "7" {
		t.Fatalf("after wrap: %+v", got)
	}
}

func TestTracerBatchesToControlServer(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	var runner, pool string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != control.PathTraceLog || r.Method != "POST" {
			http.NotFound(w, r)
			return
		}
		var b control.TraceBatch
		_ = json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		sizes = append(sizes, len(b.Records))
		runner, pool = b.RunnerID, b.Pool
		mu.Unlock()
		w.WriteHeader(202)
		_, _ = io.WriteString(w, `{"accepted":1,"dropped":0}`)
	}))
	defer srv.Close()
	client := control.NewClient(srv.URL, func(context.Context) (string, error) { return "tok", nil }, nil)
	tr := newTestTracer(client, 5000, 10)
	tr.flushEvery, tr.batchAt = 20*time.Millisecond, 1<<30
	for i := range 1200 {
		tr.add(rec(i))
	}
	tr.start()
	waitFor(t, "all batches", func() bool { mu.Lock(); defer mu.Unlock(); return len(sizes) >= 3 })
	tr.close()
	mu.Lock()
	defer mu.Unlock()
	if !equalInts(sizes, []int{500, 500, 200}) || runner != "run1" || pool != "default" {
		t.Fatalf("batches %v runner %q pool %q", sizes, runner, pool)
	}
}

func TestTracerFlushesAtBatchSize(t *testing.T) {
	sink := &fakeTraceSink{}
	tr := newTestTracer(sink, 5000, 10)
	tr.flushEvery = time.Hour // only the size trigger can fire
	tr.start()
	defer tr.close()
	for i := range traceBatchAt - 1 {
		tr.add(rec(i))
	}
	time.Sleep(50 * time.Millisecond)
	if len(sink.sizes()) != 0 {
		t.Fatalf("flushed below the threshold: %v", sink.sizes())
	}
	tr.add(rec(999))
	waitFor(t, "size flush", func() bool { return len(sink.sizes()) == 1 })
	if sink.sizes()[0] != traceBatchAt {
		t.Fatalf("sizes = %v", sink.sizes())
	}
}

func TestTracerByteBound(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 1000, 10)
	big := rec(1)
	big.Data = bytes.Repeat([]byte("1"), 60000)
	for range 50 {
		tr.add(big)
	}
	for tr.queued() > 0 {
		b := tr.take()
		sum := 0
		for _, r := range b {
			sum += len(r.Data)
		}
		if len(b) == 0 || len(b) > control.MaxTraceBatchRecords || sum > control.MaxTraceBatchBytes {
			t.Fatalf("batch of %d records, %d data bytes", len(b), sum)
		}
	}
}

func TestTracerRetriesAfterFailure(t *testing.T) {
	sink := &fakeTraceSink{failFirst: 2}
	tr := newTestTracer(sink, 100, 10)
	tr.flushEvery, tr.backoffBase, tr.backoffMax = 10*time.Millisecond, 5*time.Millisecond, 20*time.Millisecond
	for i := range 3 {
		tr.add(rec(i))
	}
	tr.start()
	defer tr.close()
	waitFor(t, "retry success", func() bool { return len(sink.sizes()) == 1 })
	got := sink.all()
	if len(got) != 3 || got[0].Message != "0" || got[2].Message != "2" {
		t.Fatalf("shipped %+v", got)
	}
	if f := counterVal(tr.m.traceFlushFailures); f != 2 {
		t.Fatalf("flush failures = %v", f)
	}
	if d := counterVal(tr.m.traceDropped); d != 0 {
		t.Fatalf("dropped = %v", d)
	}
}

func TestTracerDropsBatchThePlatformRefuses(t *testing.T) {
	sink := &fakeTraceSink{err: &control.ErrStatus{Status: 403, Code: "FORBIDDEN", Message: "no"}}
	tr := newTestTracer(sink, 100, 10)
	tr.flushEvery = 10 * time.Millisecond
	tr.add(rec(1))
	tr.start()
	defer tr.close()
	waitFor(t, "drop", func() bool { return counterVal(tr.m.traceDropped) == 1 && tr.queued() == 0 })
	// It was tried once, not retried forever.
	time.Sleep(60 * time.Millisecond)
	sink.mu.Lock()
	defer sink.mu.Unlock()
	if sink.calls != 1 {
		t.Fatalf("calls = %d", sink.calls)
	}
}

func TestTracerShutdownFlush(t *testing.T) {
	sink := &fakeTraceSink{}
	tr := newTestTracer(sink, 100, 10)
	tr.flushEvery = time.Hour
	tr.start()
	for i := range 5 {
		tr.add(rec(i))
	}
	tr.close()
	if got := sink.all(); len(got) != 5 {
		t.Fatalf("shutdown flushed %d of 5", len(got))
	}
	tr.close() // idempotent
}

func TestTracerShutdownWithServerDownIsBounded(t *testing.T) {
	sink := &fakeTraceSink{err: errors.New("connection refused")}
	tr := newTestTracer(sink, 100, 10)
	tr.flushEvery, tr.closeGrace = time.Hour, 300*time.Millisecond
	tr.start()
	tr.add(rec(1))
	start := time.Now()
	tr.close()
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("close took %v", el)
	}
	if d := counterVal(tr.m.traceDropped); d != 1 {
		t.Fatalf("dropped = %v", d)
	}
}

// Stopping the runner ships the records of calls that finished just before.
func TestRunnerShutdownFlushesTrace(t *testing.T) {
	h, _, stop := startTrace(t, nil, nil, traceFn(httpWindow(time.Hour, 1)))
	for range 3 {
		call(h, "GET", "/fn/app.hello/echo", nil, nil)
	}
	stop()
	if got := ofKind(h.cp.traces.all(), control.TraceKindHTTP); len(got) != 3 {
		t.Fatalf("shipped %d http records at shutdown", len(got))
	}
}

// With the platform unreachable calls are unaffected and the queue stays bounded.
func TestTraceDoesNotBlockCalls(t *testing.T) {
	h, _, _ := startTrace(t, func(c *Config) { c.TraceQueue = 10 }, func(tr *tracer) {
		tr.flushEvery, tr.batchAt = 5*time.Millisecond, 1
		tr.backoffBase, tr.backoffMax = 5*time.Millisecond, 10*time.Millisecond
	}, traceFn(httpWindow(time.Hour, 1)))
	h.cp.traces.mu.Lock()
	h.cp.traces.err = errors.New("platform down")
	h.cp.traces.mu.Unlock()
	start := time.Now()
	for i := range 100 {
		if rec := call(h, "GET", "/fn/app.hello/echo", nil, nil); rec.Code != 200 {
			t.Fatalf("call %d: %d", i, rec.Code)
		}
	}
	if el := time.Since(start); el > 5*time.Second {
		t.Fatalf("100 calls took %v", el)
	}
	if q := h.r.tracer.queued(); q > 10 {
		t.Fatalf("queue grew to %d", q)
	}
	waitFor(t, "failures counted", func() bool { return counterVal(h.r.metrics.traceFlushFailures) > 0 })
	if counterVal(h.r.metrics.traceRecords.WithLabelValues("http")) != 100 {
		t.Fatal("records not counted")
	}
}

func TestTraceMetricsRegistered(t *testing.T) {
	m := newMetrics()
	m.traceRecords.WithLabelValues("http").Inc()
	mfs, err := m.reg.Gather()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, mf := range mfs {
		names[mf.GetName()] = true
	}
	if !names["fc_fn_trace_records_total"] {
		t.Fatalf("metrics: %v", names)
	}
}

func waitFor(t *testing.T, what string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// With no window live, tracing adds no allocation to a call.
func TestTraceBeginOffAllocatesNothing(t *testing.T) {
	clk := &clock{}
	clk.ms.Store(1_000_000)
	r := &Runner{tracer: newTestTracer(&fakeTraceSink{}, 10, 10), now: clk.Now}
	fn, v := &function{address: "a"}, &version{number: 1}
	if n := testing.AllocsPerRun(100, func() { _ = r.traceBegin(fn, v, "i", "") }); n != 0 {
		t.Errorf("no windows: %v allocs", n)
	}
	fn.trace.Store(newTraceWindows(nil, []control.TraceState{{Context: "http", EnabledUntilMs: 1_000, SampleRate: 1}}))
	if n := testing.AllocsPerRun(100, func() { _ = r.traceBegin(fn, v, "i", "") }); n != 0 {
		t.Errorf("expired window: %v allocs", n)
	}
	// A window that is open but sampled out allocates nothing either.
	fn.trace.Store(newTraceWindows(nil, []control.TraceState{{Context: "http", EnabledUntilMs: 2_000_000, SampleRate: 1 << 30}}))
	r.traceBegin(fn, v, "i", "") // the one sampled-in call
	if n := testing.AllocsPerRun(100, func() { _ = r.traceBegin(fn, v, "i", "") }); n != 0 {
		t.Errorf("sampled out: %v allocs", n)
	}
}

// The trace and tracing ops on an invocation with nothing live queue nothing.
func TestTraceOpsOffQueueNothing(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 10, 10)
	c := &capabilities{ver: &version{describe: &abi.Describe{TraceContexts: []abi.TraceContext{{Name: "orders"}}}}}
	meta := encode(abi.TraceCall{Context: "orders", Msg: "m"})
	if _, _, aerr := c.Call(t.Context(), abi.OpTrace, meta, nil); aerr != nil {
		t.Fatal(aerr)
	}
	c.trace = &invTrace{t: tr, live: []string{"http"}, httpLive: true} // live for http only
	if _, _, aerr := c.Call(t.Context(), abi.OpTrace, meta, nil); aerr != nil {
		t.Fatal(aerr)
	}
	if tr.queued() != 0 {
		t.Fatal("queued a record for a context that is not live")
	}
}

// BenchmarkInvokeTrace compares a call through the handler with tracing absent
// from the function, with only an expired window, and with a window that is
// open for another context. The allocations of the first two must match.
func BenchmarkInvokeTrace(b *testing.B) {
	run := func(b *testing.B, ws ...control.TraceState) {
		budgetB, _ := budget.New(512<<20, 0)
		cp := newFakeCP()
		cp.push(control.Desired{Pool: "default", Functions: []control.Function{traceFn(ws...)}})
		r, err := New(b.Context(), Config{Pool: "default", ControlPlane: cp, Budget: budgetB, Tokens: fakeTokens{}})
		if err != nil {
			b.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { _ = r.Run(ctx); close(done) }()
		defer func() { cancel(); <-done }()
		h := &harness{t: nil, cp: cp, r: r}
		for {
			r.mu.RLock()
			fn := r.functions["app.hello"]
			ok := fn != nil && fn.serving == 1
			r.mu.RUnlock()
			if ok {
				if v := fn.versions[1]; v != nil {
					if st, _ := v.currentState(); st == stateReady {
						break
					}
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		body := []byte("ping")
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			req := httptest.NewRequest("POST", "/fn/app.hello/echo", bytes.NewReader(body))
			rec := httptest.NewRecorder()
			h.r.Handler().ServeHTTP(rec, req)
		}
	}
	past := time.Now().Add(-time.Hour).UnixMilli()
	b.Run("no-window", func(b *testing.B) { run(b) })
	b.Run("expired-window", func(b *testing.B) { run(b, control.TraceState{Context: "http", EnabledUntilMs: past, SampleRate: 1}) })
	b.Run("other-context-open", func(b *testing.B) {
		run(b, control.TraceState{Context: "orders", EnabledUntilMs: time.Now().Add(time.Hour).UnixMilli(), SampleRate: 1})
	})
	b.Run("http-live", func(b *testing.B) {
		run(b, control.TraceState{Context: "http", EnabledUntilMs: time.Now().Add(time.Hour).UnixMilli(), SampleRate: 1})
	})
}

// A queue that is within its record count but over its byte budget drops the
// oldest records: two big bodies per record would otherwise let 5000 records
// hold hundreds of megabytes.
func TestTraceQueueIsBoundedInBytes(t *testing.T) {
	tr := newTestTracer(&fakeTraceSink{}, 5000, 65536)
	tr.maxBytes = minTraceQueueBytes // 1 MiB
	big := control.TraceRecord{Kind: control.TraceKindHTTP, Data: json.RawMessage(strings.Repeat("x", 60000))}
	for i := 0; i < 100; i++ {
		r := big
		r.Message = fmt.Sprintf("m%d", i)
		tr.add(r)
	}
	// each record is about 60.5 KB, so about 17 fit in 1 MiB
	if n := tr.queued(); n < 10 || n > 20 {
		t.Fatalf("queued %d records under a 1 MiB budget", n)
	}
	if tr.bytes > tr.maxBytes {
		t.Fatalf("queued bytes %d over the budget %d", tr.bytes, tr.maxBytes)
	}
	if d := counterVal(tr.m.traceDropped); d != float64(100-tr.queued()) {
		t.Fatalf("dropped %v, queued %d", d, tr.queued())
	}
	// the newest are the ones kept, in order
	first := tr.take()
	if len(first) == 0 || first[len(first)-1].Message != "m99" && tr.queued() == 0 {
		t.Fatalf("newest record lost: %+v", first[len(first)-1].Message)
	}
	// draining returns the accounting to zero
	for tr.queued() > 0 {
		tr.take()
	}
	if tr.bytes != 0 {
		t.Fatalf("bytes after draining = %d", tr.bytes)
	}
}
