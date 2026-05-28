package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2/humatest"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

// stubStreamHealthProvider implements routerapi.StreamHealthProvider.
type stubStreamHealthProvider struct {
	agg   routerapi.StreamHealthAggregate
	live  bool
	ready bool
}

func (s stubStreamHealthProvider) Aggregate() routerapi.StreamHealthAggregate { return s.agg }
func (s stubStreamHealthProvider) IsLive() bool                               { return s.live }
func (s stubStreamHealthProvider) IsReady() bool                              { return s.ready }

func TestStreamHealth_NotConfigured(t *testing.T) {
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	_, api := humatest.New(t)
	routerapi.Register(api, &routerapi.State{
		Warnings: ws, Health: hs, Mocks: routerapi.NewMockState(),
	})

	resp := api.Get("/monitoring/stream-health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d", resp.Code)
	}
	var body routerapi.StreamHealthResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Enabled {
		t.Errorf("Enabled=true without provider; want false")
	}
	if body.Status != "NOT_CONFIGURED" {
		t.Errorf("Status=%q want NOT_CONFIGURED", body.Status)
	}
}

func TestStreamHealth_LiveProvider(t *testing.T) {
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)

	provider := stubStreamHealthProvider{
		agg: routerapi.StreamHealthAggregate{
			Healthy:        true,
			TotalStreams:   2,
			HealthyStreams: 2,
			Streams: []routerapi.StreamHealth{
				{Name: "event_projection", Running: true, Healthy: true, BatchSequence: 42, Status: "RUNNING"},
				{Name: "dispatch_job_projection", Running: true, Healthy: true, BatchSequence: 17, Status: "RUNNING"},
			},
		},
		live:  true,
		ready: true,
	}

	_, api := humatest.New(t)
	routerapi.Register(api, &routerapi.State{
		Warnings:     ws,
		Health:       hs,
		StreamHealth: provider,
		Mocks:        routerapi.NewMockState(),
	})

	resp := api.Get("/monitoring/stream-health")
	if resp.Code != http.StatusOK {
		t.Fatalf("status %d", resp.Code)
	}
	var body routerapi.StreamHealthResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v body=%s", err, resp.Body.String())
	}
	if !body.Enabled || body.TotalStreams != 2 || body.HealthyStreams != 2 {
		t.Errorf("body=%+v", body)
	}
	if len(body.Streams) != 2 || body.Streams[0].BatchSequence != 42 {
		t.Errorf("Streams=%+v", body.Streams)
	}

	// live + ready probes
	live := api.Get("/monitoring/stream-health/live")
	if live.Code != http.StatusOK {
		t.Errorf("live status=%d", live.Code)
	}
	ready := api.Get("/monitoring/stream-health/ready")
	if ready.Code != http.StatusOK {
		t.Errorf("ready status=%d", ready.Code)
	}
}

func TestStreamHealth_LiveProviderNotReady(t *testing.T) {
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)

	provider := stubStreamHealthProvider{
		agg: routerapi.StreamHealthAggregate{
			Healthy:          false,
			TotalStreams:     2,
			HealthyStreams:   1,
			UnhealthyStreams: 1,
		},
		live:  true,
		ready: false,
	}

	_, api := humatest.New(t)
	routerapi.Register(api, &routerapi.State{
		Warnings: ws, Health: hs, StreamHealth: provider, Mocks: routerapi.NewMockState(),
	})

	ready := api.Get("/monitoring/stream-health/ready")
	if ready.Code != http.StatusServiceUnavailable {
		t.Errorf("ready status=%d want 503", ready.Code)
	}
}
