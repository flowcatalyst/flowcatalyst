//go:build integration

package loginattempt_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// seedFailure records one FAILURE attempt for identifier at exactly at (UTC),
// bypassing loginattempt.New's time.Now() stamp so tests can place rows
// precisely — including outside the quarters migration 049 pre-creates, to
// exercise the DEFAULT partition.
func seedFailure(t *testing.T, ctx context.Context, repo *loginattempt.Repository, identifier string, at time.Time) {
	t.Helper()
	a := loginattempt.New(loginattempt.AttemptUserLogin, loginattempt.OutcomeFailure)
	a.Identifier = &identifier
	a.AttemptedAt = at.UTC()
	require.NoError(t, repo.Record(ctx, a))
}

// TestIamLoginAttemptsIsPartitioned pins the migration-049 shape: the table
// is RANGE-partitioned, and partitions exist for the current quarter, the
// next quarter, and a DEFAULT catch-all.
func TestIamLoginAttemptsIsPartitioned(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)

	var relkind string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT relkind::text FROM pg_class WHERE relname = 'iam_login_attempts'`).Scan(&relkind))
	assert.Equal(t, "p", relkind, "iam_login_attempts must be RANGE-partitioned (migration 049)")

	var count int
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM pg_inherits i
		  JOIN pg_class parent ON i.inhparent = parent.oid
		  JOIN pg_class child  ON i.inhrelid  = child.oid
		 WHERE parent.relname = 'iam_login_attempts' AND child.relname = 'iam_login_attempts_default'`,
	).Scan(&count))
	assert.Equal(t, 1, count, "a DEFAULT partition must exist")

	now := time.Now().UTC()
	for _, q := range []time.Time{now, now.AddDate(0, 3, 0)} {
		year := q.Year()
		quarter := (int(q.Month())-1)/3 + 1
		name := "iam_login_attempts_" + itoa(year) + "_q" + itoa(quarter)
		var n int
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_inherits i
			  JOIN pg_class parent ON i.inhparent = parent.oid
			  JOIN pg_class child  ON i.inhrelid  = child.oid
			 WHERE parent.relname = 'iam_login_attempts' AND child.relname = $1`, name,
		).Scan(&n))
		assert.Equal(t, 1, n, "partition %s (current/next quarter) must exist", name)
	}

	var pkCols []string
	rows, err := pool.Query(ctx, `
		SELECT a.attname FROM pg_index idx
		  JOIN pg_attribute a ON a.attrelid = idx.indrelid AND a.attnum = ANY(idx.indkey)
		 WHERE idx.indrelid = 'iam_login_attempts'::regclass AND idx.indisprimary
		 ORDER BY array_position(idx.indkey, a.attnum)`)
	require.NoError(t, err)
	for rows.Next() {
		var c string
		require.NoError(t, rows.Scan(&c))
		pkCols = append(pkCols, c)
	}
	rows.Close()
	assert.Equal(t, []string{"id", "attempted_at"}, pkCols, "PK must be (id, attempted_at)")
}

// itoa avoids pulling in strconv just for this test's assertions.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestFindRecentByIdentifier_CorruptOutcomeFailsSafe is the X-06 read
// boundary, end to end against a real row: a row whose outcome column holds
// something other than SUCCESS/FAILURE (hand-edited, or written before
// validation existed) must come back as FAILURE, never SUCCESS — the old
// ParseOutcome default. Unlike the serviceaccount webhook-auth-type read
// boundary, this does NOT fail the whole list: it's a display/audit path,
// and the real backoff counters query the column directly in SQL and are
// unaffected by this Go-level parse.
func TestFindRecentByIdentifier_CorruptOutcomeFailsSafe(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "x06-corrupt-outcome@example.com"

	// Migration 051 forbids this value at the write boundary. The fail-safe
	// read path exists for a value that predates the constraint, so the row is
	// seeded with the constraint briefly off — precisely the case simulated.
	// The row is deleted before the constraint is restored (cleanups run LIFO),
	// or the restore would fail to validate against it.
	//
	// iam_login_attempts is RANGE-partitioned (migration 049) and the CHECK
	// lives on the parent, so dropping it there covers every partition.
	testpg.WithConstraintDropped(t, pool, "iam_login_attempts", "chk_iam_login_attempts_outcome", func() {
		_, err := pool.Exec(ctx, `
			INSERT INTO iam_login_attempts (id, attempt_type, outcome, identifier, attempted_at)
			VALUES ('sa_corruptoutcm1', 'USER_LOGIN', 'GARBAGE', $1, NOW())`, identifier)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(),
				`DELETE FROM iam_login_attempts WHERE id = 'sa_corruptoutcm1'`)
		})
	})

	rows, err := repo.FindRecentByIdentifier(ctx, identifier, 5)
	require.NoError(t, err, "a corrupt outcome must not fail the whole read")
	require.Len(t, rows, 1)
	assert.Equal(t, loginattempt.OutcomeFailure, rows[0].Outcome,
		"an unrecognised outcome must read as FAILURE, never SUCCESS")
}

// seedSuccess is seedFailure's SUCCESS counterpart, for LastSuccessAt tests.
func seedSuccess(t *testing.T, ctx context.Context, repo *loginattempt.Repository, identifier string, at time.Time) {
	t.Helper()
	a := loginattempt.New(loginattempt.AttemptUserLogin, loginattempt.OutcomeSuccess)
	a.Identifier = &identifier
	a.AttemptedAt = at.UTC()
	require.NoError(t, repo.Record(ctx, a))
}

// TestLastSuccessAt_FindsARecentSuccess pins the happy path: a success well
// inside the 400-day lookback bound is found.
func TestLastSuccessAt_FindsARecentSuccess(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "x06-lastsuccess-recent@example.com"

	at := time.Now().UTC().Add(-time.Hour)
	seedSuccess(t, ctx, repo, identifier, at)

	got, err := repo.LastSuccessAt(ctx, identifier)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.WithinDuration(t, at, *got, time.Millisecond)
}

// TestLastSuccessAt_PicksTheMostRecentOfSeveral pins that the rewrite from
// MAX(attempted_at) to ORDER BY ... LIMIT 1 still returns the LATEST
// success, not e.g. the first one the index happens to visit.
func TestLastSuccessAt_PicksTheMostRecentOfSeveral(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "x06-lastsuccess-several@example.com"

	older := time.Now().UTC().Add(-48 * time.Hour)
	newer := time.Now().UTC().Add(-1 * time.Hour)
	seedSuccess(t, ctx, repo, identifier, older)
	seedSuccess(t, ctx, repo, identifier, newer)
	// A FAILURE in between must not be picked up as a success.
	seedFailure(t, ctx, repo, identifier, time.Now().UTC().Add(-24*time.Hour))

	got, err := repo.LastSuccessAt(ctx, identifier)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.WithinDuration(t, newer, *got, time.Millisecond, "must return the most recent success, not the oldest")
}

// TestLastSuccessAt_OutsideLookbackReadsAsNone pins the X-06 performance fix's
// documented tradeoff: a real SUCCESS older than the 400-day lookback bound
// is indistinguishable from "never succeeded" — the query returns nil, not
// the stale timestamp. This is intentional (see lastSuccessLookback's
// doc comment for why loginbackoff.Check's own 30-day fallback makes this
// safe), and this test is what would catch an accidental change to the
// bound's width.
func TestLastSuccessAt_OutsideLookbackReadsAsNone(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "x06-lastsuccess-stale@example.com"

	stale := time.Now().UTC().AddDate(-2, 0, 0) // 2 years ago — well outside 400 days
	seedSuccess(t, ctx, repo, identifier, stale)

	got, err := repo.LastSuccessAt(ctx, identifier)
	require.NoError(t, err)
	assert.Nil(t, got, "a success older than the lookback bound must read as none, not the stale timestamp")
}

// TestLastSuccessAt_NeverSucceededReturnsNil pins the pre-existing "no rows
// at all" case, now via ErrNoRows (the query changed from MAX() to
// ORDER BY ... LIMIT 1) rather than a NULL aggregate result.
func TestLastSuccessAt_NeverSucceededReturnsNil(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "x06-lastsuccess-never@example.com"

	seedFailure(t, ctx, repo, identifier, time.Now().UTC())

	got, err := repo.LastSuccessAt(ctx, identifier)
	require.NoError(t, err)
	assert.Nil(t, got)
}

// TestGlobalCeilingTrippedAt_FindsTheCeilingThMostRecentFailure pins the
// A-19 query: with N failures on record, offset ceiling-1 (0-indexed from
// the newest) must land on the failure that pushed the in-window count up
// to N, not the oldest or the newest indiscriminately.
func TestGlobalCeilingTrippedAt_FindsTheCeilingThMostRecentFailure(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "a19-tripped-at@example.com"

	base := time.Now().UTC().Add(-1 * time.Hour)
	// 5 failures, 10s apart: base, base+10s, base+20s, base+30s, base+40s
	// (oldest to newest).
	var stamps []time.Time
	for i := 0; i < 5; i++ {
		ts := base.Add(time.Duration(i) * 10 * time.Second)
		stamps = append(stamps, ts)
		seedFailure(t, ctx, repo, identifier, ts)
	}

	since := base.Add(-time.Minute)

	// ceiling=5: the 5th most recent of exactly 5 failures is the oldest.
	got, err := repo.GlobalCeilingTrippedAt(ctx, identifier, since, 5)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.WithinDuration(t, stamps[0], *got, time.Millisecond, "ceiling=5 of 5 must land on the oldest")

	// ceiling=2: the 2nd most recent is stamps[3] (base+30s).
	got, err = repo.GlobalCeilingTrippedAt(ctx, identifier, since, 2)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.WithinDuration(t, stamps[3], *got, time.Millisecond, "ceiling=2 must land on the 2nd most recent")

	// ceiling=6: only 5 failures exist → never tripped → nil.
	got, err = repo.GlobalCeilingTrippedAt(ctx, identifier, since, 6)
	require.NoError(t, err)
	assert.Nil(t, got, "fewer than ceiling failures must report untripped")

	// A since bound that excludes the failures entirely (X-03: the query
	// must carry an occurred_at/attempted_at lower bound, and honour it).
	got, err = repo.GlobalCeilingTrippedAt(ctx, identifier, stamps[4].Add(time.Second), 1)
	require.NoError(t, err)
	assert.Nil(t, got, "since must exclude failures before it")
}

// TestGlobalCeilingTrippedAt_ReachesIntoTheDefaultPartition: a failure
// backdated well outside every quarter migration 049 pre-created still
// lands in the DEFAULT partition and must still be found — partition
// pruning on attempted_at must not silently drop it.
func TestGlobalCeilingTrippedAt_ReachesIntoTheDefaultPartition(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)
	const identifier = "a19-default-partition@example.com"

	old := time.Date(2015, 6, 1, 0, 0, 0, 0, time.UTC)
	seedFailure(t, ctx, repo, identifier, old)

	got, err := repo.GlobalCeilingTrippedAt(ctx, identifier, old.Add(-time.Hour), 1)
	require.NoError(t, err)
	require.NotNil(t, got, "a row in the DEFAULT partition must still be found")
	assert.WithinDuration(t, old, *got, time.Millisecond)
}

// TestEnsureQuarterlyPartition_CreatesAndIsIdempotent pins the housekeeping
// forward maintenance: a partition for a quarter migration 049 didn't
// pre-create can be added on demand, and a second call is a no-op, not an
// error.
func TestEnsureQuarterlyPartition_CreatesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)

	// 3 quarters out: migration 049 only pre-creates current + next.
	future := time.Now().UTC().AddDate(0, 9, 0)
	year := future.Year()
	quarter := (int(future.Month())-1)/3 + 1
	name := "iam_login_attempts_" + itoa(year) + "_q" + itoa(quarter)

	exists := func() int {
		var n int
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM pg_inherits i
			  JOIN pg_class parent ON i.inhparent = parent.oid
			  JOIN pg_class child  ON i.inhrelid  = child.oid
			 WHERE parent.relname = 'iam_login_attempts' AND child.relname = $1`, name,
		).Scan(&n))
		return n
	}
	require.Equal(t, 0, exists(), "precondition: partition not pre-created")

	require.NoError(t, repo.EnsureQuarterlyPartition(ctx, future))
	assert.Equal(t, 1, exists(), "partition must now exist")

	require.NoError(t, repo.EnsureQuarterlyPartition(ctx, future), "second call must be idempotent")
	assert.Equal(t, 1, exists(), "still exactly one partition, not duplicated")

	// A row for that quarter routes into it, not the DEFAULT partition.
	a := loginattempt.New(loginattempt.AttemptUserLogin, loginattempt.OutcomeFailure)
	id := "a19-ensure-quarter@example.com"
	a.Identifier = &id
	a.AttemptedAt = future
	require.NoError(t, repo.Record(ctx, a))

	var tableoid string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT tableoid::regclass::text FROM iam_login_attempts WHERE id = $1`, a.ID,
	).Scan(&tableoid))
	assert.Equal(t, name, tableoid, "the new row must route into the newly ensured partition")
}

// TestDropPartitionsOlderThan_DropsOnlyPartitionsWhollyBeforeCutoff pins the
// retention sweep: only quarterly partitions entirely before the cutoff are
// dropped; the DEFAULT partition and anything still inside the retention
// window survive.
func TestDropPartitionsOlderThan_DropsOnlyPartitionsWhollyBeforeCutoff(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := loginattempt.NewRepository(pool)

	// A partition from 2010 — far older than any real retention cutoff, and
	// distinct from anything migration 049 or another test could create.
	const oldName = "iam_login_attempts_2010_q1"
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS `+oldName+`
		    PARTITION OF iam_login_attempts FOR VALUES FROM ('2010-01-01') TO ('2010-04-01')`)
	require.NoError(t, err)

	cutoff := time.Now().UTC().AddDate(-3, 0, 0) // the ruled 3-year retention

	dropped, err := repo.DropPartitionsOlderThan(ctx, cutoff)
	require.NoError(t, err)
	assert.Contains(t, dropped, oldName, "the 2010 partition is wholly before a 3-year cutoff")

	var n int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pg_class WHERE relname = $1`, oldName).Scan(&n))
	assert.Zero(t, n, "the dropped partition must no longer exist")

	// The DEFAULT partition is never touched, however old the cutoff.
	var defaultCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pg_class WHERE relname = 'iam_login_attempts_default'`).Scan(&defaultCount))
	assert.Equal(t, 1, defaultCount, "the DEFAULT partition must survive")

	// The current quarter's partition (well inside retention) survives too.
	now := time.Now().UTC()
	currentName := "iam_login_attempts_" + itoa(now.Year()) + "_q" + itoa((int(now.Month())-1)/3+1)
	var currentCount int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM pg_class WHERE relname = $1`, currentName).Scan(&currentCount))
	assert.Equal(t, 1, currentCount, "the current quarter's partition must survive")
}
