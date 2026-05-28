package api

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

const (
	tagHealth     = "health"
	tagMonitoring = "monitoring"
)

func registerHealth(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "health", Method: http.MethodGet, Path: "/health",
		Summary: "Health check", Tags: []string{tagHealth}, DefaultStatus: http.StatusOK,
	}, s.health)
	huma.Register(api, huma.Operation{
		OperationID: "healthAlias", Method: http.MethodGet, Path: "/q/health",
		Summary: "Health check (k8s alias)", Tags: []string{tagHealth}, DefaultStatus: http.StatusOK,
	}, s.health)
	huma.Register(api, huma.Operation{
		OperationID: "livenessProbe", Method: http.MethodGet, Path: "/health/live",
		Summary: "Kubernetes liveness probe", Tags: []string{tagHealth}, DefaultStatus: http.StatusOK,
	}, s.liveness)
	huma.Register(api, huma.Operation{
		OperationID: "readinessProbe", Method: http.MethodGet, Path: "/health/ready",
		Summary: "Kubernetes readiness probe", Tags: []string{tagHealth}, DefaultStatus: http.StatusOK,
	}, s.readiness)
	huma.Register(api, huma.Operation{
		OperationID: "startupProbe", Method: http.MethodGet, Path: "/health/startup",
		Summary: "Kubernetes startup probe", Tags: []string{tagHealth}, DefaultStatus: http.StatusOK,
	}, s.readiness)
}

type healthOutput struct {
	Body SimpleHealthResponse
}

func (s *State) health(_ context.Context, _ *emptyInput) (*healthOutput, error) {
	report := s.Health.HealthReport(s.poolStatsSnap())
	return &healthOutput{Body: SimpleHealthResponse{
		Status:           statusString(report.Status),
		Version:          Version,
		ActiveWarnings:   report.ActiveWarnings,
		CriticalWarnings: report.CriticalWarnings,
	}}, nil
}

type probeOutput struct {
	Status int
	Body   ProbeResponse
}

func (s *State) liveness(_ context.Context, _ *emptyInput) (*probeOutput, error) {
	return &probeOutput{Status: http.StatusOK, Body: ProbeResponse{Status: "LIVE"}}, nil
}

func (s *State) readiness(_ context.Context, _ *emptyInput) (*probeOutput, error) {
	report := s.Health.HealthReport(s.poolStatsSnap())
	if report.Status == router.HealthDegraded {
		return &probeOutput{
			Status: http.StatusServiceUnavailable,
			Body:   ProbeResponse{Status: "NOT_READY"},
		}, nil
	}
	return &probeOutput{Status: http.StatusOK, Body: ProbeResponse{Status: "READY"}}, nil
}

// ── Monitoring overview ──────────────────────────────────────────────────

func registerMonitoring(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "monitoring", Method: http.MethodGet, Path: "/monitoring",
		Summary: "Detailed monitoring", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.monitoring)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringHealth", Method: http.MethodGet, Path: "/monitoring/health",
		Summary: "Dashboard health summary", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.dashboardHealth)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringPools", Method: http.MethodGet, Path: "/monitoring/pools",
		Summary: "Pool statistics", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.monitoringPools)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringWarnings", Method: http.MethodGet, Path: "/monitoring/warnings",
		Summary: "Active warnings for dashboard", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.monitoringWarnings)
	huma.Register(api, huma.Operation{
		OperationID: "consumerHealth", Method: http.MethodGet, Path: "/monitoring/consumer-health",
		Summary: "Per-consumer health", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.consumerHealth)
}

type monitoringOutput struct {
	Body MonitoringResponse
}

func (s *State) monitoring(_ context.Context, _ *emptyInput) (*monitoringOutput, error) {
	stats := s.poolStatsSnap()
	report := s.Health.HealthReport(stats)
	return &monitoringOutput{Body: MonitoringResponse{
		Status:           statusString(report.Status),
		Version:          Version,
		HealthReport:     fromHealthReport(report),
		PoolStats:        fromPoolStats(stats),
		ActiveWarnings:   uint32(s.Warnings.UnacknowledgedCount()),
		CriticalWarnings: uint32(s.Warnings.CriticalCount()),
	}}, nil
}

type dashboardHealthOutput struct {
	Body DashboardHealthResponse
}

func (s *State) dashboardHealth(_ context.Context, _ *emptyInput) (*dashboardHealthOutput, error) {
	stats := s.poolStatsSnap()
	report := s.Health.HealthReport(stats)
	var degradationReason *string
	if len(report.Issues) > 0 {
		joined := strings.Join(report.Issues, "; ")
		degradationReason = &joined
	}
	breakersOpen := uint32(0)
	if s.OpenCount != nil {
		breakersOpen = uint32(s.OpenCount.OpenCount())
	}
	return &dashboardHealthOutput{Body: DashboardHealthResponse{
		Status:       statusString(report.Status),
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		UptimeMillis: time.Since(startTime).Milliseconds(),
		Details: &DashboardHealthDetails{
			TotalQueues:         report.ConsumersHealthy + report.ConsumersUnhealthy,
			HealthyQueues:       report.ConsumersHealthy,
			TotalPools:          report.PoolsHealthy + report.PoolsUnhealthy,
			HealthyPools:        report.PoolsHealthy,
			ActiveWarnings:      report.ActiveWarnings,
			CriticalWarnings:    report.CriticalWarnings,
			CircuitBreakersOpen: breakersOpen,
			DegradationReason:   degradationReason,
		},
	}}, nil
}

type monitoringPoolsOutput struct {
	Body []WirePoolStats
}

func (s *State) monitoringPools(_ context.Context, _ *emptyInput) (*monitoringPoolsOutput, error) {
	return &monitoringPoolsOutput{Body: fromPoolStats(s.poolStatsSnap())}, nil
}

type monitoringWarningsOutput struct {
	Body []WireWarning
}

func (s *State) monitoringWarnings(_ context.Context, _ *emptyInput) (*monitoringWarningsOutput, error) {
	warnings := s.Warnings.Active(30)
	sort.Slice(warnings, func(i, j int) bool {
		return warnings[i].CreatedAt.After(warnings[j].CreatedAt)
	})
	return &monitoringWarningsOutput{Body: fromWarnings(warnings)}, nil
}

type consumerHealthOutput struct {
	Body ConsumerHealthResponse
}

func (s *State) consumerHealth(_ context.Context, _ *emptyInput) (*consumerHealthOutput, error) {
	now := time.Now().UTC()
	consumers := map[string]ConsumerHealthDetail{}
	for _, id := range s.Health.StalledConsumers() {
		h := s.Health.ConsumerHealth(id)
		consumers[id] = consumerDetail(id, h, now)
	}
	return &consumerHealthOutput{Body: ConsumerHealthResponse{
		CurrentTimeMs: now.UnixMilli(),
		CurrentTime:   now.Format(time.RFC3339Nano),
		Consumers:     consumers,
	}}, nil
}

// ── helpers ──────────────────────────────────────────────────────────────

func (s *State) poolStatsSnap() []router.PoolStats {
	if s.PoolStats == nil {
		return nil
	}
	return s.PoolStats.PoolStats()
}

func statusString(s router.HealthStatus) string {
	switch s {
	case router.HealthHealthy:
		return "HEALTHY"
	case router.HealthWarning:
		return "WARNING"
	case router.HealthDegraded:
		return "DEGRADED"
	default:
		return "UNKNOWN"
	}
}

func consumerDetail(id string, h router.ConsumerHealth, now time.Time) ConsumerHealthDetail {
	lastPollMs := int64(0)
	if h.LastPollTimeMs != nil {
		lastPollMs = *h.LastPollTimeMs
	}
	timeSinceMs := int64(-1)
	if h.TimeSinceLastPollMs != nil {
		timeSinceMs = *h.TimeSinceLastPollMs
	}
	lastPollTime := "never"
	if lastPollMs > 0 {
		lastPollTime = now.Add(-time.Duration(timeSinceMs) * time.Millisecond).Format(time.RFC3339Nano)
	}
	timeSinceSeconds := int64(-1)
	if timeSinceMs > 0 {
		timeSinceSeconds = timeSinceMs / 1000
	}
	return ConsumerHealthDetail{
		MapKey:                   id,
		QueueIdentifier:          id,
		ConsumerQueueIdentifier:  id,
		IsHealthy:                h.IsHealthy,
		LastPollTimeMs:           lastPollMs,
		LastPollTime:             lastPollTime,
		TimeSinceLastPollMs:      timeSinceMs,
		TimeSinceLastPollSeconds: timeSinceSeconds,
		IsRunning:                h.IsRunning,
	}
}
