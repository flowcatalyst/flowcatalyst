//go:build !wasip1

package fn

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func buildRequestFrame(t *testing.T, meta requestMeta, body []byte) []byte {
	t.Helper()
	mb, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("marshal request meta: %v", err)
	}
	return encodeFrame(mb, body)
}

func decodeResponseFrame(t *testing.T, frame []byte) (responseMeta, []byte) {
	t.Helper()
	mb, body, err := decodeFrame(frame)
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	var rm responseMeta
	if err := json.Unmarshal(mb, &rm); err != nil {
		t.Fatalf("unmarshal response meta: %v", err)
	}
	return rm, body
}

func TestDispatchBasicRoute(t *testing.T) {
	resetRegistry()
	Platform("GET /hello/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := r.PathValue("name")
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello " + name))
	})

	frame := buildRequestFrame(t, requestMeta{
		ID: "inv_1", Address: "app.fn", Version: 1,
		Method: "GET", Path: "/hello/world", Route: "/hello/{name}",
		Caller:         callerMeta{Kind: "anonymous"},
		DeadlineUnixMs: time.Now().Add(time.Second).UnixMilli(),
	}, nil)

	respFrame := handleRequestFrame(frame)
	rm, body := decodeResponseFrame(t, respFrame)

	if rm.Status != http.StatusOK {
		t.Fatalf("status = %d, want 200", rm.Status)
	}
	if string(body) != "hello world" {
		t.Fatalf("body = %q, want %q", body, "hello world")
	}
	if ct := rm.Headers["Content-Type"]; len(ct) != 1 || ct[0] != "text/plain" {
		t.Fatalf("Content-Type header = %v", ct)
	}
}

func TestDispatchPathValueViaServeMux(t *testing.T) {
	// r.PathValue must work because the registered mux (not manual wiring)
	// sets it from matching the route pattern against the request path.
	resetRegistry()
	var got string
	Open("GET /widgets/{id}/parts/{part}", func(w http.ResponseWriter, r *http.Request) {
		got = r.PathValue("id") + "/" + r.PathValue("part")
		w.WriteHeader(http.StatusOK)
	})
	frame := buildRequestFrame(t, requestMeta{
		Method: "GET", Path: "/widgets/42/parts/7",
		Caller: callerMeta{Kind: "anonymous"},
	}, nil)
	handleRequestFrame(frame)
	if got != "42/7" {
		t.Fatalf("path values = %q, want %q", got, "42/7")
	}
}

func TestDispatchHeadersAndQuery(t *testing.T) {
	resetRegistry()
	var gotHeader, gotQuery string
	Open("GET /q", func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Test")
		gotQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusOK)
	})
	frame := buildRequestFrame(t, requestMeta{
		Method: "GET", Path: "/q", RawQuery: "a=1&b=2",
		Headers: map[string][]string{"X-Test": {"yes"}},
		Caller:  callerMeta{Kind: "anonymous"},
	}, nil)
	handleRequestFrame(frame)
	if gotHeader != "yes" {
		t.Errorf("header = %q, want %q", gotHeader, "yes")
	}
	if gotQuery != "a=1&b=2" {
		t.Errorf("query = %q, want %q", gotQuery, "a=1&b=2")
	}
}

func TestDispatchBody(t *testing.T) {
	resetRegistry()
	var gotBody []byte
	Webhook("POST /events/x", func(w http.ResponseWriter, r *http.Request) {
		b := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(b)
		gotBody = b
		w.WriteHeader(http.StatusAccepted)
	})
	frame := buildRequestFrame(t, requestMeta{
		Method: "POST", Path: "/events/x",
		Caller: callerMeta{Kind: "webhook"},
	}, []byte(`{"x":1}`))
	respFrame := handleRequestFrame(frame)
	rm, _ := decodeResponseFrame(t, respFrame)
	if rm.Status != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", rm.Status)
	}
	if string(gotBody) != `{"x":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestDispatchUnknownRoute404(t *testing.T) {
	resetRegistry()
	Open("GET /known", noopHandler)
	frame := buildRequestFrame(t, requestMeta{Method: "GET", Path: "/unknown", Caller: callerMeta{Kind: "anonymous"}}, nil)
	rm, _ := decodeResponseFrame(t, handleRequestFrame(frame))
	if rm.Status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rm.Status)
	}
}

func TestDispatchWrongMethod405(t *testing.T) {
	resetRegistry()
	Open("POST /only-post", noopHandler)
	frame := buildRequestFrame(t, requestMeta{Method: "GET", Path: "/only-post", Caller: callerMeta{Kind: "anonymous"}}, nil)
	rm, _ := decodeResponseFrame(t, handleRequestFrame(frame))
	if rm.Status != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rm.Status)
	}
}

// TestDispatchPanicRecovery pins the panic->500 FUNCTION_PANIC contract
// (plan §9): a handler panic must never trap the guest; it becomes a plain
// 500 response with a fixed body, and the SDK logs it via host op 1.
func TestDispatchPanicRecovery(t *testing.T) {
	resetRegistry()
	Open("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		panic("kaboom")
	})
	fake := &FakeHost{}
	restore := SetHost(fake)
	defer restore()

	frame := buildRequestFrame(t, requestMeta{Method: "GET", Path: "/boom", Caller: callerMeta{Kind: "anonymous"}}, nil)
	rm, body := decodeResponseFrame(t, handleRequestFrame(frame))

	if rm.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rm.Status)
	}
	if string(body) != panicResponseBody {
		t.Fatalf("body = %q, want %q", body, panicResponseBody)
	}
	foundLog := false
	for _, c := range fake.Calls {
		if c.Op == int32(opLog) {
			foundLog = true
		}
	}
	if !foundLog {
		t.Error("panic recovery did not log via op 1")
	}
}

func TestDispatchPanicRecoveryAfterPartialWrite(t *testing.T) {
	// A handler that writes a 200 header and some body before panicking
	// must still end up as a clean 500 FUNCTION_PANIC — the partial write
	// is discarded, not merged with the panic body.
	resetRegistry()
	Open("GET /boom", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("late kaboom")
	})
	frame := buildRequestFrame(t, requestMeta{Method: "GET", Path: "/boom", Caller: callerMeta{Kind: "anonymous"}}, nil)
	rm, body := decodeResponseFrame(t, handleRequestFrame(frame))
	if rm.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rm.Status)
	}
	if string(body) != panicResponseBody {
		t.Fatalf("body = %q, want %q (partial write must be discarded)", body, panicResponseBody)
	}
}

func TestDispatchCallerAndInvocationInContext(t *testing.T) {
	resetRegistry()
	var kind, principalID, invID string
	var version int
	Platform("GET /whoami", func(w http.ResponseWriter, r *http.Request) {
		c := CallerFrom(r.Context())
		kind = c.Kind()
		if p := c.Principal(); p != nil {
			principalID = p.ID
		}
		inv := Invocation(r.Context())
		invID = inv.ID
		version = inv.Version
		w.WriteHeader(http.StatusOK)
	})
	frame := buildRequestFrame(t, requestMeta{
		ID: "inv_42", Address: "app.fn", Version: 3,
		Method: "GET", Path: "/whoami",
		Caller: callerMeta{Kind: "principal", ID: "prn_1", Tier: "CLIENT"},
	}, nil)
	handleRequestFrame(frame)

	if kind != "principal" {
		t.Errorf("kind = %q, want principal", kind)
	}
	if principalID != "prn_1" {
		t.Errorf("principal id = %q, want prn_1", principalID)
	}
	if invID != "inv_42" {
		t.Errorf("invocation id = %q, want inv_42", invID)
	}
	if version != 3 {
		t.Errorf("invocation version = %d, want 3", version)
	}
}

func TestDispatchRetryAndReject(t *testing.T) {
	resetRegistry()
	Open("GET /retry", func(w http.ResponseWriter, r *http.Request) { Retry(w, 5*time.Second) })
	Open("GET /reject", func(w http.ResponseWriter, r *http.Request) { Reject(w, "bad input") })

	rm, _ := decodeResponseFrame(t, handleRequestFrame(buildRequestFrame(t, requestMeta{Method: "GET", Path: "/retry"}, nil)))
	if rm.Status != http.StatusTooManyRequests {
		t.Fatalf("retry status = %d, want 429", rm.Status)
	}
	if ra := rm.Headers["Retry-After"]; len(ra) != 1 || ra[0] != "5" {
		t.Fatalf("Retry-After = %v, want [5]", ra)
	}

	rm2, body2 := decodeResponseFrame(t, handleRequestFrame(buildRequestFrame(t, requestMeta{Method: "GET", Path: "/reject"}, nil)))
	if rm2.Status != http.StatusUnprocessableEntity {
		t.Fatalf("reject status = %d, want 422", rm2.Status)
	}
	if got := rm2.Headers["Flowcatalyst-Outcome"]; len(got) != 1 || got[0] != "reject" {
		t.Fatalf("FlowCatalyst-Outcome header = %v, want [reject]", got)
	}
	if string(body2) != "bad input" {
		t.Fatalf("reject body = %q", body2)
	}
}

func TestDispatchMalformedFrame(t *testing.T) {
	resetRegistry()
	rm, body := decodeResponseFrame(t, handleRequestFrame([]byte{0x01})) // too short to hold a length prefix
	if rm.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rm.Status)
	}
	if string(body) != panicResponseBody {
		t.Fatalf("body = %q", body)
	}
}
