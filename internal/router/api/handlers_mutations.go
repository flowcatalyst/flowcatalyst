package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func registerMutations(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "updatePoolConfig", Method: http.MethodPut, Path: "/monitoring/pools/{poolCode}",
		Summary: "Hot-update a pool's concurrency / rate limit", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.updatePoolConfig)
	huma.Register(api, huma.Operation{
		OperationID: "brokerStatsRefresh", Method: http.MethodPost, Path: "/monitoring/broker-stats/refresh",
		Summary: "Trigger an immediate SQS attribute refresh", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.brokerStatsRefresh)
	huma.Register(api, huma.Operation{
		OperationID: "resetBreaker", Method: http.MethodPost, Path: "/monitoring/circuit-breakers/{name}/reset",
		Summary: "Reset a single circuit breaker", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.resetBreaker)
	huma.Register(api, huma.Operation{
		OperationID: "resetAllBreakers", Method: http.MethodPost, Path: "/monitoring/circuit-breakers/reset-all",
		Summary: "Reset every circuit breaker", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.resetAllBreakers)
	huma.Register(api, huma.Operation{
		OperationID: "monitoringAcknowledgeWarning", Method: http.MethodPost, Path: "/monitoring/warnings/{id}/acknowledge",
		Summary: "Acknowledge a warning (dashboard alias)", Tags: []string{tagMonitoring}, DefaultStatus: http.StatusOK,
	}, s.acknowledgeWarning)
	huma.Register(api, huma.Operation{
		OperationID: "inFlightForceAck", Method: http.MethodPost, Path: "/monitoring/in-flight-messages/{messageId}/ack",
		Summary:       "Force-ACK an in-flight message (operator override)",
		Description:   "Deletes the broker copy (freshest receipt handle) and releases the tracker entry so future redeliveries/requeues are processed instead of being ACK-dropped as duplicates. Does not abort a delivery attempt already running in a worker.",
		Tags:          []string{tagMonitoring},
		DefaultStatus: http.StatusOK,
	}, s.inFlightForceAck)
}

type inFlightForceAckInput struct {
	MessageID string `path:"messageId"`
}

type inFlightForceAckOutput struct {
	Body ForceAckResponse
}

func (s *State) inFlightForceAck(ctx context.Context, in *inFlightForceAckInput) (*inFlightForceAckOutput, error) {
	if s.InFlightAck == nil {
		return nil, notConfigured("in-flight ack")
	}
	// Capture the mediating state BEFORE clearing the entry, so the response
	// can warn that a live delivery attempt will still run to completion.
	wasMediating := false
	if s.Mediating != nil {
		for _, e := range s.Mediating.MediatingSnapshot() {
			if e.MessageID == in.MessageID {
				wasMediating = true
				break
			}
		}
	}
	res, found := s.InFlightAck.ForceAckInFlight(ctx, in.MessageID)
	if !found {
		return nil, huma.Error404NotFound("message not in pipeline: " + in.MessageID)
	}
	out := ForceAckResponse{
		MessageID:     in.MessageID,
		Removed:       true,
		BrokerAcked:   res.AckErr == nil,
		QueueID:       res.Entry.QueueIdentifier,
		PoolCode:      res.Entry.PoolCode,
		ElapsedTimeMs: uint64(res.Entry.ElapsedSeconds()) * 1000,
		WasMediating:  wasMediating,
	}
	if res.AckErr != nil {
		out.BrokerAckError = res.AckErr.Error()
	}
	return &inFlightForceAckOutput{Body: out}, nil
}

type updatePoolConfigInput struct {
	PoolCode string `path:"poolCode"`
	Body     PoolConfigUpdateRequest
}

type updatePoolConfigOutput struct {
	Body PoolConfigUpdateResponse
}

func (s *State) updatePoolConfig(_ context.Context, in *updatePoolConfigInput) (*updatePoolConfigOutput, error) {
	if s.PoolUpdater == nil {
		return nil, notConfigured("pool updater")
	}
	var concurrency uint32
	if in.Body.Concurrency != nil {
		concurrency = *in.Body.Concurrency
	}
	setRate := in.Body.RateLimitPerMinute != nil
	if !s.PoolUpdater.UpdatePool(in.PoolCode, concurrency, in.Body.RateLimitPerMinute, setRate) {
		return nil, huma.Error404NotFound("pool not found or update rejected: " + in.PoolCode)
	}
	slog.Info("pool config updated via API",
		"pool", in.PoolCode, "concurrency", concurrency, "rate_limit", in.Body.RateLimitPerMinute)
	return &updatePoolConfigOutput{Body: PoolConfigUpdateResponse{
		Success:  true,
		PoolCode: in.PoolCode,
		NewConfig: PoolConfigUpdateNewConfig{
			Concurrency:        in.Body.Concurrency,
			RateLimitPerMinute: in.Body.RateLimitPerMinute,
		},
	}}, nil
}

type brokerStatsRefreshOutput struct {
	Body BrokerStatsRefreshResponse
}

func (s *State) brokerStatsRefresh(_ context.Context, _ *emptyInput) (*brokerStatsRefreshOutput, error) {
	if s.BrokerStats == nil {
		return nil, notConfigured("broker stats")
	}
	s.BrokerStats.Refresh()
	age := s.BrokerStats.AgeSeconds()
	if age < 0 {
		age = 0
	}
	return &brokerStatsRefreshOutput{Body: BrokerStatsRefreshResponse{
		Refreshed:  true,
		AgeSeconds: age,
	}}, nil
}

type resetBreakerInput struct {
	Name string `path:"name"`
}

type resetBreakerOutput struct {
	Body BreakerResetResponse
}

func (s *State) resetBreaker(_ context.Context, in *resetBreakerInput) (*resetBreakerOutput, error) {
	if s.Breakers == nil {
		return nil, notConfigured("breakers")
	}
	if !s.Breakers.Reset(in.Name) {
		return nil, huma.Error404NotFound("breaker not found: " + in.Name)
	}
	return &resetBreakerOutput{Body: BreakerResetResponse{Reset: true, Name: in.Name}}, nil
}

type resetAllBreakersOutput struct {
	Body BreakerResetAllResponse
}

func (s *State) resetAllBreakers(_ context.Context, _ *emptyInput) (*resetAllBreakersOutput, error) {
	if s.Breakers == nil {
		return nil, notConfigured("breakers")
	}
	n := s.Breakers.ResetAll()
	return &resetAllBreakersOutput{Body: BreakerResetAllResponse{Reset: uint64(n)}}, nil
}
