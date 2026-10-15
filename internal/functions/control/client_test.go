package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/abi"
)

func testClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, func(context.Context) (string, error) { return "tok", nil }, nil)
}

func TestClientDispatchSendsTheRequestAndReturnsTheJob(t *testing.T) {
	var got DispatchRequest
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != PathDispatchJobs || r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("%s %s auth=%q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"jobId":"djob_9"}`))
	})
	out, aerr := c.Dispatch(t.Context(), DispatchRequest{
		FunctionID: "fnc_1", Version: 2, InvocationID: "inv_1",
		Dispatch: abi.Dispatch{Target: "a.b", Path: "/d", Metadata: []abi.KV{{Key: "k", Value: "v"}}},
		Payload:  []byte(`{"é":1}`),
	})
	if aerr != nil || out.JobID != "djob_9" {
		t.Fatalf("out=%+v err=%v", out, aerr)
	}
	if got.FunctionID != "fnc_1" || got.Version != 2 || got.Dispatch.Target != "a.b" || string(got.Payload) != `{"é":1}` || got.Dispatch.Metadata[0].Key != "k" {
		t.Fatalf("server saw %+v", got)
	}
}

// The classification the guest sees: platform trouble is the only retryable
// class; a 403 is a refusal of what the function declared or may do; any
// other 4xx is a bad request.
func TestClientDispatchClassifiesRefusals(t *testing.T) {
	for status, want := range map[int]string{
		http.StatusBadGateway:          abi.CodeUnavailable,
		http.StatusInternalServerError: abi.CodeUnavailable,
		http.StatusForbidden:           abi.CodeNotAllowed,
		http.StatusBadRequest:          abi.CodeBadRequest,
		http.StatusNotFound:            abi.CodeBadRequest,
		http.StatusConflict:            abi.CodeBadRequest,
	} {
		c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"error":"X","message":"why"}`))
		})
		_, aerr := c.Dispatch(t.Context(), DispatchRequest{})
		if aerr == nil || aerr.Code != want || aerr.Message != "why" {
			t.Errorf("%d: %+v, want %s", status, aerr, want)
		}
	}
}

func TestClientDispatchUnreachablePlatformIsRetryable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	c := NewClient(url, func(context.Context) (string, error) { return "tok", nil }, nil)
	if _, aerr := c.Dispatch(t.Context(), DispatchRequest{}); aerr == nil || aerr.Code != abi.CodeUnavailable {
		t.Fatalf("err = %+v", aerr)
	}
}

func TestClientDispatchDeadline(t *testing.T) {
	c := testClient(t, func(http.ResponseWriter, *http.Request) {})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, aerr := c.Dispatch(ctx, DispatchRequest{}); aerr == nil || aerr.Code != abi.CodeDeadline {
		t.Fatalf("err = %+v", aerr)
	}
}

func TestClientDispatchUnreadableAnswerIsRetryable(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`not json`))
	})
	if _, aerr := c.Dispatch(t.Context(), DispatchRequest{}); aerr == nil || aerr.Code != abi.CodeUnavailable {
		t.Fatalf("err = %+v", aerr)
	}
}
