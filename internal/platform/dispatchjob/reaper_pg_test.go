//go:build integration

package dispatchjob_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// reapSeedOpts customizes reapSeedJob beyond the required fields.
type reapSeedOpts struct {
	Mode     common.DispatchMode
	Sequence int32
}

// reapSeedJob writes a job through the production Insert path (see
// entity.go's package doc: this is the only way dispatch-job rows are
// created) and returns its id.
func reapSeedJob(t *testing.T, repo *dispatchjob.Repository, code, group string, status common.DispatchStatus, opts reapSeedOpts) string {
	t.Helper()
	j := &dispatchjob.DispatchJob{
		// msg_dispatch_jobs.id is VARCHAR(13) — an unprefixed raw TSID, not
		// the typed/prefixed 17-char form (see the production idiom in
		// internal/platform/shared/sdk/dispatch_jobs_batch.go).
		ID:                 tsid.GenerateUntyped(),
		Kind:               dispatchjob.KindEvent,
		Code:               code,
		TargetURL:          "http://example.invalid/hook",
		Protocol:           dispatchjob.ProtocolHTTPWebhook,
		PayloadContentType: "application/json",
		Mode:               opts.Mode,
		MessageGroup:       &group,
		Sequence:           opts.Sequence,
		TimeoutSeconds:     30,
		MaxRetries:         3,
		RetryStrategy:      dispatchjob.RetryExponentialBackoff,
		Status:             status,
	}
	if status == common.DispatchFailed {
		now := time.Now().UTC()
		j.CompletedAt = &now
	}
	require.NoError(t, repo.Insert(context.Background(), j))
	return j.ID
}

// backdateUpdatedAt pushes a row's updated_at into the past — used to
// simulate a PROCESSING job that's been sitting there long enough that the
// reaper's liveness heuristic should stop treating it as in flight.
func backdateUpdatedAt(t *testing.T, pool *pgxpool.Pool, id string, age time.Duration) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET updated_at = $2 WHERE id = $1`,
		id, time.Now().Add(-age).UTC())
	require.NoError(t, err)
}

func statusOf(t *testing.T, repo *dispatchjob.Repository, id string) common.DispatchStatus {
	t.Helper()
	j, err := repo.FindByID(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, j)
	return j.Status
}

// TestSweepStrandedGroupSiblings_BlockOnError is the reaper's core contract:
// a QUEUED sibling behind a FAILED BLOCK_ON_ERROR head is reset regardless
// of age; a PROCESSING sibling is reset only once it's older than the
// liveness cutoff (a fresher one is presumed genuinely in flight and left
// alone); a sibling in a group with no FAILED head, and a sibling under a
// non-BLOCK_ON_ERROR mode, are both left alone.
func TestSweepStrandedGroupSiblings_BlockOnError(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	group := "reaptest-group-" + tsid.Generate(tsid.DispatchJob)
	const code = "reaptest:sweep"

	head := reapSeedJob(t, repo, code, group, common.DispatchFailed,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 1})
	queuedSibling := reapSeedJob(t, repo, code, group, common.DispatchQueued,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 2})
	staleProcessing := reapSeedJob(t, repo, code, group, common.DispatchProcessing,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 3})
	backdateUpdatedAt(t, pool, staleProcessing, time.Hour)
	freshProcessing := reapSeedJob(t, repo, code, group, common.DispatchProcessing,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 4})

	// Control: a sibling positioned BEFORE the head is never "behind" a
	// holder and must be left alone regardless of status.
	beforeHead := reapSeedJob(t, repo, code, group, common.DispatchQueued,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 0})

	// Control: same shape, but NEXT_ON_ERROR — the mode that explicitly does
	// NOT block on a failed sibling — must never be swept.
	otherGroup := "reaptest-group-nxt-" + tsid.Generate(tsid.DispatchJob)
	reapSeedJob(t, repo, "reaptest:sweep:nexthead", otherGroup, common.DispatchFailed,
		reapSeedOpts{Mode: common.DispatchNextOnError, Sequence: 1})
	nextOnErrorSibling := reapSeedJob(t, repo, "reaptest:sweep:nextsib", otherGroup, common.DispatchQueued,
		reapSeedOpts{Mode: common.DispatchNextOnError, Sequence: 2})

	ids, err := repo.SweepStrandedGroupSiblings(context.Background(), 30*time.Minute)
	require.NoError(t, err)
	// Assert on each row's resulting STATUS, not on which sweep call
	// returned its id. The sweep is global — it takes no group parameter —
	// so with t.Parallel() a concurrently running test's sweep can legally
	// reset this test's rows first, and then they are absent from OUR
	// returned ids. Status is what the contract is actually about and is
	// unaffected by which caller did the work. (An ElementsMatch on the
	// returned ids additionally required that NOTHING else in the database
	// was stranded, which no parallel test can guarantee.)
	//
	// The negative assertions stay on the id list too: a row that must
	// never be swept must not appear in ANY sweep's result.
	assert.NotContains(t, ids, freshProcessing)
	assert.NotContains(t, ids, beforeHead)
	assert.NotContains(t, ids, head)

	assert.Equal(t, common.DispatchPending, statusOf(t, repo, queuedSibling))
	assert.Equal(t, common.DispatchPending, statusOf(t, repo, staleProcessing))
	assert.Equal(t, common.DispatchProcessing, statusOf(t, repo, freshProcessing), "a recently-updated PROCESSING row must be presumed live and left alone")
	assert.Equal(t, common.DispatchQueued, statusOf(t, repo, beforeHead), "a sibling positioned before the head must never be touched")
	assert.Equal(t, common.DispatchFailed, statusOf(t, repo, head), "the FAILED head itself is never touched by the reaper")
	assert.Equal(t, common.DispatchQueued, statusOf(t, repo, nextOnErrorSibling), "NEXT_ON_ERROR never blocks on a failed sibling, so the reaper must not touch it")

	got, err := repo.FindByID(context.Background(), queuedSibling)
	require.NoError(t, err)
	require.NotNil(t, got.LastError)
	assert.Contains(t, *got.LastError, "reaper", "last_error must record that the reaper (not the hook or a human) reset this row")
}

// TestSweepStrandedGroupSiblings_Idempotent proves a second sweep with
// nothing left to catch is a no-op — the settled hook and the reaper share
// the same "status IN (QUEUED, PROCESSING)" guard, so running both back to
// back must never double-act on a row.
func TestSweepStrandedGroupSiblings_Idempotent(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	group := "reaptest-group-idem-" + tsid.Generate(tsid.DispatchJob)
	const code = "reaptest:idempotent"
	reapSeedJob(t, repo, code, group, common.DispatchFailed,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 1})
	sibling := reapSeedJob(t, repo, code, group, common.DispatchQueued,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 2})

	// Status again, not the first sweep's id list — a parallel test's sweep
	// may reset this row before ours runs, which is legal and not a failure.
	// The idempotency claim is entirely about the SECOND sweep below.
	_, err := repo.SweepStrandedGroupSiblings(context.Background(), 30*time.Minute)
	require.NoError(t, err)
	require.Equal(t, common.DispatchPending, statusOf(t, repo, sibling))

	second, err := repo.SweepStrandedGroupSiblings(context.Background(), 30*time.Minute)
	require.NoError(t, err)
	assert.NotContains(t, second, sibling, "a row already reset to PENDING must not be re-swept")
}

// forceLegacyErrorStatus sets a row's status to the legacy 'ERROR' value.
// It cannot be seeded through the entity: 'ERROR' predates the current
// status set and has no common.DispatchStatus constant. It IS, however, an
// accepted legacy alias in common.ParseDispatchStatus (X-06: maps to
// DispatchFailed, same as the migration 052 CHECK constraint on
// msg_dispatch_jobs.status admits it) — a row read back after this helper
// runs is FAILED, not corrupt.
func forceLegacyErrorStatus(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE msg_dispatch_jobs SET status = 'ERROR' WHERE id = $1`, id)
	require.NoError(t, err)
}

// TestSweepStrandedGroupSiblings_LegacyErrorHead pins that the reaper
// recognises BOTH terminal-failure statuses, not just 'FAILED'.
//
// dispatchjob.GroupHoldingStatusSQL — the claim-time gate the scheduler's
// poller uses — holds a group on `status IN ('FAILED', 'ERROR')`, matching
// 'ERROR' deliberately "so old rows keep blocking as they always did". A
// reaper that swept only behind a 'FAILED' head would therefore leave
// siblings behind a legacy 'ERROR' head held forever at claim time and
// never reset here: permanently stranded, which is the exact failure this
// backstop exists to prevent. The two gates must agree on what holds a
// group.
func TestSweepStrandedGroupSiblings_LegacyErrorHead(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	group := "reaptest-group-legacy-" + tsid.Generate(tsid.DispatchJob)
	const code = "reaptest:legacyerror"

	head := reapSeedJob(t, repo, code, group, common.DispatchFailed,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 1})
	forceLegacyErrorStatus(t, pool, head)

	sibling := reapSeedJob(t, repo, code, group, common.DispatchQueued,
		reapSeedOpts{Mode: common.DispatchBlockOnError, Sequence: 2})

	// Status, not the returned id list — see the note in
	// TestSweepStrandedGroupSiblings_BlockOnError: the sweep is global, so a
	// parallel test's sweep may reset this row first and it would then be
	// absent from our own result.
	_, err := repo.SweepStrandedGroupSiblings(context.Background(), 30*time.Minute)
	require.NoError(t, err)
	assert.Equal(t, common.DispatchPending, statusOf(t, repo, sibling),
		"a sibling behind a legacy ERROR head must be swept, exactly as behind a FAILED head")
}
