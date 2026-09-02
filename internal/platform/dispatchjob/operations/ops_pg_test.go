//go:build integration

package operations_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// runAuthorized drives op through the full use-case envelope as an anchor
// principal — mirrors connection/operations/ops_pg_test.go's helper of the
// same name.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// seedOpts customizes seedJob beyond the required id/code/status.
type seedOpts struct {
	ClientID     *string
	Mode         common.DispatchMode
	MessageGroup *string
	Sequence     int32
	AttemptCount int32
	LastError    *string
}

// seedJob writes a dispatch job straight through the production Insert path
// (the only way dispatch jobs are created — see entity.go's package doc) so
// these tests exercise the same row shape the router/scheduler produce.
func seedJob(t *testing.T, repo *dispatchjob.Repository, code string, status common.DispatchStatus, opts seedOpts) *dispatchjob.DispatchJob {
	t.Helper()
	j := &dispatchjob.DispatchJob{
		// msg_dispatch_jobs.id is VARCHAR(13) — an UNPREFIXED raw TSID (see
		// internal/platform/shared/sdk/dispatch_jobs_batch.go's
		// tsid.GenerateUntyped() call for the production idiom). A typed,
		// prefixed id (tsid.Generate(tsid.DispatchJob), 17 chars) overflows
		// the column.
		ID:                 tsid.GenerateUntyped(),
		Kind:               dispatchjob.KindEvent,
		Code:               code,
		TargetURL:          "http://example.invalid/hook",
		Protocol:           dispatchjob.ProtocolHTTPWebhook,
		PayloadContentType: "application/json",
		Mode:               common.DispatchNextOnError,
		TimeoutSeconds:     30,
		MaxRetries:         3,
		RetryStrategy:      dispatchjob.RetryExponentialBackoff,
		Status:             status,
		ClientID:           opts.ClientID,
		MessageGroup:       opts.MessageGroup,
		Sequence:           opts.Sequence,
		AttemptCount:       opts.AttemptCount,
		LastError:          opts.LastError,
	}
	if opts.Mode != "" {
		j.Mode = opts.Mode
	}
	if status == common.DispatchFailed {
		now := time.Now().UTC()
		j.CompletedAt = &now
	}
	require.NoError(t, repo.Insert(context.Background(), j))
	return j
}

// ── Cancel ────────────────────────────────────────────────────────────────

func TestCancelDispatchJob_HappyPath(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	j := seedJob(t, repo, "opstest:cancel:happy", common.DispatchFailed, seedOpts{})

	ev, err := runAuthorized(uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: j.ID})
	require.NoError(t, err)
	assert.Equal(t, j.ID, ev.DispatchJobID)

	got, err := repo.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.DispatchCancelled, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestCancelDispatchJob_NotFound(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: "dpj_doesnotexist01"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "DispatchJob_NOT_FOUND")
}

// TestCancelDispatchJob_NonFailedSource_Conflict pins the T3 brief's explicit
// guard: Cancel/Complete are terminal→terminal operator OVERRIDES, valid
// only from FAILED. DispatchStatus.IsTerminal() already counts FAILED as
// terminal, so without this guard an operator could "cancel" an
// already-COMPLETED or already-CANCELLED job.
func TestCancelDispatchJob_NonFailedSource_Conflict(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	statuses := []common.DispatchStatus{
		common.DispatchPending, common.DispatchQueued, common.DispatchProcessing,
		common.DispatchCompleted, common.DispatchCancelled, common.DispatchExpired,
	}
	for _, st := range statuses {
		st := st
		t.Run(string(st), func(t *testing.T) {
			t.Parallel()
			j := seedJob(t, repo, "opstest:cancel:conflict:"+string(st), st, seedOpts{})
			_, err := runAuthorized(uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: j.ID})
			testpg.RequireUsecaseError(t, err, usecase.KindConflict, "NOT_FAILED")
		})
	}
}

func TestCancelDispatchJob_ResourceScope(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ownClient := tsid.Generate(tsid.Client)
	otherClient := tsid.Generate(tsid.Client)
	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_djcancelscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
	})

	// Bound to another tenant → denied.
	other := seedJob(t, repo, "opstest:cancel:scope:other", common.DispatchFailed, seedOpts{ClientID: &otherClient})
	_, err := usecaseop.Run(clientCtx, uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: other.ID}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Platform-scoped (nil ClientID) → cross-client → anchor required → denied.
	platform := seedJob(t, repo, "opstest:cancel:scope:platform", common.DispatchFailed, seedOpts{})
	_, err = usecaseop.Run(clientCtx, uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: platform.ID}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "SCOPE_FORBIDDEN")

	// Bound to the principal's own client → allowed.
	own := seedJob(t, repo, "opstest:cancel:scope:own", common.DispatchFailed, seedOpts{ClientID: &ownClient})
	ev, err := usecaseop.Run(clientCtx, uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: own.ID}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, own.ID, ev.DispatchJobID)
}

// ── Complete ──────────────────────────────────────────────────────────────

func TestCompleteDispatchJob_HappyPath(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	j := seedJob(t, repo, "opstest:complete:happy", common.DispatchFailed, seedOpts{})

	ev, err := runAuthorized(uow, operations.CompleteDispatchJob(repo), operations.CompleteCommand{ID: j.ID})
	require.NoError(t, err)
	assert.Equal(t, j.ID, ev.DispatchJobID)

	got, err := repo.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.DispatchCompleted, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

func TestCompleteDispatchJob_NonFailedSource_Conflict(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	j := seedJob(t, repo, "opstest:complete:conflict", common.DispatchPending, seedOpts{})

	_, err := runAuthorized(uow, operations.CompleteDispatchJob(repo), operations.CompleteCommand{ID: j.ID})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "NOT_FAILED")
}

// ── Resend ────────────────────────────────────────────────────────────────

func TestResendDispatchJobs_Validation(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.ResendDispatchJobs(repo), operations.ResendCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "IDS_REQUIRED")
}

// TestResendDispatchJobs_HappyPath resets a FAILED job's attempt_count,
// scheduled_for and last_error, and unknown ids are silently dropped from
// the result (mirrors the pre-envelope Requeue's no-op-on-unknown-id
// behaviour).
func TestResendDispatchJobs_HappyPath(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	errMsg := "boom"
	future := time.Now().Add(time.Hour)
	j := seedJob(t, repo, "opstest:resend:happy", common.DispatchFailed, seedOpts{
		AttemptCount: 3, LastError: &errMsg,
	})
	// backdate scheduled_for via a second retry-schedule call so the row
	// actually carries a non-nil value to prove Resend clears it.
	require.NoError(t, repo.ScheduleRetry(context.Background(), j.ID, j.CreatedAt, future, &errMsg))

	ev, err := runAuthorized(uow, operations.ResendDispatchJobs(repo),
		operations.ResendCommand{IDs: []string{j.ID, "dpj_doesnotexist02"}})
	require.NoError(t, err)
	assert.Equal(t, []string{j.ID}, ev.IDs, "unknown id must be silently dropped from the result")

	got, err := repo.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.DispatchPending, got.Status)
	assert.Equal(t, int32(0), got.AttemptCount, "attempt_count must reset to a full budget")
	assert.Nil(t, got.ScheduledFor)
	assert.Nil(t, got.LastError)
	assert.Nil(t, got.CompletedAt)
}

// TestResendDispatchJobs_ScopeFiltering pins the bulk endpoint's preserved
// security contract: ids outside the caller's scope are silently dropped
// from the batch (not a 403 for the whole request) — see resend.go's doc
// comment for why this deliberately differs from the single-resource ops.
func TestResendDispatchJobs_ScopeFiltering(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ownClient := tsid.Generate(tsid.Client)
	otherClient := tsid.Generate(tsid.Client)
	own := seedJob(t, repo, "opstest:resend:scope:own", common.DispatchFailed, seedOpts{ClientID: &ownClient})
	other := seedJob(t, repo, "opstest:resend:scope:other", common.DispatchFailed, seedOpts{ClientID: &otherClient})
	platform := seedJob(t, repo, "opstest:resend:scope:platform", common.DispatchFailed, seedOpts{})

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_djresendscope1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
	})

	ev, err := usecaseop.Run(clientCtx, uow, operations.ResendDispatchJobs(repo),
		operations.ResendCommand{IDs: []string{own.ID, other.ID, platform.ID}}, testpg.TestEC())
	require.NoError(t, err, "the whole batch must not fail just because some ids are out of scope")
	assert.Equal(t, []string{own.ID}, ev.IDs)

	gotOther, err := repo.FindByID(context.Background(), other.ID)
	require.NoError(t, err)
	assert.Equal(t, common.DispatchFailed, gotOther.Status, "out-of-scope job must be left untouched")

	gotPlatform, err := repo.FindByID(context.Background(), platform.ID)
	require.NoError(t, err)
	assert.Equal(t, common.DispatchFailed, gotPlatform.Status, "platform-scoped job must be left untouched for a non-anchor")
}
