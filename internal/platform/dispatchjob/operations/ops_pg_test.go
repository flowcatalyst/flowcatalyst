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
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
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

// seedJob writes a dispatch job straight through the lifecycle
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
	// Born PENDING through the lifecycle (the only creator), then put into the
	// status under test with a test-only write: the lifecycle deliberately has
	// no way to create a job in an arbitrary status.
	_, cerr := repo.Lifecycle().CreateBatch(context.Background(), []dispatchjob.DispatchJob{*j})
	require.NoError(t, cerr)
	_, uerr := testpg.Pool(t).Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = $2, completed_at = $3 WHERE id = $1`,
		j.ID, string(status), j.CompletedAt)
	require.NoError(t, uerr)
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

// TestCancelDispatchJob_FromPending: an operator may ignore a job that is
// still waiting (owner ruling 2026-10-30), not only a FAILED one.
func TestCancelDispatchJob_FromPending(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	j := seedJob(t, repo, "opstest:cancel:pending", common.DispatchPending, seedOpts{})

	_, err := runAuthorized(uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: j.ID})
	require.NoError(t, err)

	got, err := repo.FindByID(context.Background(), j.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.DispatchCancelled, got.Status)
	assert.NotNil(t, got.CompletedAt)
}

// TestCancelDispatchJob_NotCancellable_Conflict pins the guard: Cancel is
// valid only from PENDING or FAILED. Without it an operator could "cancel"
// an already-COMPLETED job, or one whose delivery is under way.
func TestCancelDispatchJob_NotCancellable_Conflict(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	statuses := []common.DispatchStatus{
		common.DispatchQueued, common.DispatchProcessing,
		common.DispatchCompleted, common.DispatchCancelled, common.DispatchExpired,
	}
	for _, st := range statuses {
		t.Run(string(st), func(t *testing.T) {
			t.Parallel()
			j := seedJob(t, repo, "opstest:cancel:conflict:"+string(st), st, seedOpts{})
			_, err := runAuthorized(uow, operations.CancelDispatchJob(repo), operations.CancelCommand{ID: j.ID})
			testpg.RequireUsecaseError(t, err, usecase.KindConflict, "NOT_CANCELLABLE")
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

// ── Cancel many ("Ignore selected") ───────────────────────────────────────

func TestCancelDispatchJobs_Validation(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.CancelDispatchJobs(repo), operations.CancelManyCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "IDS_REQUIRED")
}

// TestCancelDispatchJobs_CancelsPendingAndFailedSkipsTheRest: PENDING and
// FAILED jobs become CANCELLED; QUEUED, PROCESSING and COMPLETED ones are
// left as they are and reported skipped; unknown ids are dropped.
func TestCancelDispatchJobs_CancelsPendingAndFailedSkipsTheRest(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	pending := seedJob(t, repo, "opstest:cancelmany:pending", common.DispatchPending, seedOpts{})
	failed := seedJob(t, repo, "opstest:cancelmany:failed", common.DispatchFailed, seedOpts{})
	queued := seedJob(t, repo, "opstest:cancelmany:queued", common.DispatchQueued, seedOpts{})
	processing := seedJob(t, repo, "opstest:cancelmany:processing", common.DispatchProcessing, seedOpts{})
	completed := seedJob(t, repo, "opstest:cancelmany:completed", common.DispatchCompleted, seedOpts{})

	ev, err := runAuthorized(uow, operations.CancelDispatchJobs(repo), operations.CancelManyCommand{
		IDs: []string{pending.ID, failed.ID, queued.ID, processing.ID, completed.ID, "dpj_doesnotexist03"},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{pending.ID, failed.ID}, ev.IDs)
	skipped := map[string]string{}
	for _, s := range ev.Skipped {
		assert.Equal(t, dispatchjob.SkipNotCancellable, s.Reason)
		skipped[s.ID] = s.Status
	}
	assert.Equal(t, map[string]string{
		queued.ID: "QUEUED", processing.ID: "PROCESSING", completed.ID: "COMPLETED",
	}, skipped)

	for id, want := range map[string]common.DispatchStatus{
		pending.ID: common.DispatchCancelled, failed.ID: common.DispatchCancelled,
		queued.ID: common.DispatchQueued, processing.ID: common.DispatchProcessing, completed.ID: common.DispatchCompleted,
	} {
		got, err := repo.FindByID(context.Background(), id)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, want, got.Status, id)
	}
}

// TestCancelDispatchJobs_ScopeFiltering: the bulk contract drops ids outside
// the caller's scope silently, as the bulk requeue does.
func TestCancelDispatchJobs_ScopeFiltering(t *testing.T) {
	t.Parallel()
	repo := dispatchjob.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ownClient := tsid.Generate(tsid.Client)
	otherClient := tsid.Generate(tsid.Client)
	own := seedJob(t, repo, "opstest:cancelmany:scope:own", common.DispatchFailed, seedOpts{ClientID: &ownClient})
	other := seedJob(t, repo, "opstest:cancelmany:scope:other", common.DispatchFailed, seedOpts{ClientID: &otherClient})

	clientCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_djcancelmany1",
		Scope:       auth.ScopeClient,
		Clients:     []string{ownClient},
	})
	ev, err := usecaseop.Run(clientCtx, uow, operations.CancelDispatchJobs(repo),
		operations.CancelManyCommand{IDs: []string{own.ID, other.ID}}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, []string{own.ID}, ev.IDs)

	got, err := repo.FindByID(context.Background(), other.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, common.DispatchFailed, got.Status, "another tenant's job is untouched")
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

// The operator's cancel/complete check "is FAILED" in the SQL, not only in
// Execute: if the job moved between the load and the write (a delivery that
// finished it), the persister refuses, so the use-case transaction rolls back
// and no event or audit row is written for a change that did not happen.
func TestOperatorPersisters_RefuseNonFailedInSQL(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)
	ctx := context.Background()
	for name, p := range map[string]usecasepgx.Persist[dispatchjob.DispatchJob]{
		"cancel":   repo.CancelPersister(),
		"complete": repo.CompletePersister(),
	} {
		j := seedJob(t, repo, "opstest:persist:"+name, common.DispatchCompleted, seedOpts{})
		tx, err := pool.Begin(ctx)
		require.NoError(t, err)
		err = p.Persist(ctx, j, usecasepgx.WrapTxForBootstrap(tx))
		require.ErrorIs(t, err, dispatchjob.ErrTransitionRefused, name)
		require.NoError(t, tx.Rollback(ctx))

		got, err := repo.FindByID(ctx, j.ID)
		require.NoError(t, err)
		assert.Equal(t, common.DispatchCompleted, got.Status, name)
	}
}
