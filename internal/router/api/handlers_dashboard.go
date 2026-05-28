package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

func registerDashboardReads(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "dashboardPoolStats", Method: http.MethodGet, Path: "/monitoring/pool-stats",
		Summary: "Pool stats (dashboard shape)", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.dashboardPoolStats)
	huma.Register(api, huma.Operation{
		OperationID: "dashboardQueueStats", Method: http.MethodGet, Path: "/monitoring/queue-stats",
		Summary: "Queue stats (dashboard shape)", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.dashboardQueueStats)
	huma.Register(api, huma.Operation{
		OperationID: "queueMetrics", Method: http.MethodGet, Path: "/monitoring/queues",
		Summary: "Queue metrics list", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.queueMetrics)
	huma.Register(api, huma.Operation{
		OperationID: "dashboardCircuitBreakers", Method: http.MethodGet, Path: "/monitoring/circuit-breakers",
		Summary: "Circuit breaker snapshot", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.dashboardCircuitBreakers)
	huma.Register(api, huma.Operation{
		OperationID: "circuitBreakerState", Method: http.MethodGet, Path: "/monitoring/circuit-breakers/{name}/state",
		Summary: "Get a single circuit breaker's state", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.circuitBreakerState)
	huma.Register(api, huma.Operation{
		OperationID: "dashboardInFlight", Method: http.MethodGet, Path: "/monitoring/in-flight-messages",
		Summary: "List in-flight messages", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.dashboardInFlight)
	huma.Register(api, huma.Operation{
		OperationID: "inFlightCheck", Method: http.MethodGet, Path: "/monitoring/in-flight-messages/check",
		Summary: "Check if a single message is in-flight", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.inFlightCheck)
	huma.Register(api, huma.Operation{
		OperationID: "inFlightCheckBatch", Method: http.MethodPost, Path: "/monitoring/in-flight-messages/check-batch",
		Summary: "Check multiple message IDs at once", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.inFlightCheckBatch)
}

// parseTimeWindow maps the dashboard time_window query value to a Duration.
// Empty / unknown / "all" / "all-time" → 0 (all-time).
func parseTimeWindow(raw string) time.Duration {
	switch strings.TrimSpace(raw) {
	case "5min", "5m":
		return 5 * time.Minute
	case "30min", "30m":
		return 30 * time.Minute
	default:
		return 0
	}
}

type dashboardPoolStatsInput struct {
	TimeWindow string `query:"time_window"`
}

type dashboardPoolStatsOutput struct {
	Body map[string]DashboardPoolStats
}

func (s *State) dashboardPoolStats(_ context.Context, in *dashboardPoolStatsInput) (*dashboardPoolStatsOutput, error) {
	window := parseTimeWindow(in.TimeWindow)
	stats := s.poolStatsSnap()
	out := make(map[string]DashboardPoolStats, len(stats))
	for _, ps := range stats {
		out[ps.PoolCode] = poolStatsToDashboard(ps, window)
	}
	return &dashboardPoolStatsOutput{Body: out}, nil
}

func poolStatsToDashboard(s router.PoolStats, window time.Duration) DashboardPoolStats {
	var succeeded, failed, rateLimited uint64
	successRate := 1.0
	var avgMs float64
	if s.Metrics != nil {
		switch window {
		case 5 * time.Minute:
			succeeded = s.Metrics.Last5Min.SuccessCount
			failed = s.Metrics.Last5Min.FailureCount
			rateLimited = s.Metrics.Last5Min.RateLimitedCount
			successRate = s.Metrics.Last5Min.SuccessRate
			avgMs = s.Metrics.Last5Min.ProcessingTime.AvgMs
		case 30 * time.Minute:
			succeeded = s.Metrics.Last30Min.SuccessCount
			failed = s.Metrics.Last30Min.FailureCount
			rateLimited = s.Metrics.Last30Min.RateLimitedCount
			successRate = s.Metrics.Last30Min.SuccessRate
			avgMs = s.Metrics.Last30Min.ProcessingTime.AvgMs
		default:
			succeeded = s.Metrics.TotalSuccess
			failed = s.Metrics.TotalFailure
			rateLimited = s.Metrics.TotalRateLimited
			successRate = s.Metrics.SuccessRate
			avgMs = s.Metrics.ProcessingTime.AvgMs
		}
	}
	avail := uint32(0)
	if s.Concurrency > s.ActiveWorkers {
		avail = s.Concurrency - s.ActiveWorkers
	}
	return DashboardPoolStats{
		PoolCode:                s.PoolCode,
		TotalProcessed:          succeeded + failed,
		TotalSucceeded:          succeeded,
		TotalFailed:             failed,
		TotalRateLimited:        rateLimited,
		SuccessRate:             successRate,
		ActiveWorkers:           s.ActiveWorkers,
		AvailablePermits:        avail,
		MaxConcurrency:          s.Concurrency,
		QueueSize:               s.QueueSize,
		MaxQueueCapacity:        s.QueueCapacity,
		AverageProcessingTimeMs: avgMs,
	}
}

type dashboardQueueStatsInput struct {
	TimeWindow string `query:"time_window"`
	Refresh    string `query:"refresh"`
}

type dashboardQueueStatsOutput struct {
	Body map[string]DashboardQueueStats
}

func (s *State) dashboardQueueStats(_ context.Context, in *dashboardQueueStatsInput) (*dashboardQueueStatsOutput, error) {
	out := map[string]DashboardQueueStats{}
	if s.BrokerStats == nil {
		return &dashboardQueueStatsOutput{Body: out}, nil
	}
	if in.Refresh == "true" {
		s.BrokerStats.Refresh()
	}
	window := parseTimeWindow(in.TimeWindow)
	for _, m := range s.BrokerStats.GetWindowed(window) {
		processed := m.TotalAcked + m.TotalNacked
		rate := 1.0
		if processed > 0 {
			rate = float64(m.TotalAcked) / float64(processed)
		}
		out[m.QueueIdentifier] = DashboardQueueStats{
			Name:               m.QueueIdentifier,
			TotalMessages:      m.TotalPolled,
			TotalConsumed:      m.TotalAcked,
			TotalFailed:        m.TotalNacked,
			TotalDeferred:      m.TotalDeferred,
			SuccessRate:        rate,
			CurrentSize:        m.PendingMessages + m.InFlightMessages,
			Throughput:         0.0,
			PendingMessages:    m.PendingMessages,
			MessagesNotVisible: m.InFlightMessages,
		}
	}
	return &dashboardQueueStatsOutput{Body: out}, nil
}

type queueMetricsOutput struct {
	Body []QueueMetricsView
}

func (s *State) queueMetrics(_ context.Context, _ *emptyInput) (*queueMetricsOutput, error) {
	out := []QueueMetricsView{}
	if s.BrokerStats == nil {
		return &queueMetricsOutput{Body: out}, nil
	}
	for _, m := range s.BrokerStats.GetWindowed(0) {
		out = append(out, QueueMetricsView{
			QueueIdentifier:  m.QueueIdentifier,
			PendingMessages:  m.PendingMessages,
			InFlightMessages: m.InFlightMessages,
		})
	}
	return &queueMetricsOutput{Body: out}, nil
}

type dashboardBreakersOutput struct {
	Body map[string]DashboardCircuitBreaker
}

func (s *State) dashboardCircuitBreakers(_ context.Context, _ *emptyInput) (*dashboardBreakersOutput, error) {
	out := map[string]DashboardCircuitBreaker{}
	if s.Breakers == nil {
		return &dashboardBreakersOutput{Body: out}, nil
	}
	for name, b := range s.Breakers.Snapshot() {
		total := b.Successes + b.Failures
		rate := 0.0
		if total > 0 {
			rate = float64(b.Failures) / float64(total)
		}
		out[name] = DashboardCircuitBreaker{
			Name:            name,
			State:           strings.ToUpper(breakerStateString(b.State)),
			SuccessfulCalls: b.Successes,
			FailedCalls:     b.Failures,
			RejectedCalls:   0,
			FailureRate:     rate,
			BufferedCalls:   uint32(b.RecentFailures),
			BufferSize:      0,
		}
	}
	return &dashboardBreakersOutput{Body: out}, nil
}

func breakerStateString(s router.CircuitState) string {
	switch s {
	case router.CircuitClosed:
		return "closed"
	case router.CircuitOpen:
		return "open"
	case router.CircuitHalfOpen:
		return "halfOpen"
	default:
		return "unknown"
	}
}

type circuitBreakerStateInput struct {
	Name string `path:"name"`
}

type circuitBreakerStateOutput struct {
	Body CircuitBreakerStateResponse
}

func (s *State) circuitBreakerState(_ context.Context, in *circuitBreakerStateInput) (*circuitBreakerStateOutput, error) {
	if s.Breakers == nil {
		return nil, notConfigured("breakers")
	}
	snap := s.Breakers.Snapshot()
	st, ok := snap[in.Name]
	if !ok {
		return nil, huma.Error404NotFound("breaker not found: " + in.Name)
	}
	return &circuitBreakerStateOutput{Body: CircuitBreakerStateResponse{
		Name:           in.Name,
		State:          strings.ToUpper(breakerStateString(st.State)),
		Successes:      st.Successes,
		Failures:       st.Failures,
		RecentFailures: uint32(st.RecentFailures),
	}}, nil
}

type dashboardInFlightInput struct {
	Limit     int    `query:"limit"`
	MessageID string `query:"messageId"`
	PoolCode  string `query:"poolCode"`
}

type dashboardInFlightOutput struct {
	Body []InFlightMessageInfo
}

func (s *State) dashboardInFlight(_ context.Context, in *dashboardInFlightInput) (*dashboardInFlightOutput, error) {
	if s.InFlight == nil {
		return &dashboardInFlightOutput{Body: []InFlightMessageInfo{}}, nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = 100
	}
	idFilter := strings.ToLower(in.MessageID)
	poolFilter := strings.ToLower(in.PoolCode)
	now := time.Now()
	out := make([]InFlightMessageInfo, 0, limit)
	for _, im := range s.InFlight.Snapshot() {
		if idFilter != "" && !strings.Contains(strings.ToLower(im.MessageID), idFilter) {
			continue
		}
		if poolFilter != "" && !strings.EqualFold(im.PoolCode, poolFilter) {
			continue
		}
		var brokerID *string
		if im.BrokerMessageID != "" {
			b := im.BrokerMessageID
			brokerID = &b
		}
		out = append(out, InFlightMessageInfo{
			MessageID:           im.MessageID,
			BrokerMessageID:     brokerID,
			QueueID:             im.QueueIdentifier,
			PoolCode:            im.PoolCode,
			ElapsedTimeMs:       uint64(now.Sub(im.StartedAt).Milliseconds()),
			AddedToInPipelineAt: im.StartedAt.UTC(),
		})
		if len(out) >= limit {
			break
		}
	}
	return &dashboardInFlightOutput{Body: out}, nil
}

type inFlightCheckInput struct {
	MessageID string `query:"messageId" required:"true"`
}

type inFlightCheckOutput struct {
	Body InFlightCheckResponse
}

func (s *State) inFlightCheck(_ context.Context, in *inFlightCheckInput) (*inFlightCheckOutput, error) {
	if s.InFlight == nil {
		return &inFlightCheckOutput{Body: InFlightCheckResponse{MessageID: in.MessageID, InPipeline: false}}, nil
	}
	for _, im := range s.InFlight.Snapshot() {
		if im.MessageID == in.MessageID {
			return &inFlightCheckOutput{Body: InFlightCheckResponse{
				MessageID:  in.MessageID,
				InPipeline: true,
				PoolCode:   im.PoolCode,
				QueueID:    im.QueueIdentifier,
			}}, nil
		}
	}
	return &inFlightCheckOutput{Body: InFlightCheckResponse{MessageID: in.MessageID, InPipeline: false}}, nil
}

type inFlightCheckBatchInput struct {
	Body InFlightCheckBatchRequest
}

type inFlightCheckBatchOutput struct {
	Body map[string]bool
}

func (s *State) inFlightCheckBatch(_ context.Context, in *inFlightCheckBatchInput) (*inFlightCheckBatchOutput, error) {
	result := make(map[string]bool, len(in.Body.MessageIDs))
	for _, id := range in.Body.MessageIDs {
		result[id] = false
	}
	if s.InFlight == nil {
		return &inFlightCheckBatchOutput{Body: result}, nil
	}
	live := map[string]bool{}
	for _, im := range s.InFlight.Snapshot() {
		live[im.MessageID] = true
	}
	for _, id := range in.Body.MessageIDs {
		result[id] = live[id]
	}
	return &inFlightCheckBatchOutput{Body: result}, nil
}
