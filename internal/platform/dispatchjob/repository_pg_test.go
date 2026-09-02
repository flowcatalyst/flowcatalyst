//go:build integration

package dispatchjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindWithFilters_TenantScoping pins the SQL-side AccessibleClientIDs
// enforcement on the dispatch-job list — same contract as the event repo: a
// non-anchor's caller-controlled clientId filters narrow within its own
// tenants (plus platform-scoped rows), never across.
func TestFindWithFilters_TenantScoping(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	const (
		code    = "scopetest:jobs:list" // unique code so assertions see only our rows
		clientA = "clt_scopejob0001"
		clientB = "clt_scopejob0002"
	)
	// FindWithFilters reads the msg_dispatch_jobs_read projection, so seed
	// there directly (the projector isn't running in this unit test). The
	// projection has fewer column defaults than the write table, so the
	// NOT NULL columns are supplied explicitly.
	seed := func(id string, clientID *string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs_read
			     (id, code, target_url, client_id, kind, protocol, mode, status, max_retries, updated_at)
			 VALUES ($1, $2, 'http://example.invalid/hook', $3,
			         'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'PENDING', 3, NOW())`,
			id, code, clientID)
		require.NoError(t, err)
	}
	a, b := clientA, clientB
	seed("djscopetest01", &a)  // tenant A
	seed("djscopetest02", &b)  // tenant B
	seed("djscopetest03", nil) // platform-scoped

	ids := func(rows []dispatchjob.DispatchJob) []string {
		out := make([]string, 0, len(rows))
		for i := range rows {
			out = append(out, rows[i].ID)
		}
		return out
	}

	// Anchor (no scoping): all three.
	rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"djscopetest01", "djscopetest02", "djscopetest03"}, ids(rows))

	// Non-anchor with access to A: own tenant + platform-scoped, never B.
	accessible := []string{clientA}
	rows, err = repo.FindWithFilters(ctx, dispatchjob.FilterParams{
		Codes: []string{code}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"djscopetest01", "djscopetest03"}, ids(rows))

	// The attack shape: explicit filter for the OTHER tenant yields nothing.
	rows, err = repo.FindWithFilters(ctx, dispatchjob.FilterParams{
		Codes: []string{code}, ClientIDs: []string{clientB}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.Empty(t, ids(rows), "cross-tenant filter must not leak another tenant's jobs")
}

// ── X-06: strict enum reads on msg_dispatch_jobs ──────────────────────────
//
// These pin migration 052's four converted parsers (common.ParseDispatchStatus,
// dispatchjob.ParseKind, dispatchjob.ParseRetryStrategy, dispatchjob.ParseErrorType):
// a row whose status/kind/retry_strategy/error_type column holds a value outside
// the known constants must fail the read with a distinct CORRUPT_* error, never
// round-trip that literal string and never silently coerce to a default (PENDING /
// EVENT / exponential / UNKNOWN respectively — the pre-X-06 defaults).

// TestFindByID_CorruptStatusFailsLoudly is the read-boundary pin for status.
func TestFindByID_CorruptStatusFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	id := tsid.GenerateUntyped()
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_jobs", "chk_msg_dispatch_jobs_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, status)
			 VALUES ($1, 'corrupt:status', 'http://example.invalid/hook', 'NOT_A_REAL_STATUS')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, id)
		})
	})

	j, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt status must fail the read, not round-trip it or default to PENDING")
	assert.Nil(t, j)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_JOB_STATUS")
}

// TestFindByID_CorruptKindFailsLoudly is the read-boundary pin for kind.
func TestFindByID_CorruptKindFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	id := tsid.GenerateUntyped()
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_jobs", "chk_msg_dispatch_jobs_kind", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, kind)
			 VALUES ($1, 'corrupt:kind', 'http://example.invalid/hook', 'NOT_A_REAL_KIND')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, id)
		})
	})

	j, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt kind must fail the read, not round-trip it or default to EVENT")
	assert.Nil(t, j)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_JOB_KIND")
}

// TestFindByID_CorruptRetryStrategyFailsLoudly is the read-boundary pin for
// retry_strategy (nullable — NULL must keep defaulting to exponential; only
// an explicit unrecognised value is corrupt).
func TestFindByID_CorruptRetryStrategyFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	id := tsid.GenerateUntyped()
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_jobs", "chk_msg_dispatch_jobs_retry_strategy", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, retry_strategy)
			 VALUES ($1, 'corrupt:retrystrategy', 'http://example.invalid/hook', 'NOT_A_REAL_STRATEGY')`, id)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, id)
		})
	})

	j, err := repo.FindByID(ctx, id)
	require.Error(t, err, "a corrupt retry_strategy must fail the read, not round-trip it or default to exponential")
	assert.Nil(t, j)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_JOB_RETRY_STRATEGY")
}

// TestFindByID_LegacyErrorStatusReadable pins the other half of the ruling:
// 'ERROR' is a known legacy alias (dispatchjob.GroupHoldingStatusSQL still
// matches it, and migration 052's CHECK constraint admits it), so a row
// holding it is NOT corrupt — it reads back as common.DispatchFailed. No
// constraint-drop needed: 'ERROR' is an allowed value, an ordinary write.
func TestFindByID_LegacyErrorStatusReadable(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	id := tsid.GenerateUntyped()
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_jobs (id, code, target_url, status)
		 VALUES ($1, 'legacy:error:status', 'http://example.invalid/hook', 'ERROR')`, id)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, id)
	})

	j, err := repo.FindByID(ctx, id)
	require.NoError(t, err, "the legacy ERROR status must read cleanly, not be treated as corrupt")
	require.NotNil(t, j)
	assert.Equal(t, common.DispatchFailed, j.Status)
}

// TestFindWithFilters_CorruptStatusFailsTheWholeList pins the ruling's list
// semantics explicitly on the msg_dispatch_jobs_read projection FindWithFilters
// reads: "a list containing the row fails too" — one bad row must not be
// silently skipped or coerced while the rest of the list is returned
// successfully.
func TestFindWithFilters_CorruptStatusFailsTheWholeList(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	const code = "corrupt:list:status"
	goodID := tsid.GenerateUntyped()
	badID := tsid.GenerateUntyped()

	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_jobs_read
		     (id, code, target_url, kind, protocol, mode, status, max_retries, updated_at)
		 VALUES ($1, $2, 'http://example.invalid/hook', 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'PENDING', 3, NOW())`,
		goodID, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs_read WHERE id = $1`, goodID)
	})

	testpg.WithConstraintDropped(t, pool, "msg_dispatch_jobs_read", "chk_msg_dispatch_jobs_read_status", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs_read
			     (id, code, target_url, kind, protocol, mode, status, max_retries, updated_at)
			 VALUES ($1, $2, 'http://example.invalid/hook', 'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'NOT_A_REAL_STATUS', 3, NOW())`,
			badID, code)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs_read WHERE id = $1`, badID)
		})
	})

	rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}})
	require.Error(t, err, "the list read must fail, not silently drop the bad row")
	assert.Nil(t, rows)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_JOB_STATUS")
}

// TestAttemptsByJob_CorruptErrorTypeFailsLoudly pins the read-boundary
// conversion for dispatchjob.ParseErrorType, applied inside
// Repository.AttemptsByJob: an unrecognised error_type on one attempt fails
// the whole attempts list for that job, never silently coerces to UNKNOWN.
func TestAttemptsByJob_CorruptErrorTypeFailsLoudly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	jobID := tsid.GenerateUntyped()
	_, err := pool.Exec(ctx,
		`INSERT INTO msg_dispatch_jobs (id, code, target_url) VALUES ($1, 'corrupt:attempt:errtype', 'http://example.invalid/hook')`, jobID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_jobs WHERE id = $1`, jobID)
	})

	attemptID := tsid.GenerateUntyped()
	testpg.WithConstraintDropped(t, pool, "msg_dispatch_job_attempts", "chk_msg_dispatch_job_attempts_error_type", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_job_attempts (id, dispatch_job_id, attempt_number, status, error_type)
			 VALUES ($1, $2, 1, 'FAILURE', 'NOT_A_REAL_ERRTYPE')`, attemptID, jobID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM msg_dispatch_job_attempts WHERE id = $1`, attemptID)
		})
	})

	attempts, err := repo.AttemptsByJob(ctx, jobID)
	require.Error(t, err, "a corrupt error_type must fail the whole attempts read, not silently coerce to UNKNOWN")
	assert.Nil(t, attempts)
	assert.Contains(t, err.Error(), "CORRUPT_DISPATCH_JOB_ATTEMPT_ERROR_TYPE")
}
