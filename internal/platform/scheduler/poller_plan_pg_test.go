//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Plan tests for every statement the dispatch path runs against msg_dispatch_jobs —
// the claim, mark-QUEUED, the hold-back (both halves), the stale sweeps and the backlog
// sample — on a real database, with the scheduler's own connection settings
// (PoolRuntimeParams), at ~5,000 and ~200,000 PENDING jobs inside a job table of
// 300,000 mostly-COMPLETED rows over three monthly partitions, in the statistics
// states a production table lands in:
//
//	1 freshly analysed
//	2 never analysed
//	3 the active partition analysed when it held ZERO pending jobs, then a burst (the
//	  realistic case: the statistics say PENDING is absent)
//	4 the empty forward/back partitions VACUUMed and ANALYZEd, then a burst (the state
//	  that made the old claim sort every pending row)
//	5 every pending row sharing one created_at and one sequence (ties)
//	6 the cached-plan case: the statements run several times on ONE connection while
//	  there are no pending jobs, then the burst arrives and they run again on it
//	7 an in-flight array of 0, 1,000 and 5,000 ids that are the FIRST rows of the walk
//
// They assert plan SHAPE (no Sort / Seq Scan where there must not be; the primary key
// for mark-QUEUED; the plain index for the claim, hold-back, sweeps and backlog) and
// log the timings (go test -v). A negative control repeats the claim on a pool WITHOUT
// the two settings. Each state is a clone (CREATE DATABASE ... TEMPLATE) of one of two
// pre-built 300,000-row templates, so the data set is loaded twice, not per state.

const planCompleted = 300_000

type planStateName string

const (
	stFresh      planStateName = "1 freshly analysed"
	stNever      planStateName = "2 never analysed"
	stZeroThen   planStateName = "3 analysed at zero pending, then burst"
	stVacEmpty   planStateName = "4 empty partitions vacuumed, then burst"
	stTies       planStateName = "5 one created_at and sequence (ties)"
	stCachedPlan planStateName = "6 cached plan, burst after"
)

func monthStart(t time.Time, back int) time.Time {
	return time.Date(t.Year(), t.Month()-time.Month(back), 1, 0, 0, 0, 0, time.UTC)
}

// buildPlanTemplate makes a database with planCompleted mostly-COMPLETED jobs and no
// PENDING ones, in the partitions of the current month and the two before it, and
// returns its name (its pool closed, so it can be cloned). analysed: VACUUM ANALYZE
// everything while PENDING is absent; otherwise no statistics at all.
func buildPlanTemplate(t *testing.T, name string, analysed bool) string {
	t.Helper()
	ctx := context.Background()
	p := testpg.ScratchDB(t, name)
	now := time.Now().UTC()
	for back := 0; back <= 2; back++ {
		s, e := monthStart(now, back), monthStart(now, back-1)
		_, err := p.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_%s PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('%s') TO ('%s')`,
			s.Format("2006_01"), s.Format("2006-01-02"), e.Format("2006-01-02")))
		require.NoError(t, err)
	}
	rows, err := p.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'msg_dispatch_jobs'::regclass`)
	require.NoError(t, err)
	var parts []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		parts = append(parts, n)
	}
	rows.Close()
	for _, tb := range parts {
		_, err := p.Exec(ctx, "ALTER TABLE "+tb+" SET (autovacuum_enabled = false)")
		require.NoError(t, err)
	}
	m0, m1, m2 := monthStart(now, 0), monthStart(now, 1), monthStart(now, 2)
	_, err = p.Exec(ctx, `
	INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, sequence, created_at, updated_at, mode)
	SELECT 'j' || lpad(n::text, 12, '0'), 'c', 'http://x',
	  CASE WHEN n % 1000 < 5 THEN 'FAILED' WHEN n % 1000 < 15 THEN 'QUEUED' WHEN n % 1000 < 20 THEN 'PROCESSING' ELSE 'COMPLETED' END,
	  'g' || lpad((n % 5000)::text, 4, '0'), 0,
	  CASE n % 3 WHEN 0 THEN $1::timestamptz WHEN 1 THEN $2::timestamptz ELSE $3::timestamptz END + ((n / 3) * interval '5 seconds'),
	  CASE WHEN n % 1000 < 15 THEN NOW() - interval '20 minutes' WHEN n % 1000 < 20 THEN NOW() - interval '100 minutes'
	       ELSE CASE n % 3 WHEN 0 THEN $1::timestamptz WHEN 1 THEN $2::timestamptz ELSE $3::timestamptz END + ((n / 3) * interval '5 seconds') END,
	  'BLOCK_ON_ERROR'
	FROM generate_series(1, $4::int) n`, m2, m1, m0, planCompleted)
	require.NoError(t, err)
	if analysed {
		_, err = p.Exec(ctx, `VACUUM (ANALYZE) msg_dispatch_jobs`)
		require.NoError(t, err)
	}
	p.Close()
	return name
}

// burst inserts n PENDING jobs into the current month's partition: 500 groups, spread
// positions (or all sharing one created_at and one sequence when ties), 0.1% backed off.
func burst(t *testing.T, p *pgxpool.Pool, n int, ties bool) {
	t.Helper()
	m0 := monthStart(time.Now().UTC(), 0)
	seq, ca := "(n / 500)::int", "$1::timestamptz + (n * interval '1 second')"
	if ties {
		seq, ca = "0", "$1::timestamptz + interval '1 hour'"
	}
	_, err := p.Exec(context.Background(), fmt.Sprintf(`
	INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, sequence, created_at, updated_at, mode, scheduled_for)
	SELECT 'p' || lpad(n::text, 12, '0'), 'c', 'http://x', 'PENDING', 'g' || lpad((n %% 500)::text, 4, '0'), %s, %s, %s, 'BLOCK_ON_ERROR',
	       CASE WHEN n %% 1000 = 7 THEN NOW() + interval '1 hour' END
	  FROM generate_series(1, $2::int) n`, seq, ca, ca), m0, n)
	require.NoError(t, err)
}

// firstClaimed runs the claim for the first limit rows of the walk, as the poller would.
func firstClaimed(t *testing.T, p *pgxpool.Pool, limit int, inFlight []string) []dispatchjob.ClaimedJob {
	t.Helper()
	rows, err := dispatchjob.NewLifecycle(p).ClaimPending(context.Background(), limit, nil, nil, inFlight)
	require.NoError(t, err)
	return rows
}

type planSet struct {
	claim, mark, holders, heldBefore, staleQ, staleP, backlog, oldest planNode
}

func (s planSet) timing(label string) string {
	return fmt.Sprintf("TIMING %-44s claim %7.2f  mark %7.2f  holders %7.2f  held-before %5.2f  stale-Q %7.2f  stale-P %7.2f  backlog %7.2f  oldest %6.2f ms",
		label, s.claim.ExecTime, s.mark.ExecTime, s.holders.ExecTime, s.heldBefore.ExecTime, s.staleQ.ExecTime, s.staleP.ExecTime, s.backlog.ExecTime, s.oldest.ExecTime)
}

// plansFor EXPLAINs every statement on p as the poller would run it: a claim of 500 with
// the given in-flight ids, mark-QUEUED for that claim's rows, the hold-back for its
// groups, the sweeps and the backlog sample.
func plansFor(t *testing.T, p *pgxpool.Pool, inFlight []string) planSet {
	t.Helper()
	var s planSet
	held := make([]string, 20)
	for i := range held {
		held[i] = fmt.Sprintf("g%04d", 400+i)
	}
	cl := dispatchjob.ClaimPendingStatement(500, []string{}, held, inFlight)
	s.claim = planOf(t, p, cl.SQL, literals(cl.Args))

	claimed := firstClaimed(t, p, 500, inFlight)
	require.NotEmpty(t, claimed, "the claim returned nothing: the state is not what the test thinks")
	ids := make([]string, len(claimed))
	created := make([]time.Time, len(claimed))
	versions := make([]time.Time, len(claimed))
	last := map[string]dispatchjob.ClaimedJob{}
	for i, c := range claimed {
		ids[i], created[i], versions[i] = c.ID, c.CreatedAt, c.UpdatedAt
		if c.MessageGroup != nil {
			last[*c.MessageGroup] = c
		}
	}
	mk := dispatchjob.MarkQueuedStatement(ids, created, versions)
	s.mark = planOf(t, p, mk.SQL, literals(mk.Args))

	var groups []string
	var seqs []int32
	var cas []time.Time
	var jids []string
	for g, c := range last {
		groups, seqs, cas, jids = append(groups, g), append(seqs, c.Sequence), append(cas, c.CreatedAt), append(jids, c.ID)
	}
	s.holders = planOf(t, p, dispatchjob.GroupHoldersSQL, literals([]any{dispatchjob.GroupHoldingStatuses, groups, seqs, cas, jids}))
	s.heldBefore = planOf(t, p, dispatchjob.GroupHeldBeforeSQL, literals([]any{dispatchjob.GroupHoldingStatuses, groups[0], seqs[0], cas[0], jids[0]}))
	st := dispatchjob.StaleRecoveryStatements(time.Now().Add(-15 * time.Minute))
	s.staleQ = planOf(t, p, st[0].SQL, literals(st[0].Args))
	st = dispatchjob.StaleRecoveryStatements(time.Now().Add(-75 * time.Minute))
	s.staleP = planOf(t, p, st[1].SQL, literals(st[1].Args))
	s.backlog = planOf(t, p, dispatchjob.PendingBacklogSQL, "")
	s.oldest = planOf(t, p, dispatchjob.OldestWaitingSQL, "")
	return s
}

// The backlog count is capped at 100,001 matches, and when PENDING is a large share of
// the table (200,000 of 500,000) the planner may read the partition sequentially to find
// them; that is cheap and bounded, so only its time is asserted.
func assertBacklog(t *testing.T, s planSet) {
	t.Helper()
	assert.Less(t, s.backlog.ExecTime, 150.0, "the capped backlog count stays cheap")
}

func usesStatusIndex(n planNode) bool {
	return strings.Contains(n.Index, "status_message_group_sequence")
}

func isPK(n planNode) bool {
	return strings.HasPrefix(n.Index, "msg_dispatch_jobs") && strings.HasSuffix(n.Index, "_pkey")
}

func isSort(n planNode) bool { return strings.Contains(n.NodeType, "Sort") }

// assertShapes: the claim has no Sort and no Seq Scan and walks the plain index;
// mark-QUEUED reads by primary key and never seq-scans or walks the status index;
// hold-back, sweeps, backlog use the plain index and scan no partition.
func assertShapes(t *testing.T, s planSet) {
	t.Helper()
	assert.False(t, hasNode(s.claim, isSort), "claim must not sort")
	assert.Empty(t, seqScansWithData(s.claim, "msg_dispatch_jobs"), "claim must not scan a partition")
	assert.True(t, hasNode(s.claim, usesStatusIndex), "claim walks idx_dispatch_jobs_status_group")

	assert.Empty(t, seqScansWithData(s.mark, "msg_dispatch_jobs"), "mark-QUEUED must not scan a partition")
	assert.True(t, hasNode(s.mark, isPK), "mark-QUEUED reads by primary key")
	assert.False(t, hasNode(s.mark, usesStatusIndex), "mark-QUEUED must not walk the status index")

	assertBacklog(t, s)
	for name, pl := range map[string]planNode{"holders": s.holders, "held-before": s.heldBefore, "stale QUEUED": s.staleQ,
		"stale PROCESSING": s.staleP, "oldest": s.oldest} {
		assert.Empty(t, seqScansWithData(pl, "msg_dispatch_jobs"), "%s must not scan a partition", name)
		assert.True(t, hasNode(pl, usesStatusIndex), "%s must use the plain index", name)
	}
}

// planEnv holds the two pre-built templates: planCompleted jobs analysed while no job was
// PENDING, and the same with no statistics at all.
type planEnv struct{ analysed, never string }

func (e planEnv) tpl(analysed bool) string {
	if analysed {
		return e.analysed
	}
	return e.never
}

// TestPlans runs every plan test against clones of two templates built once (a clone
// source must have no sessions, and a template built inside a subtest would be dropped
// with it).
func TestPlans(t *testing.T) {
	env := planEnv{
		analysed: buildPlanTemplate(t, "plantpl_analysed", true),
		never:    buildPlanTemplate(t, "plantpl_never", false),
	}
	t.Run("every statement in every statistics state", func(t *testing.T) { plansEveryState(t, env) })
	t.Run("large in-flight array", func(t *testing.T) { plansInFlight(t, env) })
	t.Run("cached plan after a burst", func(t *testing.T) { plansCached(t, env) })
	t.Run("negative control", func(t *testing.T) { plansNegative(t, env) })
}

func prepareState(t *testing.T, env planEnv, st planStateName, name string, params map[string]string, pending int) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	switch st {
	case stFresh:
		p := testpg.CloneDB(t, env.tpl(false), name, params)
		burst(t, p, pending, false)
		_, err := p.Exec(ctx, `ANALYZE msg_dispatch_jobs`)
		require.NoError(t, err)
		return p
	case stNever:
		p := testpg.CloneDB(t, env.tpl(false), name, params)
		burst(t, p, pending, false)
		return p
	case stZeroThen:
		p := testpg.CloneDB(t, env.tpl(true), name, params)
		burst(t, p, pending, false)
		return p
	case stVacEmpty:
		p := testpg.CloneDB(t, env.tpl(true), name, params)
		rows, err := p.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid
		  WHERE i.inhparent = 'msg_dispatch_jobs'::regclass AND NOT EXISTS (SELECT 1 FROM pg_class x WHERE x.oid = c.oid AND x.reltuples > 0)`)
		require.NoError(t, err)
		empties, err := pgx.CollectRows(rows, pgx.RowTo[string])
		require.NoError(t, err)
		for _, e := range empties {
			_, err := p.Exec(ctx, "VACUUM (ANALYZE) "+e)
			require.NoError(t, err)
		}
		burst(t, p, pending, false)
		return p
	case stTies:
		p := testpg.CloneDB(t, env.tpl(true), name, params)
		burst(t, p, pending, true)
		return p
	}
	t.Fatalf("unknown state %s", st)
	return nil
}

func plansEveryState(t *testing.T, env planEnv) {
	for _, pending := range []int{5_000, 200_000} {
		for si, st := range []planStateName{stFresh, stNever, stZeroThen, stVacEmpty, stTies} {
			t.Run(fmt.Sprintf("%d pending/%s", pending, st), func(t *testing.T) {
				p := prepareState(t, env, st, fmt.Sprintf("plan_%d_%d", pending, si), PoolRuntimeParams, pending)
				s := plansFor(t, p, nil)
				t.Log(s.timing(fmt.Sprintf("%dk pending, %s", pending/1000, st)))
				assertShapes(t, s)
			})
		}
	}
}

// 7: an in-flight array of 0, 1,000 and 5,000 ids that are the first rows of the walk
// (the claim must skip them with the filter, not by a different plan).
func plansInFlight(t *testing.T, env planEnv) {
	for _, pending := range []int{5_000, 200_000} {
		p := prepareState(t, env, stZeroThen, fmt.Sprintf("plan_inflight_%d", pending), PoolRuntimeParams, pending)
		for _, k := range []int{0, 1_000, 4_000} {
			first := firstClaimed(t, p, max(k, 1), nil)
			ids := make([]string, 0, k)
			for _, c := range first[:min(k, len(first))] {
				ids = append(ids, c.ID)
			}
			t.Run(fmt.Sprintf("%d pending/%d in flight", pending, k), func(t *testing.T) {
				s := plansFor(t, p, ids)
				t.Log(s.timing(fmt.Sprintf("%dk pending, %d in flight", pending/1000, k)))
				assertShapes(t, s)
			})
		}
	}
}

// 6: the cached-plan case. ONE connection; the statements run several times while no job
// is PENDING (Postgres may settle on a generic plan made for an empty set), then the
// burst arrives and they run again on that same connection. With the scheduler's
// settings the plans must stay sane; without them (the negative control) they are
// reported.
func plansCached(t *testing.T, env planEnv) {
	for _, pending := range []int{5_000, 200_000} {
		run := func(t *testing.T, params map[string]string, name string) (claim, mark planNode) {
			ctx := context.Background()
			p := testpg.CloneDB(t, env.tpl(true), name, params)
			conn, err := p.Acquire(ctx)
			require.NoError(t, err)
			defer conn.Release()
			held := "'{}'::text[]"
			_, err = conn.Exec(ctx, `PREPARE c1 AS `+strings.ReplaceAll(dispatchjob.ClaimPendingSQL, "\n", " "))
			require.NoError(t, err)
			for range 8 { // no PENDING jobs: the claim returns nothing, Postgres may now cache a generic plan
				_, err = conn.Exec(ctx, fmt.Sprintf(`EXECUTE c1(500, '{}'::text[], %s, '{}'::text[])`, held), pgx.QueryExecModeSimpleProtocol)
				require.NoError(t, err)
			}
			_, err = conn.Exec(ctx, fmt.Sprintf(`
			INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, sequence, created_at, updated_at, mode)
			SELECT 'p' || lpad(n::text, 12, '0'), 'c', 'http://x', 'PENDING', 'g' || lpad((n %% 500)::text, 4, '0'), (n / 500)::int,
			       $1::timestamptz + (n * interval '1 second'), $1::timestamptz + (n * interval '1 second'), 'BLOCK_ON_ERROR'
			  FROM generate_series(1, %d) n`, pending), monthStart(time.Now().UTC(), 0))
			require.NoError(t, err)

			explain := func(stmt string) planNode {
				_, err := conn.Exec(ctx, "BEGIN")
				require.NoError(t, err)
				defer func() { _, _ = conn.Exec(ctx, "ROLLBACK") }()
				rs, err := conn.Query(ctx, "EXPLAIN (ANALYZE, FORMAT JSON) "+stmt, pgx.QueryExecModeSimpleProtocol)
				require.NoError(t, err)
				var raw []byte
				for rs.Next() {
					require.NoError(t, rs.Scan(&raw))
				}
				require.NoError(t, rs.Err())
				rs.Close()
				return decodePlan(t, raw)
			}
			claim = explain(fmt.Sprintf(`EXECUTE c1(500, '{}'::text[], %s, '{}'::text[])`, held))
			// mark-QUEUED, prepared and exercised the same way
			rows, err := conn.Query(ctx, `SELECT id, created_at, updated_at FROM msg_dispatch_jobs WHERE status = 'PENDING' ORDER BY message_group, sequence, created_at, id LIMIT 500`)
			require.NoError(t, err)
			var ids []string
			var cs, vs []time.Time
			for rows.Next() {
				var id string
				var c, v time.Time
				require.NoError(t, rows.Scan(&id, &c, &v))
				ids, cs, vs = append(ids, id), append(cs, c), append(vs, v)
			}
			rows.Close()
			mk := dispatchjob.MarkQueuedStatement(ids, cs, vs)
			_, err = conn.Exec(ctx, `PREPARE m1(text[], timestamptz[], timestamptz[]) AS `+strings.ReplaceAll(mk.SQL, "\n", " "))
			require.NoError(t, err)
			emptyArgs := `'{}'::text[], '{}'::timestamptz[], '{}'::timestamptz[]`
			for range 8 {
				_, err = conn.Exec(ctx, `EXECUTE m1(`+emptyArgs+`)`, pgx.QueryExecModeSimpleProtocol)
				require.NoError(t, err)
			}
			mark = explain(`EXECUTE m1(` + literals(mk.Args) + `)`)
			return claim, mark
		}
		bad := func(claim, mark planNode) bool {
			return hasNode(claim, isSort) || len(seqScansWithData(claim, "msg_dispatch_jobs")) > 0 ||
				len(seqScansWithData(mark, "msg_dispatch_jobs")) > 0 || hasNode(mark, usesStatusIndex)
		}
		t.Run(fmt.Sprintf("%d pending/with the scheduler's settings", pending), func(t *testing.T) {
			claim, mark := run(t, PoolRuntimeParams, fmt.Sprintf("cached_on_%d", pending))
			t.Logf("TIMING %dk pending, cached plan, settings ON : claim %7.2f ms  mark %7.2f ms", pending/1000, claim.ExecTime, mark.ExecTime)
			assert.False(t, hasNode(claim, isSort), "claim must not sort after the burst")
			assert.Empty(t, seqScansWithData(claim, "msg_dispatch_jobs"), "claim must not scan a partition after the burst")
			assert.Empty(t, seqScansWithData(mark, "msg_dispatch_jobs"), "mark-QUEUED must not scan a partition after the burst")
			assert.True(t, hasNode(mark, isPK), "mark-QUEUED reads by primary key after the burst")
		})
		t.Run(fmt.Sprintf("%d pending/negative control without the settings", pending), func(t *testing.T) {
			claim, mark := run(t, nil, fmt.Sprintf("cached_off_%d", pending))
			t.Logf("TIMING %dk pending, cached plan, settings OFF: claim %7.2f ms  mark %7.2f ms  (bad plan reproduced: %v)", pending/1000, claim.ExecTime, mark.ExecTime, bad(claim, mark))
		})
	}
}

// Negative control for the connection settings: the same states WITHOUT them. Logs
// which states give a bad claim or mark-QUEUED plan; asserts nothing (if none do on
// this Postgres, the log says so).
func plansNegative(t *testing.T, env planEnv) {
	var badStates []string
	for si, st := range []planStateName{stFresh, stNever, stZeroThen, stVacEmpty, stTies} {
		p := prepareState(t, env, st, fmt.Sprintf("plan_off_%d", si), nil, 200_000)
		s := plansFor(t, p, nil)
		isBad := hasNode(s.claim, isSort) || len(seqScansWithData(s.claim, "msg_dispatch_jobs")) > 0 ||
			len(seqScansWithData(s.mark, "msg_dispatch_jobs")) > 0 || hasNode(s.mark, usesStatusIndex)
		t.Log(s.timing(fmt.Sprintf("200k pending, %s, NO settings (bad: %v)", st, isBad)))
		if isBad {
			badStates = append(badStates, string(st))
		}
	}
	if len(badStates) == 0 {
		t.Log("NEGATIVE CONTROL: no state gave a bad plan without the settings on this Postgres")
	} else {
		t.Logf("NEGATIVE CONTROL: bad without the settings in: %v", badStates)
	}
}
