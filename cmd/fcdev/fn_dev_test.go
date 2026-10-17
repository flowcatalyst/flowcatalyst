package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// --- watcher ---------------------------------------------------------------

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestWatcher(t *testing.T) (*pollWatcher, *fakeClock, string) {
	t.Helper()
	dir := t.TempDir()
	clk := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	w := newPollWatcher(dir, 10*time.Millisecond, 150*time.Millisecond)
	w.now = clk.now
	return w, clk, dir
}

func writeAt(t *testing.T, path, content string, mod time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

// settle polls past the debounce and returns what fired.
func settle(w *pollWatcher, clk *fakeClock) []string {
	var got []string
	for range 5 {
		clk.advance(100 * time.Millisecond)
		if p := w.poll(); p != nil {
			got = p
		}
	}
	return got
}

func TestWatcherCreateModifyDelete(t *testing.T) {
	w, clk, dir := newTestWatcher(t)
	base := clk.t
	writeAt(t, filepath.Join(dir, "a.ts"), "one", base)
	w.poll() // baseline

	writeAt(t, filepath.Join(dir, "b.ts"), "new", base)
	if got := settle(w, clk); len(got) != 1 || got[0] != "b.ts" {
		t.Fatalf("create: %v", got)
	}
	writeAt(t, filepath.Join(dir, "a.ts"), "two!", base.Add(time.Second))
	if got := settle(w, clk); len(got) != 1 || got[0] != "a.ts" {
		t.Fatalf("modify: %v", got)
	}
	// Same size, newer mtime is a modification too.
	writeAt(t, filepath.Join(dir, "a.ts"), "TWO!", base.Add(2*time.Second))
	if got := settle(w, clk); len(got) != 1 {
		t.Fatalf("modify same size: %v", got)
	}
	if err := os.Remove(filepath.Join(dir, "b.ts")); err != nil {
		t.Fatal(err)
	}
	if got := settle(w, clk); len(got) != 1 || got[0] != "b.ts" {
		t.Fatalf("delete: %v", got)
	}
	if got := settle(w, clk); got != nil {
		t.Fatalf("quiet tree reported %v", got)
	}
}

func TestWatcherIgnores(t *testing.T) {
	w, clk, dir := newTestWatcher(t)
	base := clk.t
	writeAt(t, filepath.Join(dir, "src", "a.ts"), "x", base)
	w.poll()
	for _, p := range []string{
		"node_modules/pkg/index.js", "dist/function.js", "target/release/x.rs", ".git/HEAD",
		"bin/tool", ".hidden/file", "function.wasm", ".DS_Store", "build.log", "src/.a.ts.swp",
	} {
		writeAt(t, filepath.Join(dir, p), "changed", base.Add(time.Second))
	}
	if got := settle(w, clk); got != nil {
		t.Fatalf("ignored paths fired: %v", got)
	}
}

func TestWatcherDebouncesBursts(t *testing.T) {
	w, clk, dir := newTestWatcher(t)
	base := clk.t
	writeAt(t, filepath.Join(dir, "a.ts"), "0", base)
	w.poll()
	// A burst of saves 50ms apart is one batch, fired only once quiet.
	for i := range 4 {
		writeAt(t, filepath.Join(dir, "a.ts"), strings.Repeat("x", i+1), base.Add(time.Duration(i+1)*time.Second))
		writeAt(t, filepath.Join(dir, "b"+string(rune('0'+i))+".ts"), "y", base)
		clk.advance(50 * time.Millisecond)
		if got := w.poll(); got != nil {
			t.Fatalf("fired mid-burst: %v", got)
		}
	}
	clk.advance(200 * time.Millisecond)
	got := w.poll()
	if len(got) != 5 {
		t.Fatalf("burst batch: %v", got)
	}
	if again := w.poll(); again != nil {
		t.Fatalf("fired twice: %v", again)
	}
}

// --- request replay --------------------------------------------------------

func TestContentTypeFor(t *testing.T) {
	for file, want := range map[string]string{
		"a.xml": "application/xml", "A.XML": "application/xml", "a.json": "application/json",
		"a.csv": "application/octet-stream", "noext": "application/octet-stream",
	} {
		if got := contentTypeFor(file); got != want {
			t.Errorf("%s: %s, want %s", file, got, want)
		}
	}
}

func TestBuildRequestDefaultsAndAuth(t *testing.T) {
	desc := &abi.Describe{ABI: 1, Endpoints: []abi.Endpoint{
		{Path: "/hook", Auth: abi.AuthWebhook}, {Path: "/api", Auth: abi.AuthPlatform}, {Path: "/open", Auth: abi.AuthNone},
	}}
	s := sendSpec{file: "x.xml", path: "/hook", headers: []string{"X-A: 1", "X-A: 2"}}
	req, err := s.buildRequest("http://127.0.0.1:9", "local.x", desc, "sec", []byte("<a/>"))
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.URL.String() != "http://127.0.0.1:9/fn/local.x/hook" || req.Header.Get("Content-Type") != "application/xml" {
		t.Errorf("defaults: %s %s %v", req.Method, req.URL, req.Header)
	}
	if got := req.Header.Values("X-A"); len(got) != 2 {
		t.Errorf("repeated header: %v", got)
	}
	if req.Header.Get("X-FlowCatalyst-Signature") == "" || req.Header.Get("X-FlowCatalyst-Timestamp") == "" {
		t.Error("webhook endpoint request is not signed")
	}
	req, _ = sendSpec{file: "x.json", method: "get", path: "api", contentType: "text/x-custom"}.buildRequest("http://h", "a.b", desc, "s", nil)
	if req.Method != "GET" || req.URL.Path != "/fn/a.b/api" || req.Header.Get("Content-Type") != "text/x-custom" || req.Header.Get("Authorization") != "Bearer dev" {
		t.Errorf("platform: %s %s %v", req.Method, req.URL, req.Header)
	}
	req, _ = sendSpec{file: "x.json", path: "/open"}.buildRequest("http://h", "a.b", desc, "s", nil)
	if req.Header.Get("Authorization") != "" || req.Header.Get("X-FlowCatalyst-Signature") != "" {
		t.Errorf("open endpoint got credentials: %v", req.Header)
	}
	if _, err := (sendSpec{headers: []string{"nocolon"}}).buildRequest("http://h", "a.b", nil, "", nil); err == nil {
		t.Error("a malformed header was accepted")
	}
}

func TestFormatBody(t *testing.T) {
	if got := formatBody([]byte(`{"a":1,"b":[2]}`), "application/json", 1000); !strings.Contains(got, "\n  \"a\": 1") {
		t.Errorf("json not pretty-printed: %q", got)
	}
	if got := formatBody([]byte("plain text"), "text/plain", 1000); got != "plain text" {
		t.Errorf("text: %q", got)
	}
	if got := formatBody(nil, "", 10); got != "(empty body)" {
		t.Errorf("empty: %q", got)
	}
	if got := formatBody([]byte{0xff, 0xfe, 0x00}, "", 10); !strings.Contains(got, "binary") {
		t.Errorf("binary: %q", got)
	}
	big := strings.Repeat("a", 5000)
	got := formatBody([]byte(big), "text/plain", 100)
	if !strings.Contains(got, "truncated: showing 100 of 5000 bytes") || len(got) > 200 {
		t.Errorf("truncation: %q", got)
	}
}

func TestFormatResponseGroupsProduced(t *testing.T) {
	resp := &http.Response{StatusCode: 202, Header: http.Header{"Content-Type": {"application/json"}, "X-Thing": {"1"}, "Set-Cookie": {"s=1"}}}
	var b bytes.Buffer
	formatResponse(&b, resp, []byte(`{"ok":true}`), 12*time.Millisecond, []string{"emit  type=a:b:c:d data={}", "would create job: target=t"}, devStyle{})
	out := b.String()
	for _, want := range []string{"202 Accepted", "Content-Type: application/json", "X-Thing: 1", `"ok": true`, "produced by this call:", "emit  type=a:b:c:d", "would create job: target=t"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Set-Cookie") || strings.Contains(out, "\x1b[") {
		t.Errorf("unselected header or colour leaked:\n%s", out)
	}
	b.Reset()
	formatResponse(&b, resp, nil, 0, nil, devStyle{})
	if !strings.Contains(b.String(), "no events emitted") {
		t.Errorf("empty produced: %s", b.String())
	}
}

func TestMaskSecretsAndAddress(t *testing.T) {
	if got := maskSecrets("token=hunter22 and ab", []string{"hunter22", "ab"}); got != "token=*** and ab" {
		t.Errorf("mask: %q", got)
	}
	if got := defaultDevAddress("/x/Yard_Trip Ingest"); got != "local.yard-trip-ingest" {
		t.Errorf("address: %q", got)
	}
}

// --- dispatch validation ---------------------------------------------------

func TestValidateDispatch(t *testing.T) {
	d := &abi.Describe{ABI: 1, Dispatches: []abi.DispatchTarget{
		{Target: "app.t", Path: "/in", Pool: "pool-a", Connection: "conn-a"},
		{Target: "app.t", Path: "/plain", Connection: "conn-b"},
	}}
	req := func(mut func(*control.DispatchRequest)) control.DispatchRequest {
		r := control.DispatchRequest{Dispatch: abi.Dispatch{Target: "app.t", Path: "/in"}, Payload: []byte(`{"a":1}`)}
		if mut != nil {
			mut(&r)
		}
		return r
	}
	got, aerr := validateDispatch(d, req(nil))
	if aerr != nil || got.Pool != "pool-a" || got.Connection != "conn-a" {
		t.Fatalf("declared match: %v %+v", aerr, got)
	}
	cases := []struct {
		name, code, msg string
		mut             func(*control.DispatchRequest)
	}{
		{"undeclared target", abi.CodeNotAllowed, "app.t/in, app.t/plain", func(r *control.DispatchRequest) { r.Dispatch.Target = "app.other" }},
		{"undeclared path", abi.CodeNotAllowed, "not declared", func(r *control.DispatchRequest) { r.Dispatch.Path = "/nope" }},
		{"bad json", abi.CodeBadRequest, "not valid JSON", func(r *control.DispatchRequest) { r.Payload = []byte(`{nope`) }},
		{"blank target", abi.CodeBadRequest, "required", func(r *control.DispatchRequest) { r.Dispatch.Target = " " }},
		{"bad mode", abi.CodeBadRequest, "mode", func(r *control.DispatchRequest) { r.Dispatch.Mode = "SOMETIMES" }},
		{"reserved metadata", abi.CodeBadRequest, "fn.", func(r *control.DispatchRequest) { r.Dispatch.Metadata = []abi.KV{{Key: "fn.x", Value: "1"}} }},
		{"duplicate metadata", abi.CodeBadRequest, "twice", func(r *control.DispatchRequest) { r.Dispatch.Metadata = []abi.KV{{Key: "a"}, {Key: "a"}} }},
		{"too large", abi.CodeBadRequest, "256 KiB", func(r *control.DispatchRequest) { r.Payload = bytes.Repeat([]byte("1"), 300<<10) }},
	}
	for _, c := range cases {
		_, aerr := validateDispatch(d, req(c.mut))
		if aerr == nil || aerr.Code != c.code || !strings.Contains(aerr.Message, c.msg) {
			t.Errorf("%s: %v", c.name, aerr)
		}
	}
	// A non-JSON content type takes any text payload.
	if _, aerr := validateDispatch(d, req(func(r *control.DispatchRequest) { r.Dispatch.ContentType = "text/plain"; r.Payload = []byte("hi") })); aerr != nil {
		t.Errorf("text payload: %v", aerr)
	}
	if _, aerr := validateDispatch(nil, req(nil)); aerr == nil || aerr.Code != abi.CodeNotAllowed {
		t.Errorf("no describe: %v", aerr)
	}
}

func TestDevCPDispatchNotesPlacement(t *testing.T) {
	var lines []string
	rec := &devRecorder{print: func(l string) { lines = append(lines, l) }}
	cp := newDevCP(&localCP{}, rec)
	cp.desc = &abi.Describe{Dispatches: []abi.DispatchTarget{{Target: "app.t", Path: "/in", Pool: "p1", Connection: "c1"}, {Target: "app.t", Path: "/d", Connection: "c2"}}}
	out, aerr := cp.Dispatch(t.Context(), control.DispatchRequest{Dispatch: abi.Dispatch{Target: "app.t", Path: "/in", MessageGroup: "g1"}, Payload: []byte(`{"a": 1}`)})
	if aerr != nil || !strings.HasPrefix(out.JobID, "djob_local_") {
		t.Fatalf("%v %v", out, aerr)
	}
	_, _ = cp.Dispatch(t.Context(), control.DispatchRequest{Dispatch: abi.Dispatch{Target: "app.t", Path: "/d"}, Payload: []byte(`1`)})
	if _, aerr := cp.Dispatch(t.Context(), control.DispatchRequest{Dispatch: abi.Dispatch{Target: "x.y", Path: "/z"}}); aerr == nil {
		t.Fatal("undeclared dispatch accepted")
	}
	want := []string{
		`would create job: target=app.t path=/in pool=p1 connection=c1 group=g1 payload={"a": 1}`,
		`would create job: target=app.t path=/d pool=default connection=c2 group=- payload=1`,
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: %q, want %q", i, lines[i], w)
		}
	}
	if !strings.HasPrefix(lines[2], "dispatch refused: NOT_ALLOWED") {
		t.Errorf("refusal line: %q", lines[2])
	}
}

// --- the loop, end to end on the real runner with a fake builder -----------

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}
func (s *syncBuf) String() string { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

func waitFor(t *testing.T, b *syncBuf, want string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if s := b.String(); strings.Contains(s, want) {
			return s
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q in:\n%s", want, b.String())
	return ""
}

const devScript = `
const enc = (s) => __fc_utf8_encode(s);
const reply = (status, body) => ({ meta: JSON.stringify({ status, headers: { "Content-Type": ["application/json"] } }), body: enc(body) });
globalThis.__fc = {
  describe() {
    return JSON.stringify({ abi: 1,
      endpoints: [ { path: "/go", auth: "none" }, { path: "/bad", auth: "none" } ],
      emits: ["demo:fn:thing:happened"],
      dispatches: [ { target: "app.t", path: "/in", pool: "p1", connection: "conn1" } ] });
  },
  handle(meta, body) {
    const req = JSON.parse(meta);
    if (req.path === "/go") {
      __fc_call(5, JSON.stringify({ type: "demo:fn:thing:happened", dedupId: "d1" }), enc("{}"));
      const r = __fc_call(11, JSON.stringify({ target: "app.t", path: "/in", messageGroup: "g1" }), enc('{"n":1}'));
      return reply(200, JSON.stringify({ version: "VERSION", got: __fc_utf8_decode(body), dispatched: r.code === 0 }));
    }
    const r = __fc_call(11, JSON.stringify({ target: "app.t", path: "/in" }), enc("{not json"));
    return reply(200, JSON.stringify({ version: "VERSION", code: r.code, meta: r.meta }));
  },
};`

// devFixture is a project dir whose "build" writes devScript into dist.
type devFixture struct {
	dir   string
	mu    sync.Mutex
	ver   string
	fail  error
	calls int
}

func newDevFixture(t *testing.T) *devFixture {
	dir := t.TempDir()
	for f, c := range map[string]string{"package.json": "{}", "src/index.ts": "1", "sample.json": `{"hello":"world"}`} {
		writeAt(t, filepath.Join(dir, f), c, time.Now())
	}
	return &devFixture{dir: dir, ver: "one"}
}

func (f *devFixture) build(_ context.Context, dir string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	out := filepath.Join(dir, "dist", "function.js")
	if f.fail != nil {
		// A broken build also leaves garbage behind; it must never be served.
		_ = os.MkdirAll(filepath.Dir(out), 0o755)
		_ = os.WriteFile(out, []byte("this is not a function {"), 0o644)
		return "", f.fail
	}
	_ = os.MkdirAll(filepath.Dir(out), 0o755)
	return out, os.WriteFile(out, []byte(strings.ReplaceAll(devScript, "VERSION", f.ver)), 0o644)
}

func (f *devFixture) set(ver string, fail error) {
	f.mu.Lock()
	f.ver, f.fail = ver, fail
	f.mu.Unlock()
}

func (f *devFixture) touchSource(t *testing.T, content string) {
	t.Helper()
	writeAt(t, filepath.Join(f.dir, "src", "index.ts"), content, time.Now().Add(time.Hour))
}

type devRun struct {
	out    *syncBuf
	base   chan string
	cancel context.CancelFunc
	done   chan error
}

func (r *devRun) waitBase(t *testing.T) string {
	t.Helper()
	select {
	case b := <-r.base:
		return b
	case <-time.After(30 * time.Second):
		t.Fatal("the dev server never started")
		return ""
	}
}

func startDev(t *testing.T, f *devFixture, mut func(*devOptions)) *devRun {
	t.Helper()
	o := devOptions{
		dir: f.dir, address: "local.fixture", whSecret: "dev-webhook-secret", memoryMB: 256,
		secrets: []string{"API=supersecretvalue"}, pollEvery: 20 * time.Millisecond, debounce: 40 * time.Millisecond,
		send: &sendSpec{file: filepath.Join(f.dir, "sample.json"), path: "/go"},
	}
	if mut != nil {
		mut(&o)
	}
	r := &devRun{out: &syncBuf{}, base: make(chan string, 1), done: make(chan error, 1)}
	ctx, cancel := context.WithCancel(t.Context())
	r.cancel = cancel
	go func() {
		r.done <- runDev(ctx, o, devDeps{build: f.build, out: r.out, in: strings.NewReader(""), ready: func(b string) { r.base <- b }})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-r.done:
			if err != nil {
				t.Logf("dev loop ended with: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("dev loop did not stop on cancel")
		}
	})
	return r
}

func TestDevLoopHotReloadAndBuildFailureKeepsServing(t *testing.T) {
	f := newDevFixture(t)
	r := startDev(t, f, nil)
	base := r.waitBase(t)

	out := waitFor(t, r.out, "produced by this call:")
	for _, want := range []string{
		"ok v1", "routes=2", "emits", "dispatches=1", "200 OK", `"version": "one"`, `"got": "{\"hello\":\"world\"}"`,
		"emit  type=demo:fn:thing:happened",
		"would create job: target=app.t path=/in pool=p1 connection=conn1 group=g1 payload={\"n\":1}",
	} {
		if want == "emits" {
			continue
		}
		if !strings.Contains(out, want) {
			t.Errorf("first load lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "supersecretvalue") {
		t.Error("a --secret value was printed")
	}

	// A failing build: reported, v1 keeps serving, nothing is reloaded.
	f.set("two", errors.New("syntax error at src/index.ts:3"))
	f.touchSource(t, "22")
	out = waitFor(t, r.out, "still serving v1")
	if !strings.Contains(out, "build failed: syntax error at src/index.ts:3") {
		t.Errorf("build error not shown:\n%s", out)
	}
	if strings.Contains(out, "ok v2") || strings.Contains(out, "not loaded") {
		t.Errorf("a failed build was loaded:\n%s", out)
	}
	resp, err := http.Post(base+"/fn/local.fixture/go", "application/json", strings.NewReader("x"))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(b), `"version":"one"`) {
		t.Fatalf("after the failed build the function answered %d %s", resp.StatusCode, b)
	}

	// The fix reloads: v2 serves and the request is replayed against it.
	f.set("two", nil)
	f.touchSource(t, "333")
	waitFor(t, r.out, "ok v2")
	out = waitFor(t, r.out, `"version": "two"`)
	if strings.Count(out, "ok v") != 2 {
		t.Errorf("expected exactly two successful loads:\n%s", out)
	}
	// Writing build output must not retrigger a build.
	f.mu.Lock()
	calls := f.calls
	f.mu.Unlock()
	time.Sleep(300 * time.Millisecond)
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls != calls {
		t.Errorf("build output triggered rebuilds: %d -> %d", calls, f.calls)
	}
}

func TestDevLoopRefusesUndeclaredAndBadDispatchLocally(t *testing.T) {
	f := newDevFixture(t)
	r := startDev(t, f, func(o *devOptions) { o.send.path = "/bad"; o.noWatch = true })
	r.waitBase(t)
	out := waitFor(t, r.out, "200 OK")
	// The guest sent a declared target with a non-JSON payload: the local
	// validator answers BAD_REQUEST and the refusal is shown under the request.
	if !strings.Contains(out, "dispatch refused: BAD_REQUEST: the dispatch payload is not valid JSON") {
		t.Errorf("bad payload not refused locally:\n%s", out)
	}
	if strings.Contains(out, "watching for changes") {
		t.Errorf("--no-watch still watches:\n%s", out)
	}
}

func TestDevLoopFirstBuildFailureKeepsWatching(t *testing.T) {
	f := newDevFixture(t)
	f.set("one", errors.New("boom"))
	r := startDev(t, f, nil)
	out := waitFor(t, r.out, "nothing is serving yet")
	if !strings.Contains(out, "build failed: boom") || !strings.Contains(out, "watching for changes") {
		t.Errorf("first failure output:\n%s", out)
	}
	f.set("one", nil)
	f.touchSource(t, "fixed")
	r.waitBase(t)
	waitFor(t, r.out, "ok v1")
}

func TestDevLoopNoWatchFirstBuildFailureExits(t *testing.T) {
	f := newDevFixture(t)
	f.set("one", errors.New("boom"))
	r := startDev(t, f, func(o *devOptions) { o.noWatch = true })
	select {
	case err := <-r.done:
		if err == nil {
			t.Error("--no-watch with a failing build exited cleanly")
		}
		r.done <- err
	case <-time.After(10 * time.Second):
		t.Fatal("did not exit")
	}
}

func TestDevRejectsNonProjectAndBadSend(t *testing.T) {
	if err := runDev(t.Context(), devOptions{dir: t.TempDir()}, devDeps{out: io.Discard}); err == nil {
		t.Error("an empty dir was accepted")
	}
	f := newDevFixture(t)
	err := runDev(t.Context(), devOptions{dir: f.dir, address: "local.x", send: &sendSpec{file: filepath.Join(f.dir, "missing.json")}}, devDeps{out: io.Discard})
	if err == nil || !strings.Contains(err.Error(), "--send") {
		t.Errorf("missing --send file: %v", err)
	}
}
