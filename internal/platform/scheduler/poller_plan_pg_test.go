//go:build integration

package scheduler

import (
	"context"
	"encoding/json"
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

// Plan tests for the statements the dispatch path runs against the job and queue
// tables, on a real database with a few hundred thousand rows, in the three
// statistics states a production table can be in:
//
//	freshly analysed        ANALYZE after the data is loaded
//	analysed while empty    ANALYZE ran on the empty tables, then the data arrived
//	never analysed          no statistics at all (autovacuum off)
//
// The connections are the scheduler's own: they carry PoolRuntimeParams
// (plan_cache_mode = force_custom_plan, enable_sort = off), so every statement is
// planned with its actual arguments, as in production. EXPLAIN ANALYZE output is
// read as JSON and asserted on structurally. The claim's own statements, in more
// states, are in poller_claim_plan_pg_test.go.

// planShape is a seeded data set. The claim needs a deep queue (a real backlog:
// on a few thousand rows a sequential scan and a sort is the right plan and the
// planner chooses it); the sweeps need what production looks like — mostly
// COMPLETED, a thin slice in every live status — so a sequential scan is NOT the
// right answer for them.
type planShape struct {
	name    string
	pendMod int // PENDING when n % 1000 < pendMod
	queued  int // jobs n < queued that are not PENDING/FAILED/ERROR are QUEUED
}

var (
	backlogShape   = planShape{"backlog", 250, 4000}  // ~25% PENDING: a queue of ~100k rows
	realisticShape = planShape{"realistic", 10, 4000} // ~1% PENDING
)

const planJobs = 400_000

// seedPlanData loads planJobs jobs over the three monthly partitions
// 2026-08..2026-10 and the queue rows of the PENDING ones, with autovacuum off
// so the statistics are exactly what the test makes them.
func seedPlanData(t *testing.T, p *pgxpool.Pool, shape planShape) {
	t.Helper()
	ctx := context.Background()
	for _, m := range [][3]string{{"2026_08", "2026-08-01", "2026-09-01"}, {"2026_09", "2026-09-01", "2026-10-01"}, {"2026_10", "2026-10-01", "2026-11-01"}} {
		_, err := p.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_%s PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('%s') TO ('%s')`, m[0], m[1], m[2]))
		require.NoError(t, err)
	}
	rows, err := p.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'msg_dispatch_jobs'::regclass`)
	require.NoError(t, err)
	tables := []string{"msg_dispatch_queue"}
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		tables = append(tables, n)
	}
	rows.Close()
	for _, tb := range tables {
		_, err := p.Exec(ctx, "ALTER TABLE "+tb+" SET (autovacuum_enabled = false)")
		require.NoError(t, err)
	}
	_, err = p.Exec(ctx, fmt.Sprintf(`
	INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, sequence, created_at, updated_at, mode, scheduled_for)
	SELECT id, 'c', 'http://x', status, grp, seq,
	  CASE WHEN status = 'QUEUED' THEN TIMESTAMPTZ '2026-10-01' + (n * interval '1 second') ELSE ca END,
	  CASE WHEN status = 'QUEUED' THEN NOW() - interval '5 minutes' - CASE WHEN n %% 50 = 0 THEN interval '20 minutes' ELSE interval '0' END
	       ELSE ca + interval '1 second' END,
	  'BLOCK_ON_ERROR', sf
	FROM (SELECT n, 'j' || lpad(n::text, 12, '0') AS id,
	  CASE WHEN n %% 1000 < %d THEN 'PENDING'
	       WHEN n %% 1000 < 15 THEN 'FAILED'
	       WHEN n %% 1000 < 17 THEN 'ERROR'
	       WHEN n < %d THEN 'QUEUED'
	       WHEN n %% 1000 < 20 THEN 'PROCESSING'
	       ELSE 'COMPLETED' END AS status,
	  CASE WHEN n %% 50 = 0 THEN NULL ELSE 'g' || (n %% 5000)::text END AS grp,
	  ((n / 5000)::int %% 3) AS seq,
	  TIMESTAMPTZ '2026-08-05' + (n * interval '15 seconds') AS ca,
	  CASE WHEN n %% 1000 = 1 THEN NOW() + interval '1 hour' END AS sf
	 FROM generate_series(1, %d) n) x`, shape.pendMod, shape.queued, planJobs))
	require.NoError(t, err)
	// The queue rows of the PENDING jobs, as the lifecycle would have written them.
	_, err = p.Exec(ctx, `INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
	        subscription_id, dispatch_pool_id, client_id, mode, queue, version)
	SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id, dispatch_pool_id, client_id, mode, queue, updated_at
	  FROM msg_dispatch_jobs WHERE status = 'PENDING'`)
	require.NoError(t, err)
}

type planState struct {
	name string
	// load is how the state is produced: it seeds the data and sets the statistics.
	load func(t *testing.T, p *pgxpool.Pool, shape planShape)
}

var planStates = []planState{
	{"never analysed", func(t *testing.T, p *pgxpool.Pool, s planShape) { seedPlanData(t, p, s) }},
	{"analysed while empty", func(t *testing.T, p *pgxpool.Pool, s planShape) {
		ctx := context.Background()
		// The partitions must exist for the empty ANALYZE to cover them.
		for _, m := range [][3]string{{"2026_08", "2026-08-01", "2026-09-01"}, {"2026_09", "2026-09-01", "2026-10-01"}, {"2026_10", "2026-10-01", "2026-11-01"}} {
			_, err := p.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_%s PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('%s') TO ('%s')`, m[0], m[1], m[2]))
			require.NoError(t, err)
		}
		_, err := p.Exec(ctx, `ANALYZE msg_dispatch_jobs; ANALYZE msg_dispatch_queue`)
		require.NoError(t, err)
		seedPlanData(t, p, s)
	}},
	{"freshly analysed", func(t *testing.T, p *pgxpool.Pool, s planShape) {
		seedPlanData(t, p, s)
		_, err := p.Exec(context.Background(), `ANALYZE msg_dispatch_jobs; ANALYZE msg_dispatch_queue`)
		require.NoError(t, err)
	}},
}

// planNode is the part of an EXPLAIN (ANALYZE, FORMAT JSON) node the assertions read.
type planNode struct {
	ExecTime     float64    `json:"-"`
	NodeType     string     `json:"Node Type"`
	Relation     string     `json:"Relation Name"`
	Index        string     `json:"Index Name"`
	ActualRows   float64    `json:"Actual Rows"`
	Loops        float64    `json:"Actual Loops"`
	RemovedByFlt float64    `json:"Rows Removed by Filter"`
	Plans        []planNode `json:"Plans"`
}

func (n planNode) walk(fn func(planNode)) {
	fn(n)
	for _, c := range n.Plans {
		c.walk(fn)
	}
}

// planOf runs sql as a generic prepared statement and returns its executed plan.
// argSQL is the EXECUTE argument list as SQL literals. The statement runs in a
// transaction that is rolled back, so UPDATE/DELETE/INSERT leave nothing behind.
func planOf(t *testing.T, p *pgxpool.Pool, sql string, argSQL string) planNode {
	t.Helper()
	return planOfMode(t, p, sql, argSQL, "")
}

// planOfMode is planOf with an explicit plan_cache_mode for the statement ("" =
// the connection's own, which on the scheduler's pool is force_custom_plan).
func planOfMode(t *testing.T, p *pgxpool.Pool, sql string, argSQL string, mode string) planNode {
	t.Helper()
	ctx := context.Background()
	conn, err := p.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	_, _ = conn.Exec(ctx, "DEALLOCATE ALL")
	_, err = conn.Exec(ctx, "BEGIN")
	require.NoError(t, err)
	defer func() { _, _ = conn.Exec(ctx, "ROLLBACK") }()
	if mode != "" {
		_, err = conn.Exec(ctx, "SET LOCAL plan_cache_mode = "+mode)
		require.NoError(t, err)
	}
	_, err = conn.Exec(ctx, "PREPARE pv AS "+sql)
	require.NoError(t, err, sql)
	exec := "EXECUTE pv"
	if argSQL != "" {
		exec += "(" + argSQL + ")"
	}
	rs, err := conn.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+exec, pgx.QueryExecModeSimpleProtocol)
	require.NoError(t, err)
	var raw []byte
	for rs.Next() {
		require.NoError(t, rs.Scan(&raw))
	}
	require.NoError(t, rs.Err())
	rs.Close()
	return decodePlan(t, raw)
}

// literal renders a Go argument as a SQL literal for EXECUTE.
func literal(a any) string {
	switch v := a.(type) {
	case time.Time:
		return "'" + v.UTC().Format("2006-01-02 15:04:05.000000") + "+00'"
	case string:
		return "'" + strings.ReplaceAll(v, "'", "''") + "'"
	case []string:
		return "'{" + strings.Join(v, ",") + "}'::text[]"
	case []int32:
		parts := make([]string, len(v))
		for i, n := range v {
			parts[i] = fmt.Sprint(n)
		}
		return "'{" + strings.Join(parts, ",") + "}'::int[]"
	case []time.Time:
		parts := make([]string, len(v))
		for i, ts := range v {
			parts[i] = `"` + ts.UTC().Format("2006-01-02 15:04:05.000000") + `+00"`
		}
		return "'{" + strings.Join(parts, ",") + "}'::timestamptz[]"
	default:
		return fmt.Sprint(v)
	}
}

func literals(args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = literal(a)
	}
	return strings.Join(parts, ", ")
}

// hasNode reports whether any node of the plan matches.
func hasNode(plan planNode, match func(planNode) bool) bool {
	found := false
	plan.walk(func(n planNode) {
		if match(n) {
			found = true
		}
	})
	return found
}

// seqScansOfData lists the sequential scans of the named relation family that
// actually read rows (the empty future partitions are scanned for free).
func seqScansWithData(plan planNode, relPrefix string) []string {
	var out []string
	plan.walk(func(n planNode) {
		if n.NodeType == "Seq Scan" && strings.HasPrefix(n.Relation, relPrefix) && (n.ActualRows+n.RemovedByFlt) > 0 {
			out = append(out, fmt.Sprintf("%s (read %.0f, removed %.0f)", n.Relation, n.ActualRows*max(n.Loops, 1), n.RemovedByFlt))
		}
	})
	return out
}

func forEachState(t *testing.T, shape planShape, fn func(t *testing.T, p *pgxpool.Pool)) {
	for i, st := range planStates {
		t.Run(shape.name+"/"+st.name, func(t *testing.T) {
			p := testpg.ScratchDBWith(t, fmt.Sprintf("plan_%s_%d", shape.name, i), PoolRuntimeParams)
			st.load(t, p, shape)
			fn(t, p)
		})
	}
}

// The hold-back lookups at claim time and at delivery time: FAILED/ERROR holders
// from idx_dispatch_jobs_status_group (status as a bind parameter), future-
// scheduled PENDING holders from the queue by message_group. Nothing scans either
// table sequentially.
func TestPlans_HoldBackUsesTheIndexesAndNeverScansATable(t *testing.T) {
	groups := make([]string, 500)
	for i := range groups {
		groups[i] = fmt.Sprintf("g%d", i*7)
	}
	// The queue is small in the realistic shape (a few thousand rows), where a
	// sequential scan of it is the right plan; the queue half is asserted on the
	// backlog shape, a queue of ~100,000 rows.
	for _, shape := range []planShape{realisticShape, backlogShape} {
		forEachState(t, shape, func(t *testing.T, p *pgxpool.Pool) {
			seqs, created, ids := make([]int32, len(groups)), make([]time.Time, len(groups)), make([]string, len(groups))
			for i := range groups {
				seqs[i], created[i], ids[i] = unboundedKey.sequence, unboundedKey.createdAt, unboundedKey.id
			}
			holders := planOf(t, p, dispatchjob.GroupHoldersSQL, literals([]any{dispatchjob.GroupHoldingStatuses, groups, seqs, created, ids}))
			held := planOf(t, p, dispatchjob.GroupHeldBeforeSQL, literals([]any{
				dispatchjob.GroupHoldingStatuses, "g7", 2, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), "zzz"}))
			t.Logf("TIMING hold-back %s holders %.2f ms held-before %.2f ms", shape.name, holders.ExecTime, held.ExecTime)
			for name, plan := range map[string]planNode{"holders": holders, "held-before": held} {
				assert.Empty(t, seqScansWithData(plan, "msg_dispatch_jobs"), "%s must not scan the job table", name)
				if shape.name == "backlog" {
					assert.Empty(t, seqScansWithData(plan, "msg_dispatch_queue"), "%s must not scan the queue", name)
				}
			}
			assert.True(t, hasNode(holders, func(n planNode) bool {
				return strings.Contains(n.Index, "status_message_group_sequence")
			}), "the FAILED/ERROR half rides idx_dispatch_jobs_status_group")
			if shape.name == "backlog" {
				assert.True(t, hasNode(holders, func(n planNode) bool { return n.Index == "idx_dispatch_queue_order" }),
					"the backoff half rides the queue's order index")
			}
		})
	}
}

// Every sweep that used the dropped partial indexes now rides the status prefix
// of idx_dispatch_jobs_status_group (or the primary key / queue index), with the
// status values as literals in the statements: stale QUEUED and PROCESSING
// recovery, the reaper's stranded-sibling sweep and the three reconcile
// statements.
func TestPlans_SweepsNeverScanTheJobTable(t *testing.T) {
	forEachState(t, realisticShape, func(t *testing.T, p *pgxpool.Pool) {
		var sts []dispatchjob.Statement
		sts = append(sts, dispatchjob.StaleRecoveryStatements(time.Now().Add(-15*time.Minute))...)
		sts = append(sts, dispatchjob.StrandedStatement(time.Now().Add(-45*time.Minute)))
		sts = append(sts, dispatchjob.ReconcileStatements(time.Minute, 5000)...)
		for _, st := range sts {
			plan := planOf(t, p, st.SQL, literals(st.Args))
			t.Logf("TIMING sweep %-18s %8.2f ms", st.Name, plan.ExecTime)
			assert.Empty(t, seqScansWithData(plan, "msg_dispatch_jobs"), "%s: plan scans the job table", st.Name)
			switch st.Name {
			case "stale_queued", "stale_processing", "sweep_stranded", "reconcile_insert", "reconcile_refresh":
				assert.True(t, hasNode(plan, func(n planNode) bool {
					return strings.Contains(n.Index, "status_message_group_sequence")
				}), "%s: must use the status index", st.Name)
			}
		}
	})
}

// decodePlan reads an EXPLAIN (ANALYZE, FORMAT JSON) result.
func decodePlan(t *testing.T, raw []byte) planNode {
	t.Helper()
	var top []struct {
		Plan     planNode `json:"Plan"`
		ExecTime float64  `json:"Execution Time"`
	}
	require.NoError(t, json.Unmarshal(raw, &top), string(raw))
	require.Len(t, top, 1)
	top[0].Plan.ExecTime = top[0].ExecTime
	return top[0].Plan
}
