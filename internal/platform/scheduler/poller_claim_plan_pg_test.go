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

// Plan tests for the claim's two statements (S1 the ordered walk, S2 the delete
// that takes the rows) and the restore, in the statistics states that broke the
// single-statement claim: never analysed; analysed while brand new and empty;
// DRAINED, then analysed with the pages still allocated, then a burst; freshly
// analysed; analysed when every row present was scheduled for the future; and the
// cached-plan case (a prepared statement planned on an empty queue, reused after
// a burst). Connections carry PoolRuntimeParams. They assert plan SHAPE, not time;
// the timings are logged (go test -v).

const claimPlanLimit = 500

// seedClaimData loads n PENDING jobs and (unless the state says otherwise) their
// queue rows, ordered over 500 message groups.
func seedClaimJobs(t *testing.T, p *pgxpool.Pool, n int) {
	t.Helper()
	ctx := context.Background()
	month := time.Now().UTC().Format("2006-01") // the current month's partition
	start, err := time.Parse("2006-01", month)
	require.NoError(t, err)
	_, err = p.Exec(ctx, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS msg_dispatch_jobs_%s PARTITION OF msg_dispatch_jobs FOR VALUES FROM ('%s') TO ('%s')`,
		start.Format("2006_01"), start.Format("2006-01-02"), start.AddDate(0, 1, 0).Format("2006-01-02")))
	require.NoError(t, err)
	rows, err := p.Query(ctx, `SELECT c.relname FROM pg_inherits i JOIN pg_class c ON c.oid = i.inhrelid WHERE i.inhparent = 'msg_dispatch_jobs'::regclass`)
	require.NoError(t, err)
	tables := []string{"msg_dispatch_queue"}
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		tables = append(tables, name)
	}
	rows.Close()
	for _, tb := range tables {
		_, err := p.Exec(ctx, "ALTER TABLE "+tb+" SET (autovacuum_enabled = false)")
		require.NoError(t, err)
	}
	_, err = p.Exec(ctx, fmt.Sprintf(`
	INSERT INTO msg_dispatch_jobs (id, code, target_url, status, message_group, sequence, created_at, updated_at, mode)
	SELECT 'j' || lpad(n::text, 12, '0'), 'c', 'http://x', 'PENDING', 'g' || lpad((n %% 500)::text, 4, '0'), (n / 500)::int,
	       $1::timestamptz + (n * interval '1 second'), $1::timestamptz + (n * interval '1 second'), 'BLOCK_ON_ERROR'
	  FROM generate_series(1, %d) n`, n), start)
	require.NoError(t, err)
}

func burstQueue(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	_, err := p.Exec(context.Background(), `INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
	        subscription_id, dispatch_pool_id, client_id, mode, queue, version)
	SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id, dispatch_pool_id, client_id, mode, queue, updated_at
	  FROM msg_dispatch_jobs WHERE status = 'PENDING'`)
	require.NoError(t, err)
}

type claimState struct {
	name string
	// prepare brings the (empty) database to the state, with n jobs seeded and the
	// queue filled the way the state needs.
	prepare func(t *testing.T, p *pgxpool.Pool, n int)
}

func analyze(t *testing.T, p *pgxpool.Pool) {
	t.Helper()
	_, err := p.Exec(context.Background(), `ANALYZE msg_dispatch_jobs; ANALYZE msg_dispatch_queue`)
	require.NoError(t, err)
}

var claimStates = []claimState{
	{"never analysed", func(t *testing.T, p *pgxpool.Pool, n int) {
		seedClaimJobs(t, p, n)
		burstQueue(t, p)
	}},
	{"analysed while new and empty", func(t *testing.T, p *pgxpool.Pool, n int) {
		analyze(t, p) // before any data
		seedClaimJobs(t, p, n)
		burstQueue(t, p)
	}},
	{"drained, analysed, then burst", func(t *testing.T, p *pgxpool.Pool, n int) {
		seedClaimJobs(t, p, n)
		burstQueue(t, p)
		_, err := p.Exec(context.Background(), `DELETE FROM msg_dispatch_queue`) // drained: pages stay allocated
		require.NoError(t, err)
		analyze(t, p) // the planner now believes the queue holds about one row
		burstQueue(t, p)
	}},
	{"freshly analysed", func(t *testing.T, p *pgxpool.Pool, n int) {
		seedClaimJobs(t, p, n)
		burstQueue(t, p)
		analyze(t, p)
	}},
	{"analysed while all future", func(t *testing.T, p *pgxpool.Pool, n int) {
		seedClaimJobs(t, p, n)
		burstQueue(t, p)
		_, err := p.Exec(context.Background(), `UPDATE msg_dispatch_queue SET scheduled_for = NOW() + interval '1 hour'`)
		require.NoError(t, err)
		analyze(t, p)
		_, err = p.Exec(context.Background(), `UPDATE msg_dispatch_queue SET scheduled_for = NULL`) // now all due
		require.NoError(t, err)
	}},
}

// claimStatementPlans returns the plans of S1, S2 and the restore for the first
// claimPlanLimit rows of the order, as the poller would run them.
func claimStatementPlans(t *testing.T, p *pgxpool.Pool) (s1, s2, restore planNode) {
	t.Helper()
	ctx := context.Background()
	held := make([]string, 20)
	for i := range held {
		held[i] = fmt.Sprintf("g%04d", 400+i)
	}
	sel, del := dispatchjob.ClaimQueueStatements(claimPlanLimit, []string{}, held, nil)
	s1 = planOf(t, p, sel.SQL, literals(sel.Args))

	rows, err := p.Query(ctx, sel.SQL, sel.Args...)
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	require.NoError(t, err)
	require.Len(t, ids, claimPlanLimit)
	s2 = planOf(t, p, del.SQL, literals([]any{ids}))

	rows, err = p.Query(ctx, `SELECT created_at FROM msg_dispatch_jobs WHERE id = ANY($1) ORDER BY id`, ids)
	require.NoError(t, err)
	created, err := pgx.CollectRows(rows, pgx.RowTo[time.Time])
	require.NoError(t, err)
	// restore needs (id, created_at) pairs aligned: sort ids the same way
	sorted := append([]string(nil), ids...)
	sortStrings(sorted)
	rs := dispatchjob.RestoreStatement(sorted, created)
	restore = planOf(t, p, rs.SQL, literals(rs.Args))
	return s1, s2, restore
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func queueScans(plan planNode) (seq bool, indexes []string) {
	plan.walk(func(n planNode) {
		if n.Relation == "msg_dispatch_queue" && n.NodeType == "Seq Scan" {
			seq = true
		}
		if n.Index != "" && (n.Relation == "msg_dispatch_queue" || strings.HasPrefix(n.NodeType, "Bitmap Index") || strings.Contains(n.NodeType, "Index")) {
			if n.Index == "msg_dispatch_queue_pkey" || n.Index == "idx_dispatch_queue_order" {
				indexes = append(indexes, n.Index)
			}
		}
	})
	return seq, indexes
}

func TestPlans_ClaimStatementsInEveryStatisticsState(t *testing.T) {
	for _, n := range []int{5_000, 100_000} {
		for si, st := range claimStates {
			t.Run(fmt.Sprintf("%d rows/%s", n, st.name), func(t *testing.T) {
				p := testpg.ScratchDBWith(t, fmt.Sprintf("claimplan_%d_%d", n, si), PoolRuntimeParams)
				st.prepare(t, p, n)
				s1, s2, restore := claimStatementPlans(t, p)

				// S1: the ordered walk. No Sort, no Seq Scan, whatever the statistics say.
				assert.False(t, hasNode(s1, func(x planNode) bool { return strings.Contains(x.NodeType, "Sort") }), "S1 must not sort")
				seq, idx := queueScans(s1)
				assert.False(t, seq, "S1 must not scan the queue sequentially")
				assert.Contains(t, idx, "idx_dispatch_queue_order", "S1 walks the order index")

				// S2: the delete by id. At 100,000 rows never a sequential scan; an index
				// scan of the primary key (or, only in the drained state, the order index).
				seq, idx = queueScans(s2)
				if n >= 100_000 {
					assert.False(t, seq, "S2 must not scan the queue sequentially at 100,000 rows")
					assert.NotEmpty(t, idx, "S2 must use an index")
					if st.name != "drained, analysed, then burst" {
						assert.NotContains(t, idx, "idx_dispatch_queue_order", "S2 uses the primary key")
					}
				}

				// restore: the job table by primary key, never scanned.
				assert.Empty(t, seqScansWithData(restore, "msg_dispatch_jobs"), "restore must not scan the job table")
				if !assert.True(t, hasNode(restore, func(x planNode) bool {
					return strings.HasSuffix(x.Index, "_pkey") && strings.HasPrefix(x.Index, "msg_dispatch_jobs")
				}), "restore reads msg_dispatch_jobs by primary key") {
					restore.walk(func(x planNode) {
						t.Logf("restore node: %s rel=%s idx=%s rows=%.0f", x.NodeType, x.Relation, x.Index, x.ActualRows)
					})
				}

				t.Logf("TIMING %-7d %-30s S1 %7.2f ms  S2 %7.2f ms  restore %7.2f ms", n, st.name, s1.ExecTime, s2.ExecTime, restore.ExecTime)
			})
		}
	}
}

// The cached-plan case: both statements are run several times, on ONE connection,
// while the queue is empty and vacuumed; then a burst arrives and the claim runs on
// the same connection. Left to plan_cache_mode = auto, Postgres switches to a
// generic plan made while the queue was empty — a sequential scan — and reuses it
// after the burst. With the scheduler's settings it must not. The negative control
// runs the same case on a pool WITHOUT the settings.
func TestPlans_CachedPlanAfterABurst(t *testing.T) {
	for _, n := range []int{5_000, 100_000} {
		run := func(t *testing.T, params map[string]string, name string) (s1, s2 planNode) {
			p := testpg.ScratchDBWith(t, name, params)
			seedClaimJobs(t, p, n)
			ctx := context.Background()
			conn, err := p.Acquire(ctx)
			require.NoError(t, err)
			defer conn.Release()
			_, err = conn.Exec(ctx, "VACUUM msg_dispatch_queue")
			require.NoError(t, err)
			_, err = conn.Exec(ctx, `PREPARE s1 AS `+strings.ReplaceAll(dispatchjob.ClaimSelectSQL(), "\n", " "))
			require.NoError(t, err)
			_, err = conn.Exec(ctx, `PREPARE s2 AS `+strings.ReplaceAll(dispatchjob.ClaimDeleteSQL(), "\n", " "))
			require.NoError(t, err)
			for range 8 { // the queue is empty: Postgres may now settle on a generic plan
				_, err = conn.Exec(ctx, `EXECUTE s1(500, '{}'::text[], '{}'::text[])`, pgx.QueryExecModeSimpleProtocol)
				require.NoError(t, err)
				_, err = conn.Exec(ctx, `EXECUTE s2('{}'::text[])`, pgx.QueryExecModeSimpleProtocol)
				require.NoError(t, err)
			}
			_, err = conn.Exec(ctx, `INSERT INTO msg_dispatch_queue (job_id, job_created_at, message_group, sequence, scheduled_for,
			        subscription_id, dispatch_pool_id, client_id, mode, queue, version)
			SELECT id, created_at, message_group, sequence, scheduled_for, subscription_id, dispatch_pool_id, client_id, mode, queue, updated_at
			  FROM msg_dispatch_jobs WHERE status = 'PENDING'`) // the burst
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
			s1 = explain(`EXECUTE s1(500, '{}'::text[], '{}'::text[])`)
			rows, err := conn.Query(ctx, `SELECT job_id FROM msg_dispatch_queue ORDER BY message_group NULLS LAST, sequence, job_created_at, job_id LIMIT 500`)
			require.NoError(t, err)
			ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
			require.NoError(t, err)
			s2 = explain(`EXECUTE s2(` + literal(ids) + `)`)
			return s1, s2
		}
		bad := func(s1, s2 planNode) bool {
			seq1, _ := queueScans(s1)
			seq2, _ := queueScans(s2)
			return seq1 || seq2 || hasNode(s1, func(x planNode) bool { return strings.Contains(x.NodeType, "Sort") })
		}
		t.Run(fmt.Sprintf("%d rows/with the scheduler's settings", n), func(t *testing.T) {
			s1, s2 := run(t, PoolRuntimeParams, fmt.Sprintf("cachedplan_on_%d", n))
			seq1, _ := queueScans(s1)
			assert.False(t, seq1, "S1 must not scan the queue sequentially after the burst")
			assert.False(t, hasNode(s1, func(x planNode) bool { return strings.Contains(x.NodeType, "Sort") }), "S1 must not sort after the burst")
			if n >= 100_000 {
				seq2, _ := queueScans(s2)
				assert.False(t, seq2, "S2 must not scan the queue sequentially at 100,000 rows after the burst")
			}
			t.Logf("TIMING %-7d cached plan, settings ON  S1 %7.2f ms  S2 %7.2f ms", n, s1.ExecTime, s2.ExecTime)
		})
		t.Run(fmt.Sprintf("%d rows/negative control without the settings", n), func(t *testing.T) {
			s1, s2 := run(t, nil, fmt.Sprintf("cachedplan_off_%d", n))
			t.Logf("TIMING %-7d cached plan, settings OFF S1 %7.2f ms  S2 %7.2f ms  (bad plan reproduced: %v)", n, s1.ExecTime, s2.ExecTime, bad(s1, s2))
			if n >= 100_000 {
				assert.True(t, bad(s1, s2), "negative control: without the settings the cached plan is bad (a Seq Scan or a Sort)")
			}
		})
	}
}

// The settings reach the server: every connection of a pool opened with
// PoolRuntimeParams reports them, and a pool opened without them keeps the defaults.
func TestPlans_PoolRuntimeParamsAreAppliedToEveryConnection(t *testing.T) {
	ctx := context.Background()
	on := testpg.ScratchDBWith(t, "poolparams_on", PoolRuntimeParams)
	off := testpg.ScratchDB(t, "poolparams_off")
	for name, c := range map[string]struct {
		p        *pgxpool.Pool
		cache    string
		enableSt string
	}{"scheduler": {on, "force_custom_plan", "off"}, "other": {off, "auto", "on"}} {
		for range 3 {
			var cache, sorts string
			require.NoError(t, c.p.QueryRow(ctx, `SELECT current_setting('plan_cache_mode'), current_setting('enable_sort')`).Scan(&cache, &sorts))
			assert.Equal(t, c.cache, cache, name)
			assert.Equal(t, c.enableSt, sorts, name)
		}
	}
}
