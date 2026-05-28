package api

import (
	"context"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

const tagTest = "test"

// registerMocks wires the dev mock endpoints. They mirror Rust's
// /api/test/* and exist for two purposes: (1) load-testing the router
// against a controllable target, (2) verifying the dashboard renders
// outcomes correctly. Always-on; not gated by DevMode because the Rust
// router keeps them on too — the URL prefix makes their purpose clear.
func registerMocks(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID: "testFast", Method: http.MethodPost, Path: "/api/test/fast",
		Summary: "Always returns 200 immediately", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testFast)
	huma.Register(api, huma.Operation{
		OperationID: "testSlow", Method: http.MethodPost, Path: "/api/test/slow",
		Summary: "Returns 200 after a configurable delay", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testSlow)
	huma.Register(api, huma.Operation{
		OperationID: "testFaulty", Method: http.MethodPost, Path: "/api/test/faulty",
		Summary: "Random 50% success / 50% 500", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testFaulty)
	huma.Register(api, huma.Operation{
		OperationID: "testFail", Method: http.MethodPost, Path: "/api/test/fail",
		Summary: "Always returns 500", Tags: []string{tagTest}, DefaultStatus: http.StatusInternalServerError,
	}, s.testFail)
	huma.Register(api, huma.Operation{
		OperationID: "testSuccess", Method: http.MethodPost, Path: "/api/test/success",
		Summary: "Always returns 200 (alias for /fast)", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testSuccess)
	huma.Register(api, huma.Operation{
		OperationID: "testPending", Method: http.MethodPost, Path: "/api/test/pending",
		Summary: "Sleeps for a long time before responding 200", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testPending)
	huma.Register(api, huma.Operation{
		OperationID: "testClientError", Method: http.MethodPost, Path: "/api/test/client-error",
		Summary: "Always returns 400", Tags: []string{tagTest}, DefaultStatus: http.StatusBadRequest,
	}, s.testClientError)
	huma.Register(api, huma.Operation{
		OperationID: "testServerError", Method: http.MethodPost, Path: "/api/test/server-error",
		Summary: "Always returns 500", Tags: []string{tagTest}, DefaultStatus: http.StatusInternalServerError,
	}, s.testServerError)
	huma.Register(api, huma.Operation{
		OperationID: "testStats", Method: http.MethodGet, Path: "/api/test/stats",
		Summary: "Per-endpoint hit counters", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testStats)
	huma.Register(api, huma.Operation{
		OperationID: "testStatsReset", Method: http.MethodPost, Path: "/api/test/stats/reset",
		Summary: "Reset every counter to zero", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testStatsReset)

	// Java-compatible benchmark aliases.
	huma.Register(api, huma.Operation{
		OperationID: "benchmarkProcess", Method: http.MethodPost, Path: "/api/benchmark/process",
		Summary: "Alias for /api/test/fast", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testFast)
	huma.Register(api, huma.Operation{
		OperationID: "benchmarkProcessSlow", Method: http.MethodPost, Path: "/api/benchmark/process-slow",
		Summary: "Alias for /api/test/slow", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testSlow)
	huma.Register(api, huma.Operation{
		OperationID: "benchmarkStats", Method: http.MethodGet, Path: "/api/benchmark/stats",
		Summary: "Alias for /api/test/stats", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testStats)
	huma.Register(api, huma.Operation{
		OperationID: "benchmarkReset", Method: http.MethodPost, Path: "/api/benchmark/reset",
		Summary: "Alias for /api/test/stats/reset", Tags: []string{tagTest}, DefaultStatus: http.StatusOK,
	}, s.testStatsReset)
}

type testOKOutput struct {
	Body MockOKResponse
}

func (s *State) testFast(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.Fast.Add(1)
	return &testOKOutput{Body: MockOKResponse{OK: true, Endpoint: "fast"}}, nil
}

type testSlowInput struct {
	DelayMs int `query:"delay_ms"`
}

func (s *State) testSlow(ctx context.Context, in *testSlowInput) (*testOKOutput, error) {
	s.Mocks.Slow.Add(1)
	delay := time.Duration(in.DelayMs) * time.Millisecond
	if delay <= 0 || delay > 30*time.Second {
		delay = 500 * time.Millisecond
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(delay):
	}
	return &testOKOutput{Body: MockOKResponse{OK: true, Endpoint: "slow"}}, nil
}

func (s *State) testFaulty(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.Faulty.Add(1)
	if rand.IntN(2) == 0 {
		s.Mocks.FaultyFail.Add(1)
		return nil, huma.Error500InternalServerError("faulty endpoint randomly failed")
	}
	s.Mocks.FaultySuccess.Add(1)
	return &testOKOutput{Body: MockOKResponse{OK: true, Endpoint: "faulty"}}, nil
}

func (s *State) testFail(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.Fail.Add(1)
	return nil, huma.Error500InternalServerError("test/fail")
}

func (s *State) testSuccess(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.Success.Add(1)
	return &testOKOutput{Body: MockOKResponse{OK: true, Endpoint: "success"}}, nil
}

func (s *State) testPending(ctx context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.Pending.Add(1)
	// 30s sleep — long enough to exercise timeouts in the dispatcher
	// without hanging tests forever. Cancels on ctx.
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
	}
	return &testOKOutput{Body: MockOKResponse{OK: true, Endpoint: "pending"}}, nil
}

func (s *State) testClientError(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.ClientError.Add(1)
	return nil, huma.Error400BadRequest("test/client-error")
}

func (s *State) testServerError(_ context.Context, _ *emptyInput) (*testOKOutput, error) {
	s.Mocks.ServerError.Add(1)
	return nil, huma.Error500InternalServerError("test/server-error")
}

type testStatsOutput struct {
	Body MockStatsResponse
}

func (s *State) testStats(_ context.Context, _ *emptyInput) (*testStatsOutput, error) {
	return &testStatsOutput{Body: MockStatsResponse{
		Fast:          s.Mocks.Fast.Load(),
		Slow:          s.Mocks.Slow.Load(),
		Faulty:        s.Mocks.Faulty.Load(),
		FaultySuccess: s.Mocks.FaultySuccess.Load(),
		FaultyFail:    s.Mocks.FaultyFail.Load(),
		Fail:          s.Mocks.Fail.Load(),
		Success:       s.Mocks.Success.Load(),
		Pending:       s.Mocks.Pending.Load(),
		ClientError:   s.Mocks.ClientError.Load(),
		ServerError:   s.Mocks.ServerError.Load(),
	}}, nil
}

type testStatsResetOutput struct {
	Body ResetResponse
}

func (s *State) testStatsReset(_ context.Context, _ *emptyInput) (*testStatsResetOutput, error) {
	s.Mocks.Reset()
	return &testStatsResetOutput{Body: ResetResponse{Reset: true}}, nil
}
