package runner

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/fnfixture"
)

// outcomeCount reads fc_fn_invocations_total for app.hello v1.
func outcomeCount(h *harness, outcome string) float64 {
	mfs, err := h.r.metrics.reg.Gather()
	if err != nil {
		h.t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != "fc_fn_invocations_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["address"] == "app.hello" && labels["version"] == "1" && labels["outcome"] == outcome {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

// serve runs one request straight through the handler.
func serve(h *harness, req *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.r.Handler().ServeHTTP(rec, req)
	return rec
}

// errReader fails every read with err.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// Only a body over the limit is a 413; an aborted or broken body is not.
func TestBodyReadErrorsAreNotTooLarge(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))

	rec := serve(h, httptest.NewRequest("POST", "/fn/app.hello/echo", errReader{io.ErrUnexpectedEOF}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "BODY_UNREADABLE") {
		t.Fatalf("broken body: %d %s", rec.Code, rec.Body)
	}
	if outcomeCount(h, "bad_body") != 1 || outcomeCount(h, "too_large") != 0 {
		t.Fatalf("outcomes: bad_body=%v too_large=%v", outcomeCount(h, "bad_body"), outcomeCount(h, "too_large"))
	}

	// The client went away mid-body.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec = serve(h, httptest.NewRequest("POST", "/fn/app.hello/echo", errReader{context.Canceled}).WithContext(ctx))
	if rec.Code != statusClientClosed || rec.Body.Len() != 0 {
		t.Fatalf("aborted body: %d %q", rec.Code, rec.Body)
	}
	if outcomeCount(h, "client_closed") != 1 {
		t.Fatalf("client_closed = %v", outcomeCount(h, "client_closed"))
	}

	// Over the endpoint's limit is still 413.
	rec = serve(h, httptest.NewRequest("POST", "/fn/app.hello/small", strings.NewReader("12345678")))
	if rec.Code != http.StatusRequestEntityTooLarge || outcomeCount(h, "too_large") != 1 {
		t.Fatalf("oversize: %d, too_large=%v", rec.Code, outcomeCount(h, "too_large"))
	}
}

// A caller hanging up mid-call is not a 504 and not a timeout.
func TestClientDisconnectIsNotATimeout(t *testing.T) {
	h := start(t, fnDoc(ver(1, control.RoleLive)))
	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(40 * time.Millisecond); cancel() }()
	// /spin has a 150ms deadline; the client leaves well before it.
	rec := serve(h, httptest.NewRequest("GET", "/fn/app.hello/spin", nil).WithContext(ctx))
	if rec.Code == http.StatusGatewayTimeout || strings.Contains(rec.Body.String(), "FUNCTION_TIMEOUT") {
		t.Fatalf("client disconnect answered as a timeout: %d %s", rec.Code, rec.Body)
	}
	if rec.Code != statusClientClosed {
		t.Fatalf("status %d, want %d", rec.Code, statusClientClosed)
	}
	if outcomeCount(h, "client_closed") != 1 || outcomeCount(h, "timeout") != 0 {
		t.Fatalf("client_closed=%v timeout=%v", outcomeCount(h, "client_closed"), outcomeCount(h, "timeout"))
	}

	// A genuine deadline is still a 504.
	rec = serve(h, httptest.NewRequest("GET", "/fn/app.hello/spin", nil))
	if rec.Code != http.StatusGatewayTimeout || outcomeCount(h, "timeout") != 1 {
		t.Fatalf("deadline: %d timeout=%v", rec.Code, outcomeCount(h, "timeout"))
	}
}

// CORS headers come from the runner's policy only; the guest cannot add to them.
func TestGuestCannotDuplicateOrOverrideCORS(t *testing.T) {
	rec := httptest.NewRecorder()
	c := &abi.CORS{Origins: []string{"https://app.acme.com"}, AllowCredentials: true}
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://app.acme.com")
	if handleCORS(rec, req, c) {
		t.Fatal("not a preflight")
	}
	writeGuestResponse(rec, abi.Response{Status: 200, Headers: map[string][]string{
		"access-control-allow-origin":      {"*"},
		"Access-Control-Allow-Credentials": {"false"},
		"Vary":                             {"Accept"},
		"X-Own":                            {"1"},
	}}, []byte("ok"), true, "inv")
	h := rec.Header()
	if got := h.Values("Access-Control-Allow-Origin"); len(got) != 1 || got[0] != "https://app.acme.com" {
		t.Fatalf("Allow-Origin = %v", got)
	}
	if got := h.Values("Access-Control-Allow-Credentials"); len(got) != 1 || got[0] != "true" {
		t.Fatalf("Allow-Credentials = %v", got)
	}
	if h.Get("X-Own") != "1" || len(h.Values("Vary")) != 2 {
		t.Fatalf("other guest headers lost: %v", h)
	}
}

// Wildcard origin never applies with credentials, even for a stored document.
func TestWildcardOriginWithCredentialsNotReflected(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	handleCORS(rec, req, &abi.CORS{Origins: []string{"*"}, AllowCredentials: true})
	if rec.Header().Get("Access-Control-Allow-Origin") != "" || rec.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("reflected: %v", rec.Header())
	}
}

func TestGuestStatusRange(t *testing.T) {
	for s, want := range map[int]bool{0: false, 99: false, 100: false, 101: false, 199: false, 200: true, 204: true, 599: true, 600: false, 999: false, 1000: false} {
		if validGuestStatus(s) != want {
			t.Errorf("validGuestStatus(%d) = %v", s, !want)
		}
	}
}

// panicLogHandler panics when a per-call guest logger is derived (runner code
// that runs between taking an instance and handing it back), once armed.
// (A panic inside a host call is recovered by the engine, so it cannot be used
// to reach that window.)
type panicLogHandler struct {
	slog.Handler
	armed *atomic.Bool
}

func (p panicLogHandler) WithAttrs(a []slog.Attr) slog.Handler {
	if p.armed.Load() {
		panic("guest logger exploded")
	}
	return panicLogHandler{p.Handler.WithAttrs(a), p.armed}
}

// A panic between taking an instance and handing it back must not keep its
// memory charge.
func TestPanicDoesNotLeakInstance(t *testing.T) {
	var armed atomic.Bool
	h := startCfg(t, func(c *Config) {
		c.Logger = slog.New(panicLogHandler{slog.NewTextHandler(io.Discard, nil), &armed})
	}, fnDoc(ver(1, control.RoleLive)))
	armed.Store(true)
	panicked := false
	func() {
		defer func() { panicked = recover() != nil }()
		serve(h, httptest.NewRequest("GET", "/fn/app.hello/echo", nil))
	}()
	armed.Store(false)
	if !panicked {
		t.Fatal("the injected panic never fired")
	}
	h.r.mu.RLock()
	v := h.r.functions["app.hello"].versions[1]
	h.r.mu.RUnlock()
	v.mu.Lock()
	p := v.pool
	v.mu.Unlock()
	p.close() // idle instances go; only a leaked one would still be charged
	if used := h.r.budget.Stats().UsedBytes; used != 0 {
		t.Fatalf("%d bytes still charged after the pool closed", used)
	}
}

const webhookAnyMethodDoc = `{"abi":1,"endpoints":[{"path":"/hook","auth":"webhook"}]}`

// A webhook endpoint with no declared method still only takes POST.
func TestWebhookEndpointWithoutMethodRefusesNonPost(t *testing.T) {
	v := control.Version{Number: 1, Digest: fnfixture.Digest(), ABI: 1, Describe: json.RawMessage(webhookAnyMethodDoc), Roles: []string{control.RoleLive}}
	h := start(t, fnDoc(v))
	body := []byte(`{"e":1}`)
	for _, m := range []string{"GET", "PUT", "DELETE", "PATCH"} {
		resp, b := h.do(m, "/fn/app.hello/hook", body, signed(body))
		if resp.StatusCode != http.StatusMethodNotAllowed || !strings.Contains(string(b), "METHOD_NOT_ALLOWED") {
			t.Errorf("%s: %d %s", m, resp.StatusCode, b)
		}
	}
	if resp, b := h.do("POST", "/fn/app.hello/hook", body, signed(body)); resp.StatusCode != 404 || string(b) != "no such fixture path" {
		t.Fatalf("signed POST: %d %s", resp.StatusCode, b)
	}
}
