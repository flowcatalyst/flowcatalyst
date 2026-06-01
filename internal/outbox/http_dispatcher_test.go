package outbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func newItem() Item {
	return Item{ID: "ob1", ItemType: common.OutboxItemEvent, Payload: json.RawMessage(`{"k":"v"}`)}
}

// OB5 regression: a 2xx response whose per-item result is a failure must NOT
// be classified as success — the prior code ignored the body and marked the
// whole batch success on any 2xx.
func TestSend_PerItemFailureWithin2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK) // 2xx envelope...
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "ob1", "status": "BAD_REQUEST", "error": "schema invalid"}},
		})
	}))
	defer srv.Close()

	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	out := d.Send(context.Background(), newItem())
	if out.Status != common.OutboxBadRequest {
		t.Fatalf("Send status = %v, want BAD_REQUEST (per-item failure inside a 2xx)", out.Status)
	}
	if out.Message != "schema invalid" {
		t.Errorf("Send message = %q, want the per-item error", out.Message)
	}
}

func TestSend_PerItemSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{{"id": "ob1", "status": "SUCCESS"}},
		})
	}))
	defer srv.Close()

	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	if out := d.Send(context.Background(), newItem()); out.Status != common.OutboxSuccess {
		t.Fatalf("Send status = %v, want SUCCESS", out.Status)
	}
}

func TestSend_Non2xxFallsBackToHTTPStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	if out := d.Send(context.Background(), newItem()); out.Status != common.OutboxGatewayError {
		t.Fatalf("Send status = %v, want GATEWAY_ERROR for 503", out.Status)
	}
}

// Rust parity (http_dispatcher.rs match arms): only an EXACT 400 is terminal
// BAD_REQUEST. Every other 4xx — notably a transient 429 — must map to
// INTERNAL_ERROR (retryable), so the item retries instead of permanently
// failing and blocking its message group.
func TestSend_400IsTerminalBadRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	out := d.Send(context.Background(), newItem())
	if out.Status != common.OutboxBadRequest {
		t.Fatalf("400 → %v, want BAD_REQUEST", out.Status)
	}
	if out.Status.IsRetryable() {
		t.Error("400 BAD_REQUEST must not be retryable")
	}
}

func TestSend_Transient4xxIsRetryable(t *testing.T) {
	for _, code := range []int{
		http.StatusTooManyRequests,     // 429
		http.StatusNotFound,            // 404
		http.StatusConflict,            // 409
		http.StatusUnprocessableEntity, // 422
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
		out := d.Send(context.Background(), newItem())
		srv.Close()
		if out.Status != common.OutboxInternalError {
			t.Errorf("%d → %v, want INTERNAL_ERROR (retryable, Rust parity)", code, out.Status)
		}
		if !out.Status.IsRetryable() {
			t.Errorf("%d must be retryable", code)
		}
	}
}

// A transport failure (connection refused) maps to GATEWAY_ERROR (retryable),
// matching Rust's send() Err arm — not INTERNAL_ERROR.
func TestSend_TransportErrorIsGatewayError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listening now → connection refused
	d := NewHTTPDispatcher(url, "", 1*time.Second)
	out := d.Send(context.Background(), newItem())
	if out.Status != common.OutboxGatewayError {
		t.Fatalf("transport error → %v, want GATEWAY_ERROR", out.Status)
	}
	if !out.Status.IsRetryable() {
		t.Error("transport error must be retryable")
	}
}

func batchItem(id string) Item {
	return Item{ID: id, ItemType: common.OutboxItemEvent, Payload: json.RawMessage(`{"id":"` + id + `"}`)}
}

// OB4: SendBatch posts all items in one call and maps each per-item result;
// an item missing from results is INTERNAL_ERROR (retryable).
func TestSendBatch_PerItemResults(t *testing.T) {
	var gotItems int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []json.RawMessage `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotItems = len(body.Items)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"id": "a", "status": "SUCCESS"},
			{"id": "b", "status": "BAD_REQUEST", "error": "bad"},
			// "c" intentionally omitted from results.
		}})
	}))
	defer srv.Close()

	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	out := d.SendBatch(context.Background(), []Item{batchItem("a"), batchItem("b"), batchItem("c")})
	if gotItems != 3 {
		t.Fatalf("server received %d items, want 3 (one batched call)", gotItems)
	}
	if out["a"].Status != common.OutboxSuccess {
		t.Errorf("a = %v, want SUCCESS", out["a"].Status)
	}
	if out["b"].Status != common.OutboxBadRequest || out["b"].Message != "bad" {
		t.Errorf("b = %v/%q, want BAD_REQUEST/bad", out["b"].Status, out["b"].Message)
	}
	if out["c"].Status != common.OutboxInternalError {
		t.Errorf("c (missing from results) = %v, want INTERNAL_ERROR", out["c"].Status)
	}
}

// A non-2xx fails the whole batch with the mapped status.
func TestSendBatch_Non2xxFailsAll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	out := d.SendBatch(context.Background(), []Item{batchItem("a"), batchItem("b")})
	for _, id := range []string{"a", "b"} {
		if out[id].Status != common.OutboxGatewayError {
			t.Fatalf("%s = %v, want GATEWAY_ERROR for 502", id, out[id].Status)
		}
	}
}

// Batch parity: a transient 4xx (429) fails the whole batch as INTERNAL_ERROR
// (retryable), not terminal BAD_REQUEST. Guards the same regression as the
// single-item TestSend_Transient4xxIsRetryable across the batch path.
func TestSendBatch_Transient4xxIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	d := NewHTTPDispatcher(srv.URL, "", 5*time.Second)
	out := d.SendBatch(context.Background(), []Item{batchItem("a"), batchItem("b")})
	for _, id := range []string{"a", "b"} {
		if out[id].Status != common.OutboxInternalError || !out[id].Status.IsRetryable() {
			t.Fatalf("%s = %v, want retryable INTERNAL_ERROR for 429", id, out[id].Status)
		}
	}
}

func TestParseItemStatus(t *testing.T) {
	cases := map[string]common.OutboxStatus{
		"SUCCESS":        common.OutboxSuccess,
		"BAD_REQUEST":    common.OutboxBadRequest,
		"INTERNAL_ERROR": common.OutboxInternalError,
		"UNAUTHORIZED":   common.OutboxUnauthorized,
		"FORBIDDEN":      common.OutboxForbidden,
		"GATEWAY_ERROR":  common.OutboxGatewayError,
	}
	for s, want := range cases {
		got, ok := parseItemStatus(s)
		if !ok || got != want {
			t.Errorf("parseItemStatus(%q) = (%v,%v), want (%v,true)", s, got, ok, want)
		}
	}
	if _, ok := parseItemStatus("WAT"); ok {
		t.Error("parseItemStatus(WAT) should be ok=false")
	}
}
