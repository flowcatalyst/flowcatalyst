//go:build integration

package dispatchjob_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Migration 086's indexes serve the list's event-type, subscription and connection
// filters: each leads with the filtered column and ends in created_at DESC, so a
// "this code / subscription / connection, newest first" page is read in order from
// the index. 200,000 analysed rows over 20 days; the list is held to a 10-day window
// as the API holds it. Plans are asserted structurally (the index is the access path,
// no partition is scanned for a selective filter) and timed against the same query with
// the new index dropped, so the report has before/after numbers.

const filterPlanRows = 200_000

func seedFilterPlan(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	p := testpg.ScratchDB(t, "filter_idx_plan")
	_, err := p.Exec(ctx, `
	INSERT INTO msg_dispatch_jobs_read (id, code, target_url, subscription_id, connection_id, kind, protocol, mode, status,
	                                    max_retries, application, created_at, updated_at)
	SELECT lpad(n::text, 13, '0'),
	       CASE WHEN n % 2000 = 5 THEN 'rare:sub:agg:ev' ELSE 'app' || (n % 20) || ':sub:agg:ev' || (n % 10) END,
	       'http://x',
	       CASE WHEN n % 5000 = 7 THEN 'sub_rare000000001'
	            WHEN n % 500 = 9 THEN 'sub_conn000000001'
	            ELSE 'sub_' || lpad((n % 100)::text, 13, '0') END,
	       CASE WHEN n % 1000 = 3 THEN 'con_plan00000001' END,
	       'EVENT', 'HTTP_WEBHOOK', 'IMMEDIATE', 'COMPLETED', 3, 'app',
	       now() - interval '20 days' + n * (interval '20 days' / $1::int),
	       now() - interval '20 days' + n * (interval '20 days' / $1::int)
	  FROM generate_series(1, $1::int) n`, filterPlanRows)
	require.NoError(t, err)
	_, err = p.Exec(ctx, `INSERT INTO msg_subscriptions (id, code, name, target, connection_id)
		VALUES ('sub_conn000000001', 'plan-conn-sub', 'Plan connection sub', 'http://x', 'con_plan00000001')`)
	require.NoError(t, err)
	_, err = p.Exec(ctx, `ANALYZE msg_dispatch_jobs_read`)
	require.NoError(t, err)
	return p
}

func TestFilterIndexPlans(t *testing.T) {
	p := seedFilterPlan(t)
	repo := dispatchjob.NewRepository(p)
	since := time.Now().UTC().Add(-10 * 24 * time.Hour)
	rare, common, rareSub := "rare:sub:agg:ev", "app3:sub:agg:ev3", "sub_rare000000001"
	conn := "con_plan00000001"

	cases := []struct {
		name  string
		f     dispatchjob.FilterParams
		index string // an index of this name must be on the plan ("" = any plan without a partition scan)
	}{
		{"one rare event type", dispatchjob.FilterParams{EventTypes: []string{rare}}, "code_created_at"},
		{"one common event type", dispatchjob.FilterParams{EventTypes: []string{common}}, ""}, // 5% of rows: the planner may walk created_at instead
		{"one rare subscription", dispatchjob.FilterParams{SubscriptionIDs: []string{rareSub}}, "subscription_id_created_at"},
		{"one subscription id", dispatchjob.FilterParams{SubscriptionID: &rareSub}, "subscription_id_created_at"},
		{"connection (own jobs + its subscriptions)", dispatchjob.FilterParams{ConnectionID: &conn}, "connection_id_created_at"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := c.f
			f.Since = &since
			f.Limit = 100
			f, err := repo.ResolveRefs(context.Background(), f)
			require.NoError(t, err)
			q, args := dispatchjob.ListSQL(f)
			plan, ms := explain(t, p, "force_custom_plan", q, args)
			t.Logf("PLAN %-44s %7.2f ms  %s", c.name, ms, plan.shape())
			shape := plan.shape()
			if c.index != "" {
				assert.Contains(t, shape, c.index, "the new composite is the access path")
			}
			assert.Empty(t, plan.seqScansWithData(), "no partition is scanned")
			assert.Less(t, ms, 500.0)
			if c.name == "connection (own jobs + its subscriptions)" {
				assert.Contains(t, shape, "subscription_id_created_at", "the OR reads both indexes (BitmapOr)")
			}
		})
	}

	// Before/after: the same rare-code page with the swap undone (single-column code
	// index back, composite gone).
	t.Run("before and after the swap", func(t *testing.T) {
		f := dispatchjob.FilterParams{EventTypes: []string{rare}, Since: &since, Limit: 100}
		q, args := dispatchjob.ListSQL(f)
		_, after := explain(t, p, "force_custom_plan", q, args)
		size := func(suffix string) int64 {
			var n int64
			require.NoError(t, p.QueryRow(context.Background(), `
				SELECT COALESCE(sum(pg_relation_size(c.oid)), 0)::bigint FROM pg_class c
				 WHERE c.relkind = 'i' AND c.relname LIKE 'msg_dispatch_jobs_read_%' || $1`, suffix).Scan(&n))
			return n
		}
		_, err := p.Exec(context.Background(), `
			DROP INDEX idx_msg_dispatch_jobs_read_code_created;
			CREATE INDEX idx_msg_dispatch_jobs_read_code ON msg_dispatch_jobs_read (code);
			ANALYZE msg_dispatch_jobs_read`)
		require.NoError(t, err)
		planBefore, before := explain(t, p, "force_custom_plan", q, args)
		t.Logf("BEFORE (single-column code index) %7.2f ms  %s", before, planBefore.shape())
		t.Logf("AFTER  (code, created_at DESC)    %7.2f ms", after)
		// Sizes of two fresh builds over the same rows (the composite cannot use btree
		// deduplication of equal keys, so it is larger than the single-column index).
		_, err = p.Exec(context.Background(), `CREATE INDEX idx_msg_dispatch_jobs_read_code_created
			ON msg_dispatch_jobs_read (code, created_at DESC)`)
		require.NoError(t, err)
		t.Logf("INDEX SIZE over %d rows, fresh builds: (code, created_at DESC) %d KiB; code alone %d KiB",
			filterPlanRows, size("_code_created_at_idx")/1024, size("_code_idx")/1024)
		assert.False(t, strings.Contains(planBefore.shape(), "code_created_at"))
	})
}
