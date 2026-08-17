package api_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// ── Test doubles for the in-flight detail / force-ack endpoints ─────────
// (stubMediatingProvider is shared from mediating_test.go)

type stubAcker struct {
	res    router.ForceAckResult
	found  bool
	lastID string
}

func (s *stubAcker) ForceAckInFlight(_ context.Context, id string) (router.ForceAckResult, bool) {
	s.lastID = id
	return s.res, s.found
}

// setupInFlightOpsAPI wires a minimal State: two tracked messages (one also
// mediating, one retrying) plus a configurable acker.
func setupInFlightOpsAPI(t *testing.T, acker routerapi.InFlightAcker) humatest.TestAPI {
	t.Helper()
	now := time.Now()
	state := &routerapi.State{
		Warnings: router.NewWarningService(router.WarningServiceConfig{}),
		InFlight: stubInFlightProvider{entries: []common.InFlightMessage{
			{
				MessageID: "msg-med", BrokerMessageID: "br-1", PoolCode: "demo",
				QueueIdentifier: "q-demo", MessageGroupID: "g1",
				StartedAt: now.Add(-90 * time.Second), LastSeenAt: now.Add(-30 * time.Second),
			},
			{
				MessageID: "msg-retry", BrokerMessageID: "br-2", PoolCode: "demo",
				QueueIdentifier: "q-demo", Attempts: 3,
				StartedAt: now.Add(-10 * time.Minute), LastSeenAt: now.Add(-10 * time.Minute),
			},
			{
				MessageID: "msg-idle", BrokerMessageID: "br-3", PoolCode: "demo",
				QueueIdentifier: "q-demo",
				StartedAt:       now.Add(-45 * time.Minute), LastSeenAt: now.Add(-40 * time.Minute),
			},
		}},
		Mediating: stubMediatingProvider{entries: []router.MediatingEntry{{
			MessageID: "msg-med", PoolCode: "demo", Queue: "q-demo",
			Target: "https://target.example/hook", MediatedAt: now.Add(-5 * time.Second),
		}}},
		InFlightAck: acker,
		Mocks:       routerapi.NewMockState(),
	}
	_, api := humatest.New(t)
	routerapi.Register(api, state)
	return api
}

func TestInFlightDetail_Mediating(t *testing.T) {
	api := setupInFlightOpsAPI(t, &stubAcker{})
	resp := api.Get("/monitoring/in-flight-messages/detail?messageId=msg-med")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d", resp.Code)
	}
	var d routerapi.InFlightMessageDetail
	decodeBody(t, resp.Body.Bytes(), &d)
	if !d.InPipeline || d.Status != "MEDIATING" {
		t.Fatalf("want MEDIATING in-pipeline, got %+v", d)
	}
	if d.MediationTarget != "https://target.example/hook" {
		t.Fatalf("mediation target = %q", d.MediationTarget)
	}
	if d.MessageGroup != "g1" || d.PoolCode != "demo" || d.QueueID != "q-demo" {
		t.Fatalf("entry fields wrong: %+v", d)
	}
	if d.ElapsedTimeMs < 89_000 || d.LastSeenElapsedMs < 29_000 {
		t.Fatalf("elapsed fields wrong: %+v", d)
	}
}

func TestInFlightDetail_RetryBackoffAndIdle(t *testing.T) {
	api := setupInFlightOpsAPI(t, &stubAcker{})

	var d routerapi.InFlightMessageDetail
	decodeBody(t, api.Get("/monitoring/in-flight-messages/detail?messageId=msg-retry").Body.Bytes(), &d)
	if d.Status != "RETRY_BACKOFF" || d.Attempts != 3 {
		t.Fatalf("want RETRY_BACKOFF attempts=3, got %+v", d)
	}

	decodeBody(t, api.Get("/monitoring/in-flight-messages/detail?messageId=msg-idle").Body.Bytes(), &d)
	if d.Status != "TRACKED_IDLE" {
		t.Fatalf("want TRACKED_IDLE, got %+v", d)
	}
}

func TestInFlightDetail_NotInPipeline(t *testing.T) {
	api := setupInFlightOpsAPI(t, &stubAcker{})
	var d routerapi.InFlightMessageDetail
	decodeBody(t, api.Get("/monitoring/in-flight-messages/detail?messageId=ghost").Body.Bytes(), &d)
	if d.InPipeline || d.Status != "" {
		t.Fatalf("want inPipeline=false with no status, got %+v", d)
	}
}

func TestInFlightForceAck_Success(t *testing.T) {
	acker := &stubAcker{
		found: true,
		res: router.ForceAckResult{Entry: common.InFlightMessage{
			MessageID: "msg-med", BrokerMessageID: "br-1", PoolCode: "demo",
			QueueIdentifier: "q-demo", StartedAt: time.Now().Add(-90 * time.Second),
		}},
	}
	api := setupInFlightOpsAPI(t, acker)
	resp := api.Post("/monitoring/in-flight-messages/msg-med/ack")
	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", resp.Code, resp.Body.String())
	}
	var out routerapi.ForceAckResponse
	decodeBody(t, resp.Body.Bytes(), &out)
	if !out.Removed || !out.BrokerAcked || !out.WasMediating {
		t.Fatalf("want removed+acked+wasMediating, got %+v", out)
	}
	if acker.lastID != "msg-med" || out.QueueID != "q-demo" || out.PoolCode != "demo" {
		t.Fatalf("wrong routing: acker=%q out=%+v", acker.lastID, out)
	}
}

func TestInFlightForceAck_BrokerAckFails(t *testing.T) {
	acker := &stubAcker{
		found: true,
		res: router.ForceAckResult{
			Entry:  common.InFlightMessage{MessageID: "msg-idle", QueueIdentifier: "q-demo", StartedAt: time.Now()},
			AckErr: context.DeadlineExceeded,
		},
	}
	api := setupInFlightOpsAPI(t, acker)
	var out routerapi.ForceAckResponse
	decodeBody(t, api.Post("/monitoring/in-flight-messages/msg-idle/ack").Body.Bytes(), &out)
	if !out.Removed || out.BrokerAcked || out.BrokerAckError == "" {
		t.Fatalf("want removed with surfaced ack error, got %+v", out)
	}
	if out.WasMediating {
		t.Fatalf("msg-idle is not mediating, got %+v", out)
	}
}

func TestInFlightForceAck_NotFound(t *testing.T) {
	api := setupInFlightOpsAPI(t, &stubAcker{found: false})
	if resp := api.Post("/monitoring/in-flight-messages/ghost/ack"); resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d", resp.Code)
	}
}

func TestInFlightForceAck_NotConfigured(t *testing.T) {
	api := setupInFlightOpsAPI(t, nil)
	if resp := api.Post("/monitoring/in-flight-messages/x/ack"); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", resp.Code)
	}
}
