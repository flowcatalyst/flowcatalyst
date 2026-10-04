//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// explain returns the plan Postgres would run for sql, with the scan types
// that could stand in for a missing index turned off for the transaction. A
// plan produced that way says what the INDEXES can do for the statement,
// independent of table sizes and statistics (an embedded test database is
// nearly empty, where a sequential scan is always cheapest).
func explain(t *testing.T, sql string, args ...any) string {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback(ctx) }()
	_, err = tx.Exec(ctx, `SET LOCAL enable_seqscan = off`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SET LOCAL enable_bitmapscan = off`)
	require.NoError(t, err)
	// The simple protocol sends the arguments inline, so EXPLAIN (which cannot
	// be prepared with parameters of unknown type) sees one whole statement.
	rows, err := tx.Query(ctx, "EXPLAIN (COSTS OFF) "+sql, append([]any{pgx.QueryExecModeSimpleProtocol}, args...)...)
	require.NoError(t, err)
	defer rows.Close()
	var b strings.Builder
	for rows.Next() {
		var line string
		require.NoError(t, rows.Scan(&line))
		b.WriteString(line)
		b.WriteByte('\n')
	}
	require.NoError(t, rows.Err())
	return b.String()
}

// sortNode matches a Sort or Incremental Sort plan node (not the "Sort Key:"
// line a Merge Append prints).
var sortNode = regexp.MustCompile(`(?m)^\s*(->\s+)?(Incremental )?Sort\s*$`)

// The claim orders by (message_group NULLS LAST, sequence, created_at, id).
// idx_dispatch_jobs_pending_poll has exactly that key, so the index alone
// yields the order, across partitions. Were the index to stop short of id (as
// it did before migration 065), every plan would need a sort on top: one that
// reads every tied row before the LIMIT can stop.
func TestClaimPlan_NeedsNoSort(t *testing.T) {
	// An empty exclusion list and a full one: the in-flight set is a filter on the
	// rows the index walk visits, not something that may turn it into a sort.
	inflight := make([]string, 1000)
	for i := range inflight {
		inflight[i] = fmt.Sprintf("djinflight%05d", i)
	}
	for name, arr := range map[string][]string{"empty": {}, "1000 in flight": inflight} {
		plan := explain(t, claimSQL, 500, []string{}, arr)
		assert.Contains(t, plan, "Merge Append", "%s: ordered index scans merged across partitions:\n%s", name, plan)
		assert.NotRegexp(t, sortNode, plan, "%s: the pending-poll index must supply the claim's whole order:\n%s", name, plan)
		assert.NotContains(t, plan, "Seq Scan", "%s: plan:\n%s", name, plan)
	}
}

// Both stale sweeps are served by idx_dispatch_jobs_in_flight (status,
// updated_at): the partitions' copies of it are named ..._status_updated_at_idx.
func TestStaleSweepPlans_UseTheInFlightIndex(t *testing.T) {
	for name, sql := range map[string]string{"queued": staleQueuedSQL, "processing": staleProcessingSQL} {
		args := []any{"2026-01-01T00:00:00Z"}
		if name == "processing" {
			args = append(args, "reason")
		}
		plan := explain(t, sql, args...)
		assert.Contains(t, plan, "status_updated_at_idx", "%s sweep plan:\n%s", name, plan)
		assert.NotContains(t, plan, "Seq Scan", "%s sweep plan:\n%s", name, plan)
	}
}
