package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/budget"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/engine"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/runtimes"
)

// jsScript is a hand-written guest for the shared JS engine (the SDK wraps
// this protocol): globalThis.__fc with describe and handle.
const jsScript = `
const enc = (s) => __fc_utf8_encode(s);
const reply = (status, body) => ({ meta: JSON.stringify({ status, headers: { "Content-Type": ["text/plain"] } }), body: enc(body) });
globalThis.__fc = {
  describe() {
    return JSON.stringify({ abi: 1, config: ["GREETING"], endpoints: [
      { path: "/echo", auth: "none" }, { path: "/greet", auth: "none" }, { path: "/async", auth: "none" },
      { path: "/throw", auth: "none" }, { path: "/spin", auth: "none", timeoutMs: 150 } ] });
  },
  handle(meta, body) {
    const req = JSON.parse(meta);
    switch (req.path) {
      case "/echo": return reply(200, __fc_utf8_decode(body) + " from v" + req.version);
      case "/greet": {
        const r = __fc_call(2, JSON.stringify({ key: "GREETING" }), new Uint8Array(0));
        return reply(200, __fc_utf8_decode(r.body) + ", " + (req.pathParams.name || "world"));
      }
      case "/async": return Promise.resolve(1).then((n) => reply(201, "async " + n));
      case "/throw": throw new Error("boom");
      case "/spin": for (;;) {}
    }
    return reply(404, "none");
  },
};`

func jsVersion(n int, roles ...string) (control.Version, []byte) {
	src := []byte(jsScript)
	sum := sha256.Sum256(src)
	b, _ := budget.New(256<<20, 0)
	e, err := engine.New(context.Background(), engine.Config{Budget: b})
	if err != nil {
		panic(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	doc, err := runtimes.NewLoader(e).Describe(context.Background(), runtimes.JS, src, 64<<20)
	if err != nil {
		panic(err)
	}
	return control.Version{Number: n, Digest: hex.EncodeToString(sum[:]), Runtime: runtimes.JS, ABI: 1, Describe: doc, Roles: roles}, src
}

func TestJSFunction(t *testing.T) {
	v, src := jsVersion(1, control.RoleLive)
	if _, err := abi.ParseDescribe(v.Describe); err != nil {
		t.Fatalf("the script's describe does not validate: %v", err)
	}
	fn := fnDoc(v)
	b, _ := budget.New(512<<20, 0)
	cp := newFakeCP()
	cp.artifacts[v.Digest] = src
	cp.push(control.Desired{Pool: "default", Functions: []control.Function{fn}})
	r, err := New(t.Context(), Config{Pool: "default", ControlPlane: cp, Budget: b, Tokens: fakeTokens{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	h := &harness{t: t, cp: cp, r: r, srv: newTestServer(t, r)}
	h.waitServing("app.hello", 1)

	if resp, body := h.do("POST", "/fn/app.hello/echo", []byte("kia ora"), nil); resp.StatusCode != 200 || string(body) != "kia ora from v1" {
		t.Errorf("echo: %d %q", resp.StatusCode, body)
	}
	if resp, body := h.do("GET", "/fn/app.hello/greet", nil, nil); resp.StatusCode != 200 || string(body) != "hi, world" {
		t.Errorf("host call from JS: %d %q", resp.StatusCode, body)
	}
	if resp, body := h.do("GET", "/fn/app.hello/async", nil, nil); resp.StatusCode != 201 || string(body) != "async 1" {
		t.Errorf("async handler: %d %q", resp.StatusCode, body)
	}
	if resp, body := h.do("GET", "/fn/app.hello/throw", nil, nil); resp.StatusCode != 500 || strings.Contains(string(body), "boom") {
		t.Errorf("a thrown exception: %d %q", resp.StatusCode, body)
	}
	start := time.Now()
	if resp, _ := h.do("GET", "/fn/app.hello/spin", nil, nil); resp.StatusCode != 504 {
		t.Errorf("an infinite loop in JS: %d", resp.StatusCode)
	}
	if el := time.Since(start); el > 3*time.Second {
		t.Errorf("the JS loop was stopped after %v", el)
	}
	if resp, _ := h.do("POST", "/fn/app.hello/echo", nil, nil); resp.StatusCode != 200 {
		t.Errorf("the function did not recover after a timeout: %d", resp.StatusCode)
	}
	var d abi.Describe
	_ = json.Unmarshal(v.Describe, &d)
	if len(d.Endpoints) != 5 {
		t.Errorf("describe = %s", v.Describe)
	}
}

func TestJSScriptThatFailsToLoadFailsTheVersion(t *testing.T) {
	b, _ := budget.New(256<<20, 0)
	e, err := engine.New(t.Context(), engine.Config{Budget: b})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = e.Close(context.Background()) }()
	l := runtimes.NewLoader(e)
	for name, src := range map[string]string{
		"syntax error":   "this is not javascript (",
		"no __fc":        "const x = 1;",
		"throws at load": "throw new Error('nope')",
	} {
		if _, err := l.Describe(t.Context(), runtimes.JS, []byte(src), 64<<20); err == nil {
			t.Errorf("%s: loaded", name)
		}
		// The instance itself must refuse to come up — not merely answer
		// an empty describe later.
		p, err := l.Prepare(t.Context(), runtimes.JS, []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		inst, err := p.Module.Instantiate(t.Context(), p.InstanceConfig(engine.InstanceConfig{MemoryCapBytes: 64 << 20}))
		if _, ok := errors.AsType[*engine.LoadError](err); !ok {
			t.Errorf("%s: instantiate err = %v, want a LoadError", name, err)
		}
		if inst != nil {
			_ = inst.Close(context.Background())
		}
	}
	if _, err := l.Describe(t.Context(), runtimes.JS, []byte{0xff, 0xfe}, 64<<20); err == nil {
		t.Error("non-UTF-8 script loaded")
	}
}
