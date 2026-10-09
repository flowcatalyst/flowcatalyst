//go:build integration

package dispatchjob_test

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

// The requeue-by-filter statements read msg_dispatch_jobs_read with the list's
// filters. This pins their plans on a 400,000-row projection over two monthly
// partitions (1% FAILED, 2% PENDING, 0.02% EXPIRED, the rest COMPLETED; 40 tenants plus
// platform-scoped jobs; 200 codes), analysed: a page of the id walk must not
// read a partition sequentially for a selective filter, and must not sort the
// whole match set when the filter is wide.

const planRows = 400_000

type planNode struct {
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

func (n planNode) shape() string {
	var parts []string
	n.walk(func(c planNode) {
		s := c.NodeType
		if c.Index != "" {
			s += "(" + strings.TrimPrefix(c.Index, "msg_dispatch_jobs_read_") + ")"
		} else if c.Relation != "" {
			s += "(" + strings.TrimPrefix(c.Relation, "msg_dispatch_jobs_read_") + ")"
		}
		parts = append(parts, s)
	})
	return strings.Join(parts, " > ")
}

func (n planNode) seqScansWithData() []string {
	var out []string
	n.walk(func(c planNode) {
		if c.NodeType == "Seq Scan" && strings.HasPrefix(c.Relation, "msg_dispatch_jobs_read") && c.ActualRows+c.RemovedByFlt > 0 {
			out = append(out, fmt.Sprintf("%s (kept %.0f, removed %.0f)", c.Relation, c.ActualRows, c.RemovedByFlt))
		}
	})
	return out
}

func (n planNode) sortedRows() float64 {
	var rows float64
	n.walk(func(c planNode) {
		if strings.Contains(c.NodeType, "Sort") && !strings.Contains(c.NodeType, "Incremental") {
			for _, k := range c.Plans {
				rows += k.ActualRows * max(k.Loops, 1)
			}
		}
	})
	return rows
}

// explain runs the statement as a prepared statement under plan_cache_mode and
// returns the executed plan and its execution time.
func explain(t *testing.T, p *pgxpool.Pool, mode, sql string, args []any) (planNode, float64) {
	t.Helper()
	ctx := context.Background()
	conn, err := p.Acquire(ctx)
	require.NoError(t, err)
	defer conn.Release()
	tx, err := conn.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, "SET LOCAL plan_cache_mode = "+mode)
	require.NoError(t, err)
	rows, err := tx.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+sql, args...)
	require.NoError(t, err)
	raw, err := pgx.CollectOneRow(rows, pgx.RowTo[[]byte])
	require.NoError(t, err)
	var top []struct {
		Plan     planNode `json:"Plan"`
		ExecTime float64  `json:"Execution Time"`
	}
	require.NoError(t, json.Unmarshal(raw, &top), string(raw))
	return top[0].Plan, top[0].ExecTime
}

func seedPlanProjection(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	p := testpg.ScratchDB(t, "rbf_plan")
	now := time.Now().UTC()
	thisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	lastMonth := thisMonth.AddDate(0, -1, 0)
	_, err := p.Exec(ctx, `
	INSERT INTO msg_dispatch_jobs_read (id, code, target_url, client_id, kind, protocol, mode, status, max_retries,
	                                    application, subdomain, aggregate, created_at, updated_at)
	SELECT lpad(n::text, 13, '0'),
	       'app' || (n % 4) || ':sub:agg' || (n % 50) || ':ev' || (n % 200 / 50),
	       'http://x',
	       CASE WHEN n % 41 = 0 THEN NULL ELSE 'clt_plan' || lpad((n % 40)::text, 8, '0') END,
	       'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE',
	       CASE WHEN n % 5000 = 11 THEN 'EXPIRED' WHEN n % 100 = 7 THEN 'FAILED' WHEN n % 100 IN (3, 4) THEN 'PENDING' ELSE 'COMPLETED' END,
	       3, 'app' || (n % 4), 'sub', 'agg' || (n % 50),
	       ts, ts
	  FROM (SELECT n, CASE WHEN n <= $3 / 2 THEN $1::timestamptz ELSE $2::timestamptz END
	                  + ((n % ($3 / 2)) * interval '5 seconds') AS ts
	          FROM generate_series(1, $3::int) n) s`, lastMonth, thisMonth, planRows)
	require.NoError(t, err)
	_, err = p.Exec(ctx, `ANALYZE msg_dispatch_jobs_read`)
	require.NoError(t, err)
	return p
}

func TestRequeueByFilterPlans(t *testing.T) {
	p := seedPlanProjection(t)
	accessible := []string{"clt_plan00000007", "clt_plan00000008"}
	mid := fmt.Sprintf("%013d", planRows/2)
	cases := []struct {
		name string
		f    dispatchjob.FilterParams
		// selective: the walk must reach its rows by index, never a partition scan.
		selective bool
	}{
		{"error statuses", dispatchjob.FilterParams{Statuses: []string{"FAILED", "ERROR"}}, true},
		{"error statuses after cursor", dispatchjob.FilterParams{Statuses: []string{"FAILED", "ERROR"}, AfterID: &mid}, true},
		{"one tenant, failed", dispatchjob.FilterParams{ClientIDs: []string{"clt_plan00000007"}, Statuses: []string{"FAILED"}}, true},
		{"client-scoped caller, failed", dispatchjob.FilterParams{Statuses: []string{"FAILED"}, AccessibleClientIDs: &accessible, ExcludePlatformScoped: true}, true},
		{"one code", dispatchjob.FilterParams{Codes: []string{"app1:sub:agg1:ev0"}}, true},
		{"sparse status (0.02%)", dispatchjob.FilterParams{Statuses: []string{"EXPIRED"}}, true},
		{"no filter", dispatchjob.FilterParams{}, false},
		{"completed (wide)", dispatchjob.FilterParams{Statuses: []string{"COMPLETED"}}, false},
	}
	for _, mode := range []string{"force_custom_plan", "force_generic_plan"} {
		for _, c := range cases {
			t.Run(mode+"/"+c.name, func(t *testing.T) {
				q, args := dispatchjob.MatchingIDsSQL(c.f, 500)
				walk, walkMs := explain(t, p, mode, q, args)
				q, args = dispatchjob.CountMatchesSQL(c.f)
				count, countMs := explain(t, p, mode, q, args)
				t.Logf("PLAN %-18s %-30s walk %7.2f ms  %s", mode, c.name, walkMs, walk.shape())
				t.Logf("PLAN %-18s %-30s count %7.2f ms  %s", mode, c.name, countMs, count.shape())
				if c.selective {
					assert.Empty(t, walk.seqScansWithData(), "a selective walk page must not scan a partition")
				} else {
					assert.Zero(t, walk.sortedRows(), "a wide walk page must stream the id index, not sort the match set")
				}
				assert.Less(t, walkMs, 500.0, "one page of the walk stays cheap")
			})
		}
	}
}
