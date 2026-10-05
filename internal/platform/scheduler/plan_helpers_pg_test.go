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
	"github.com/stretchr/testify/require"
)

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
	_, err = conn.Exec(ctx, "BEGIN")
	require.NoError(t, err)
	defer func() {
		_, _ = conn.Exec(ctx, "ROLLBACK")
		_, _ = conn.Exec(ctx, "DEALLOCATE pv") // only our own statement: pgx caches others on this connection
	}()
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
