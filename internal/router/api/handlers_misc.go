package api

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/google/uuid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

const (
	tagConfig  = "config"
	tagStandby = "standby"
	tagSeed    = "seed"
	tagStream  = "stream"
)

func registerMisc(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "getLocalConfig", Method: http.MethodGet, Path: "/api/config",
		Summary: "Snapshot of router configuration", Tags: []string{tagConfig}, DefaultStatus: http.StatusOK,
	}, s.getLocalConfig)
	huma.Register(api, huma.Operation{
		OperationID: "configReload", Method: http.MethodPost, Path: "/config/reload",
		Summary: "Trigger a config refresh", Tags: []string{tagConfig}, DefaultStatus: http.StatusOK,
	}, s.configReload)
	huma.Register(api, huma.Operation{
		OperationID: "seedMessages", Method: http.MethodPost, Path: "/api/seed/messages",
		Summary: "Bulk publish synthetic messages (dev only)", Tags: []string{tagSeed}, DefaultStatus: http.StatusOK,
	}, s.seedMessages)
	huma.Register(api, huma.Operation{
		OperationID: "standbyStatus", Method: http.MethodGet, Path: "/monitoring/standby-status",
		Summary: "Leader-election status", Tags: []string{tagStandby}, DefaultStatus: http.StatusOK,
	}, s.standbyStatus)
	huma.Register(api, huma.Operation{
		OperationID: "trafficStatus", Method: http.MethodGet, Path: "/monitoring/traffic-status",
		Summary: "Traffic management status", Tags: []string{tagStandby}, DefaultStatus: http.StatusOK,
	}, s.trafficStatus)

	// Stream health. When the router is co-tenanted with the stream
	// processor (fc-server) a StreamHealthProvider is wired into State
	// and these endpoints reflect live state. Otherwise they report
	// NOT_CONFIGURED so probes still get 200.
	huma.Register(api, huma.Operation{
		OperationID: "streamHealth", Method: http.MethodGet, Path: "/monitoring/stream-health",
		Summary: "Stream processor health", Tags: []string{tagStream}, DefaultStatus: http.StatusOK,
	}, s.streamHealth)
	huma.Register(api, huma.Operation{
		OperationID: "streamLiveness", Method: http.MethodGet, Path: "/monitoring/stream-health/live",
		Summary: "Stream processor liveness", Tags: []string{tagStream}, DefaultStatus: http.StatusOK,
	}, s.streamLiveness)
	huma.Register(api, huma.Operation{
		OperationID: "streamReadiness", Method: http.MethodGet, Path: "/monitoring/stream-health/ready",
		Summary: "Stream processor readiness", Tags: []string{tagStream}, DefaultStatus: http.StatusOK,
	}, s.streamReadiness)
}

type streamHealthOutput struct {
	Body StreamHealthResponse
}

func (s *State) streamHealth(_ context.Context, _ *emptyInput) (*streamHealthOutput, error) {
	if s.StreamHealth == nil {
		return &streamHealthOutput{Body: StreamHealthResponse{
			Enabled: false,
			Status:  "NOT_CONFIGURED",
			Detail:  "no stream processor configured in this build",
		}}, nil
	}
	agg := s.StreamHealth.Aggregate()
	streams := make([]StreamProjectionHealth, 0, len(agg.Streams))
	for _, h := range agg.Streams {
		streams = append(streams, StreamProjectionHealth{
			Name:           h.Name,
			Status:         h.Status,
			Running:        h.Running,
			Healthy:        h.Healthy,
			BatchSequence:  h.BatchSequence,
			ErrorCount:     h.ErrorCount,
			LastPollTimeMs: h.LastPollTimeMs,
		})
	}
	status := "RUNNING"
	if !agg.Healthy {
		status = "DEGRADED"
	}
	if agg.TotalStreams == 0 {
		status = "NOT_CONFIGURED"
	}
	return &streamHealthOutput{Body: StreamHealthResponse{
		Enabled:          agg.TotalStreams > 0,
		Status:           status,
		TotalStreams:     agg.TotalStreams,
		HealthyStreams:   agg.HealthyStreams,
		UnhealthyStreams: agg.UnhealthyStreams,
		Streams:          streams,
	}}, nil
}

type streamProbeOutput struct {
	Status int
	Body   StreamProbeResponse
}

func (s *State) streamLiveness(_ context.Context, _ *emptyInput) (*streamProbeOutput, error) {
	if s.StreamHealth == nil {
		return &streamProbeOutput{
			Status: http.StatusOK,
			Body:   StreamProbeResponse{Status: "NOT_CONFIGURED"},
		}, nil
	}
	if s.StreamHealth.IsLive() {
		return &streamProbeOutput{Status: http.StatusOK, Body: StreamProbeResponse{Status: "LIVE"}}, nil
	}
	return &streamProbeOutput{
		Status: http.StatusServiceUnavailable,
		Body:   StreamProbeResponse{Status: "NOT_LIVE"},
	}, nil
}

func (s *State) streamReadiness(_ context.Context, _ *emptyInput) (*streamProbeOutput, error) {
	if s.StreamHealth == nil {
		return &streamProbeOutput{
			Status: http.StatusOK,
			Body:   StreamProbeResponse{Status: "NOT_CONFIGURED"},
		}, nil
	}
	if s.StreamHealth.IsReady() {
		return &streamProbeOutput{Status: http.StatusOK, Body: StreamProbeResponse{Status: "READY"}}, nil
	}
	return &streamProbeOutput{
		Status: http.StatusServiceUnavailable,
		Body:   StreamProbeResponse{Status: "NOT_READY"},
	}, nil
}

type localConfigOutput struct {
	Body LocalConfigResponse
}

func (s *State) getLocalConfig(_ context.Context, _ *emptyInput) (*localConfigOutput, error) {
	// Match Rust's get_local_config: never expose secrets. Report
	// version + warning counts only.
	return &localConfigOutput{Body: LocalConfigResponse{
		Version:          Version,
		WarningsTotal:    uint64(s.Warnings.Count()),
		WarningsCritical: uint64(s.Warnings.CriticalCount()),
	}}, nil
}

type configReloadOutput struct {
	Body ConfigReloadResponse
}

func (s *State) configReload(ctx context.Context, _ *emptyInput) (*configReloadOutput, error) {
	if s.Reloader == nil {
		// The router currently has a polling Watch loop; if no explicit
		// Reloader is wired we acknowledge but report no-op. 200 instead
		// of 501 to keep the dashboard's reload button working.
		return &configReloadOutput{Body: ConfigReloadResponse{Success: true, Note: "config watcher polls automatically"}}, nil
	}
	if err := s.Reloader.Reload(ctx); err != nil {
		return nil, huma.Error500InternalServerError("reload: " + err.Error())
	}
	return &configReloadOutput{Body: ConfigReloadResponse{Success: true}}, nil
}

type seedMessagesInput struct {
	Body SeedMessagesRequest
}

type seedMessagesOutput struct {
	Body SeedMessagesResponse
}

func (s *State) seedMessages(ctx context.Context, in *seedMessagesInput) (*seedMessagesOutput, error) {
	if s.Publisher == nil {
		return nil, notConfigured("publisher")
	}
	req := in.Body
	if req.PoolCode == "" {
		return nil, huma.Error400BadRequest("poolCode is required")
	}
	if req.Count <= 0 {
		return nil, huma.Error400BadRequest("count must be > 0")
	}
	if req.Count > 10000 {
		return nil, huma.Error400BadRequest("count must be <= 10000")
	}
	pub, err := s.Publisher.Publisher(ctx, req.PoolCode)
	if err != nil {
		return nil, huma.Error502BadGateway("publisher: " + err.Error())
	}
	target := req.MediationTarget
	if target == "" {
		target = "https://localhost:8080/api/test/fast"
	}

	msgs := make([]common.Message, 0, req.Count)
	for i := 0; i < req.Count; i++ {
		msgs = append(msgs, common.Message{
			ID:              uuid.NewString(),
			PoolCode:        req.PoolCode,
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: target,
			DispatchMode:    common.DispatchImmediate,
		})
	}
	ids, err := pub.PublishBatch(ctx, msgs)
	if err != nil {
		slog.Warn("seed publish failed", "pool", req.PoolCode, "err", err)
		return nil, huma.Error502BadGateway("publish batch: " + err.Error())
	}
	return &seedMessagesOutput{Body: SeedMessagesResponse{
		PoolCode:        req.PoolCode,
		QueueIdentifier: pub.Identifier(),
		Published:       len(ids),
	}}, nil
}

type standbyStatusOutput struct {
	Body StandbyStatusResponse
}

func (s *State) standbyStatus(_ context.Context, _ *emptyInput) (*standbyStatusOutput, error) {
	if s.Leader == nil {
		return &standbyStatusOutput{Body: StandbyStatusResponse{
			Enabled:    false,
			IsLeader:   true,
			InstanceID: "default",
		}}, nil
	}
	return &standbyStatusOutput{Body: StandbyStatusResponse{
		Enabled:    s.Leader.StandbyEnabled(),
		IsLeader:   s.Leader.IsLeader(),
		InstanceID: s.Leader.InstanceID(),
	}}, nil
}

type trafficStatusOutput struct {
	Body TrafficStatusResponse
}

func (s *State) trafficStatus(_ context.Context, _ *emptyInput) (*trafficStatusOutput, error) {
	if s.Traffic == nil {
		return &trafficStatusOutput{Body: TrafficStatusResponse{
			Enabled: false, Mode: "disabled",
		}}, nil
	}
	st := s.Traffic.Status()
	resp := TrafficStatusResponse{
		Enabled:     st.Enabled,
		Mode:        st.Mode,
		TargetGroup: st.TargetGroupARN,
		Registered:  st.Registered,
		LastError:   st.LastError,
	}
	if !st.LastChangedAt.IsZero() {
		resp.LastChangedAt = st.LastChangedAt.UTC().Format("2006-01-02T15:04:05.000Z")
	}
	return &trafficStatusOutput{Body: resp}, nil
}
