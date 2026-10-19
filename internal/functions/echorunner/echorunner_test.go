package echorunner

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// plane serves a scripted sequence of documents, then blocks like a long-poll.
type plane struct {
	docs  chan *control.Desired
	hbs   chan control.Heartbeat
	etags int
}

func (p *plane) Desired(ctx context.Context, _, etag string, _ time.Duration) (*control.Desired, string, bool, error) {
	select {
	case d := <-p.docs:
		p.etags++
		return d, strings.Repeat("e", p.etags), true, nil
	case <-ctx.Done():
		return nil, etag, false, ctx.Err()
	}
}

func (p *plane) Heartbeat(_ context.Context, hb control.Heartbeat) error {
	select {
	case p.hbs <- hb:
	default:
	}
	return nil
}

func doc(rev int64, fns ...control.Function) *control.Desired {
	return &control.Desired{Pool: "p", Revision: rev, Functions: fns}
}

func fn(addr string, runtimes ...string) control.Function {
	f := control.Function{ID: "fnc_" + addr, Address: addr}
	for i, rt := range runtimes {
		f.Versions = append(f.Versions, control.Version{Number: i + 1, Runtime: rt})
	}
	return f
}

func waitHB(t *testing.T, p *plane, ok func(control.Heartbeat) bool) control.Heartbeat {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case hb := <-p.hbs:
			if ok(hb) {
				return hb
			}
		case <-deadline:
			t.Fatal("no matching heartbeat")
		}
	}
}

func TestHoldsOnlyEchoAndReportsRevisionStatesAndCapabilities(t *testing.T) {
	p := &plane{docs: make(chan *control.Desired, 4), hbs: make(chan control.Heartbeat, 64)}
	r := New(Config{
		Plane: p, Pool: "p", Version: "1.2.3", HeartbeatEvery: 20 * time.Millisecond, Wait: time.Second,
		StateFor: func(_ string, v control.Version) (string, string) {
			switch v.Number {
			case 2:
				return control.StateCompiled, ""
			case 3:
				return control.StateRetrying, "storage 503"
			case 4:
				return control.StateFailed, "boom"
			}
			return control.StateLoaded, ""
		},
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { _ = r.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	// Before any document: revision 0, capabilities already advertised.
	hb := waitHB(t, p, func(hb control.Heartbeat) bool { return hb.Capabilities != nil })
	if hb.Revision != 0 || len(hb.Versions) != 0 {
		t.Fatalf("first heartbeat = %+v", hb)
	}
	c := hb.Capabilities
	if c.Protocol != control.ProtocolV1 || c.RunnerKind != Kind || c.Version != "1.2.3" || strings.Join(c.Runtimes, ",") != Runtime {
		t.Fatalf("capabilities = %+v", c)
	}

	// Revision 7: four echo versions plus a foreign wasm one that must be
	// ignored (counted, never reported).
	p.docs <- doc(7, fn("a.one", "echo", "echo", "echo", "echo"), fn("a.two", "wasm"))
	hb = waitHB(t, p, func(hb control.Heartbeat) bool { return hb.Revision == 7 && len(hb.Versions) == 4 })
	states := map[int]string{}
	for _, v := range hb.Versions {
		states[v.Number] = v.State
		if v.Number == 3 && v.Reason != "storage 503" || v.Number == 4 && v.Reason != "boom" {
			t.Fatalf("reason lost: %+v", v)
		}
	}
	want := map[int]string{1: control.StateLoaded, 2: control.StateCompiled, 3: control.StateRetrying, 4: control.StateFailed}
	for n, s := range want {
		if states[n] != s {
			t.Fatalf("states = %v, want %v", states, want)
		}
	}
	if r.Ignored() != 1 {
		t.Fatalf("ignored = %d", r.Ignored())
	}
	if n, rev := r.Held(); n != 4 || rev != 7 {
		t.Fatalf("held = %d @%d", n, rev)
	}

	// Revision 8 drops everything.
	p.docs <- doc(8)
	waitHB(t, p, func(hb control.Heartbeat) bool { return hb.Revision == 8 && len(hb.Versions) == 0 })
}

func TestHandlerEchoesHeldFunctionsOnly(t *testing.T) {
	r := New(Config{Plane: &plane{}, Pool: "p"})
	r.apply(doc(1, fn("a.one", "echo")))
	srv := httptest.NewServer(r.Handler())
	defer srv.Close()

	post := func(path, body string) (int, string) {
		resp, err := http.Post(srv.URL+path, "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	code, body := post("/fn/a.one/hook/x", "hi")
	if code != 200 || !strings.Contains(body, `"path":"/hook/x"`) || !strings.Contains(body, `"body":"hi"`) {
		t.Fatalf("held: %d %s", code, body)
	}
	if code, _ := post("/fn/a.two/hook", "hi"); code != 404 {
		t.Fatalf("unheld = %d", code)
	}
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", resp, err)
	}
	resp.Body.Close()
}
