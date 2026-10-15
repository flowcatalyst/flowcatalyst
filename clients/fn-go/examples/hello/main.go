// Command hello is the minimal FlowCatalyst function example for the Go
// guest SDK: an open health check, a platform-authenticated greeting that
// reads a declared config value, and a webhook endpoint that emits an
// event. Build it to wasm with:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o hello.wasm .
package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	fn "github.com/flowcatalyst/flowcatalyst/clients/fn-go"
)

var greeting = fn.Config("GREETING")

func init() {
	fn.Open("GET /healthz", health)
	fn.Platform("GET /hello/{name}", hello)
	fn.Webhook("POST /events/greeting", onGreetingEvent)
	fn.Open("POST /dispatch", onDispatch)

	fn.Emits("hello:greeting:greeting:sent")
	fn.Dispatches("hello.logger", "/deliver")
}

func main() {}

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func hello(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	g, found, err := greeting.Get(r.Context())
	if err != nil {
		fn.Log(r.Context()).Error("config.get failed", "error", err)
		http.Error(w, `{"error":"CONFIG_UNAVAILABLE"}`, http.StatusInternalServerError)
		return
	}
	if !found {
		g = "Hello"
	}

	who := fn.CallerFrom(r.Context())
	resp := map[string]any{"message": g + ", " + name}
	if p := who.Principal(); p != nil {
		resp["caller"] = p.ID
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func onGreetingEvent(w http.ResponseWriter, r *http.Request) {
	// Derive the dedup id from the delivered event's id, so a redelivery of
	// the same event emits the same follow-up exactly once.
	var inbound struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&inbound)
	if inbound.ID == "" {
		inbound.ID = fn.Invocation(r.Context()).ID
	}
	eventID, err := fn.Emit(r.Context(), fn.Event{
		Type:        "hello:greeting:greeting:sent",
		DedupID:     "greeting-sent-" + inbound.ID,
		Source:      "hello.example",
		ContentType: "application/json",
		Data:        []byte(`{"ok":true}`),
	})
	if err != nil {
		if errors.Is(err, fn.ErrRetryable) {
			fn.Retry(w, 5*time.Second)
			return
		}
		fn.Log(r.Context()).Error("emit failed", "error", err)
		fn.Reject(w, "emit refused")
		return
	}
	fn.Log(r.Context()).Info("greeting event emitted", "eventId", eventID)
	w.WriteHeader(http.StatusAccepted)
}

// onDispatch creates a dispatch job for the hello.logger function's /deliver
// webhook. The target and path are declared in init; the platform builds the
// URL and signs the delivery, the function holds no credential.
func onDispatch(w http.ResponseWriter, r *http.Request) {
	jobID, err := fn.Dispatch(r.Context(), fn.Job{
		Target:       "hello.logger",
		Path:         "/deliver",
		Payload:      []byte(`{"hello":"world"}`),
		Metadata:     map[string]string{"from": "hello"},
		MessageGroup: "hello",
	})
	if err != nil {
		if errors.Is(err, fn.ErrRetryable) {
			fn.Retry(w, 5*time.Second)
			return
		}
		fn.Log(r.Context()).Error("dispatch failed", "error", err)
		fn.Reject(w, "dispatch refused")
		return
	}
	fn.Log(r.Context()).Info("job dispatched", "jobId", jobID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"jobId": jobID})
}
