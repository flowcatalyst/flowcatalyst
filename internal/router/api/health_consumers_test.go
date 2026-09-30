package api_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

type stubConsumers []router.ConsumerStat

func (s stubConsumers) ConsumerStats() []router.ConsumerStat { return s }

func healthState(consumers ...router.ConsumerStat) *routerapi.State {
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	hs.SetConsumerStats(stubConsumers(consumers))
	return &routerapi.State{Warnings: ws, Health: hs, Mocks: routerapi.NewMockState()}
}

// One stalled consumer among healthy ones is only a WARNING overall, but the
// router is still NOT_READY (R-36).
func TestReadiness_AnyStalledConsumerIsNotReady(t *testing.T) {
	now := time.Now()
	_, api := humatest.New(t)
	routerapi.Register(api, healthState(
		router.ConsumerStat{QueueName: "a", Running: true, LastPoll: now},
		router.ConsumerStat{QueueName: "b", Running: true, LastPoll: now},
		router.ConsumerStat{QueueName: "stuck", Running: true, LastPoll: now.Add(-10 * time.Minute)},
	))
	if resp := api.Get("/health/ready"); resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503", resp.Code)
	}
}

func TestReadiness_HealthyConsumersReady(t *testing.T) {
	_, api := humatest.New(t)
	routerapi.Register(api, healthState(router.ConsumerStat{QueueName: "a", Running: true, LastPoll: time.Now()}))
	if resp := api.Get("/health/ready"); resp.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", resp.Code)
	}
}

// The consumer-health endpoint lists every consumer, healthy or not.
func TestConsumerHealth_ListsHealthyConsumers(t *testing.T) {
	_, api := humatest.New(t)
	routerapi.Register(api, healthState(router.ConsumerStat{QueueName: "a", Running: true, LastPoll: time.Now()}))
	resp := api.Get("/monitoring/consumer-health")
	var body routerapi.ConsumerHealthResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	d, ok := body.Consumers["a"]
	if !ok || !d.IsHealthy {
		t.Fatalf("consumers = %+v, want healthy consumer a", body.Consumers)
	}
	if d.LastPollTimeMs < time.Now().Add(-time.Minute).UnixMilli() {
		t.Errorf("LastPollTimeMs = %d, want an epoch-ms timestamp", d.LastPollTimeMs)
	}
}
