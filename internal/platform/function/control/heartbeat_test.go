//go:build integration

package control

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// TestHeartbeat_MarksReadyOnceAndBumpsRevision pins: a PUBLISHED version
// reported LOADED becomes READY (ready_at stamped) and the pool revision
// bumps; a second heartbeat reporting the SAME version LOADED again does
// not bump the revision a second time (already READY, "once" semantics).
func TestHeartbeat_MarksReadyOnceAndBumpsRevision(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "hbapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	v := seedVersion(t, s.Repo, fn.ID, 1, function.VersionPublished, "wasm", describeJSON(t))

	revBefore, _, err := s.Repo.GetPoolRevision(context.Background(), poolName)
	require.NoError(t, err)

	ctx := anchorCtx()
	_, err = s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{
		RunnerID: "runner-1", Pool: poolName,
		Versions: []fncontrol.VersionReport{{FunctionID: fn.ID, Number: 1, State: fncontrol.StateLoaded}},
	}})
	require.NoError(t, err)

	got, err := s.Repo.GetVersionByNumber(context.Background(), fn.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, function.VersionReady, got.Status)
	require.NotNil(t, got.ReadyAt)

	revAfter1, _, err := s.Repo.GetPoolRevision(context.Background(), poolName)
	require.NoError(t, err)
	assert.Greater(t, revAfter1, revBefore, "revision must bump on the PUBLISHED→READY transition")

	// Second heartbeat, same version, still LOADED: no further transition,
	// no further bump — "once".
	_, err = s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{
		RunnerID: "runner-1", Pool: poolName,
		Versions: []fncontrol.VersionReport{{FunctionID: fn.ID, Number: 1, State: fncontrol.StateLoaded}},
	}})
	require.NoError(t, err)
	revAfter2, _, err := s.Repo.GetPoolRevision(context.Background(), poolName)
	require.NoError(t, err)
	assert.Equal(t, revAfter1, revAfter2, "a version already READY must not bump the revision again")

	_ = v
}

// TestHeartbeat_FailedPath marks a PUBLISHED version FAILED with the
// reported reason, and bumps the revision.
func TestHeartbeat_FailedPath(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "hbfailapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	seedVersion(t, s.Repo, fn.ID, 1, function.VersionPublished, "wasm", describeJSON(t))

	ctx := anchorCtx()
	_, err := s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{
		RunnerID: "runner-1", Pool: poolName,
		Versions: []fncontrol.VersionReport{{FunctionID: fn.ID, Number: 1, State: fncontrol.StateFailed, Reason: "LOAD:IMPORT_NOT_ALLOWED"}},
	}})
	require.NoError(t, err)

	got, err := s.Repo.GetVersionByNumber(context.Background(), fn.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, function.VersionFailed, got.Status)
	require.NotEmpty(t, got.Failure)
	assert.Contains(t, string(got.Failure), "IMPORT_NOT_ALLOWED")
}

// TestHeartbeat_LiveVersionNeverDemoted: a READY (and, here, live) version
// reported FAILED by one runner is recorded in that runner's own heartbeat
// row only — its version-row status is left exactly as it was.
func TestHeartbeat_LiveVersionNeverDemoted(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	poolName := "pool-" + t.Name()
	appID := "app_" + shortID(t)
	appCode := "hbdemoteapp" + shortID(t)
	seedApplication(t, testpg.Pool(t), appID, appCode)
	fn := seedFunction(t, testpg.Pool(t), s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	revBefore, _, err := s.Repo.GetPoolRevision(context.Background(), poolName)
	require.NoError(t, err)

	ctx := anchorCtx()
	_, err = s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{
		RunnerID: "runner-flaky", Pool: poolName,
		Versions: []fncontrol.VersionReport{{FunctionID: fn.ID, Number: 1, State: fncontrol.StateFailed, Reason: "OOM on this one runner"}},
	}})
	require.NoError(t, err)

	got, err := s.Repo.GetVersionByNumber(context.Background(), fn.ID, 1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, function.VersionReady, got.Status, "a READY/live version must never be demoted by one runner's FAILED report")
	assert.Empty(t, got.Failure)

	revAfter, _, err := s.Repo.GetPoolRevision(context.Background(), poolName)
	require.NoError(t, err)
	assert.Equal(t, revBefore, revAfter, "no version transition happened, so no revision bump either")
}

func TestHeartbeat_RequiresRunnerIDAndPool(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ctx := anchorCtx()

	_, err := s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{Pool: "p"}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "RUNNER_ID_REQUIRED")

	_, err = s.heartbeat(ctx, &heartbeatInput{Body: fncontrol.Heartbeat{RunnerID: "r"}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "POOL_REQUIRED")
}
