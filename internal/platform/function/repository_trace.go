package function

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// This file is the fng_trace_contexts / fng_trace_log sibling of
// repository_routes.go: hand-rolled pgx SQL, not sqlc, for the same reason (a
// small self-contained surface; internal/sqlc/queries/function.sql is shared
// with concurrent work).

const traceContextColumns = `function_id, context, enabled_until, indefinite, sample_rate, enabled_by, updated_at`

type traceContextRow struct {
	FunctionID   string     `db:"function_id"`
	Context      string     `db:"context"`
	EnabledUntil *time.Time `db:"enabled_until"`
	Indefinite   bool       `db:"indefinite"`
	SampleRate   int32      `db:"sample_rate"`
	EnabledBy    *string    `db:"enabled_by"`
	UpdatedAt    time.Time  `db:"updated_at"`
}

func rowToTraceContext(r traceContextRow) TraceContext { return TraceContext(r) }

// GetTraceContext loads one context row; nil when it was never switched on.
func (r *Repository) GetTraceContext(ctx context.Context, functionID, context_ string) (*TraceContext, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+traceContextColumns+` FROM fng_trace_contexts
		WHERE function_id = $1 AND context = $2`, functionID, context_)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[traceContextRow])
	rr, err := repocommon.One(row, err, "function trace repo")
	if rr == nil || err != nil {
		return nil, err
	}
	out := rowToTraceContext(*rr)
	return &out, nil
}

// ListTraceContexts returns every context row of a function, by name.
func (r *Repository) ListTraceContexts(ctx context.Context, functionID string) ([]TraceContext, error) {
	return r.queryTraceContexts(ctx, `function_id = $1`, functionID)
}

// ListTraceContextsFor returns the context rows of the given functions (the
// ingest endpoint's accept check, one query per batch).
func (r *Repository) ListTraceContextsFor(ctx context.Context, functionIDs []string) ([]TraceContext, error) {
	if len(functionIDs) == 0 {
		return nil, nil
	}
	return r.queryTraceContexts(ctx, `function_id = ANY($1)`, functionIDs)
}

// ListLiveTraceContexts returns the function's windows open at now, by name:
// what the desired document carries.
func (r *Repository) ListLiveTraceContexts(ctx context.Context, functionID string, now time.Time) ([]TraceContext, error) {
	return r.queryTraceContexts(ctx, `function_id = $1 AND enabled_until > $2`, functionID, now)
}

func (r *Repository) queryTraceContexts(ctx context.Context, where string, args ...any) ([]TraceContext, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+traceContextColumns+` FROM fng_trace_contexts WHERE `+where+
		` ORDER BY function_id, context`, args...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[traceContextRow])
	if err != nil {
		return nil, err
	}
	out := make([]TraceContext, 0, len(collected))
	for _, row := range collected {
		out = append(out, rowToTraceContext(row))
	}
	return out, nil
}

// UpsertTraceContextTx opens (or re-opens) a context window.
func (r *Repository) UpsertTraceContextTx(ctx context.Context, c *TraceContext, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fng_trace_contexts (function_id, context, enabled_until, indefinite, sample_rate, enabled_by, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (function_id, context) DO UPDATE
		   SET enabled_until = EXCLUDED.enabled_until,
		       indefinite    = EXCLUDED.indefinite,
		       sample_rate   = EXCLUDED.sample_rate,
		       enabled_by    = EXCLUDED.enabled_by,
		       updated_at    = EXCLUDED.updated_at`,
		c.FunctionID, c.Context, c.EnabledUntil, c.Indefinite, c.SampleRate, c.EnabledBy, c.UpdatedAt)
	return err
}

// DisableTraceContextTx closes a context window at at: enabled_until becomes
// at (not NULL), so records captured just before the switch-off and still
// queued in a runner are accepted within the ingest grace. A window that had
// already closed keeps its earlier end. A context that was never switched on
// has no row and stays that way (idempotent).
func (r *Repository) DisableTraceContextTx(ctx context.Context, functionID, context_ string, at time.Time, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `UPDATE fng_trace_contexts
		   SET enabled_until = CASE WHEN enabled_until IS NULL OR enabled_until > $3 THEN $3 ELSE enabled_until END,
		       indefinite = false, updated_at = $3
		 WHERE function_id = $1 AND context = $2`, functionID, context_, at)
	return err
}

// ListLiveTraceFunctionIDs returns the functions that have a window open at
// now (the per-function retention prune's work list).
func (r *Repository) ListLiveTraceFunctionIDs(ctx context.Context, now time.Time) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT function_id FROM fng_trace_contexts WHERE enabled_until > $1 ORDER BY function_id`, now)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// FunctionIDsByAddress maps each known address to its function id; unknown
// addresses are absent.
func (r *Repository) FunctionIDsByAddress(ctx context.Context, addresses []string) (map[string]string, error) {
	out := make(map[string]string, len(addresses))
	if len(addresses) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT address, id FROM fng_functions WHERE address = ANY($1)`, addresses)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var address, id string
		if err := rows.Scan(&address, &id); err != nil {
			return nil, err
		}
		out[address] = id
	}
	return out, rows.Err()
}

// InsertTraceRecords appends records in one statement. The caller has
// already applied every limit; ids are assigned by the caller (from the
// runner's record id when it sent one), and a record whose id is already
// stored is skipped, so a batch the runner retries is not stored twice.
func (r *Repository) InsertTraceRecords(ctx context.Context, recs []TraceLogRecord) error {
	if len(recs) == 0 {
		return nil
	}
	n := len(recs)
	ids, fids, invs, ctxs, kinds, levels, msgs := make([]string, n), make([]string, n), make([]string, n),
		make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	versions := make([]int32, n)
	corrs, refs, data, runners := make([]*string, n), make([]*string, n), make([]*string, n), make([]*string, n)
	ats, created := make([]time.Time, n), make([]time.Time, n)
	for i, rec := range recs {
		ids[i], fids[i], invs[i], ctxs[i] = rec.ID, rec.FunctionID, rec.InvocationID, rec.Context
		kinds[i], levels[i], msgs[i] = rec.Kind, rec.Level, rec.Message
		versions[i], corrs[i], refs[i], runners[i] = rec.Version, rec.CorrelationID, rec.Reference, rec.RunnerID
		ats[i], created[i] = rec.At, rec.CreatedAt
		if rec.Data != nil {
			s := string(rec.Data)
			data[i] = &s
		}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO fng_trace_log (id, function_id, version, invocation_id, correlation_id, reference,
		                           context, kind, level, message, data, at, runner_id, created_at)
		SELECT t.id, t.function_id, t.version, t.invocation_id, t.correlation_id, t.reference,
		       t.context, t.kind, t.level, t.message, t.data::jsonb, t.at, t.runner_id, t.created_at
		  FROM unnest($1::text[], $2::text[], $3::int[], $4::text[], $5::text[], $6::text[],
		              $7::text[], $8::text[], $9::text[], $10::text[], $11::text[], $12::timestamptz[],
		              $13::text[], $14::timestamptz[])
		       AS t(id, function_id, version, invocation_id, correlation_id, reference,
		            context, kind, level, message, data, at, runner_id, created_at)
		ON CONFLICT (id) DO NOTHING`,
		ids, fids, versions, invs, corrs, refs, ctxs, kinds, levels, msgs, data, ats, runners, created)
	return err
}

// QueryTraceLog returns a function's records newest first, keyset-paged on
// (at, id). It returns at most f.Limit rows; ask for one more than the page
// to learn whether another page exists.
func (r *Repository) QueryTraceLog(ctx context.Context, f TraceLogFilter) ([]TraceLogRecord, error) {
	var (
		where = []string{"function_id = $1"}
		args  = []any{f.FunctionID}
	)
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, strings.Replace(cond, "?", fmt.Sprintf("$%d", len(args)), 1))
	}
	if f.Context != "" {
		add("context = ?", f.Context)
	}
	if f.Kind != "" {
		add("kind = ?", f.Kind)
	}
	if f.Level != "" {
		add("level = ?", f.Level)
	}
	if f.Reference != "" {
		add("reference = ?", f.Reference)
	}
	if f.CorrelationID != "" {
		add("correlation_id = ?", f.CorrelationID)
	}
	if f.InvocationID != "" {
		add("invocation_id = ?", f.InvocationID)
	}
	if f.Version > 0 {
		add("version = ?", f.Version)
	}
	if f.Since != nil {
		add("at >= ?", *f.Since)
	}
	if f.Until != nil {
		add("at <= ?", *f.Until)
	}
	if f.After != nil {
		args = append(args, f.After.At, f.After.ID)
		where = append(where, fmt.Sprintf("(at, id) < ($%d, $%d)", len(args)-1, len(args)))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)
	rows, err := r.pool.Query(ctx, `
		SELECT id, function_id, version, invocation_id, correlation_id, reference, context, kind, level,
		       message, data::text AS data, at, runner_id, created_at
		  FROM fng_trace_log
		 WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(`
		 ORDER BY at DESC, id DESC
		 LIMIT $%d`, len(args)), args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		ID            string    `db:"id"`
		FunctionID    string    `db:"function_id"`
		Version       int32     `db:"version"`
		InvocationID  string    `db:"invocation_id"`
		CorrelationID *string   `db:"correlation_id"`
		Reference     *string   `db:"reference"`
		Context       string    `db:"context"`
		Kind          string    `db:"kind"`
		Level         string    `db:"level"`
		Message       string    `db:"message"`
		Data          *string   `db:"data"`
		At            time.Time `db:"at"`
		RunnerID      *string   `db:"runner_id"`
		CreatedAt     time.Time `db:"created_at"`
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, err
	}
	out := make([]TraceLogRecord, 0, len(collected))
	for _, c := range collected {
		rec := TraceLogRecord{
			ID: c.ID, FunctionID: c.FunctionID, Version: c.Version, InvocationID: c.InvocationID,
			CorrelationID: c.CorrelationID, Reference: c.Reference, Context: c.Context, Kind: c.Kind,
			Level: c.Level, Message: c.Message, At: c.At, RunnerID: c.RunnerID, CreatedAt: c.CreatedAt,
		}
		if c.Data != nil {
			rec.Data = []byte(*c.Data)
		}
		out = append(out, rec)
	}
	return out, nil
}

// PruneTraceLog deletes records stored (created_at, the platform's clock)
// before before, chunk rows per statement and at most maxChunks statements,
// and returns how many it removed. An empty functionID prunes every function
// (the sweep); a function id prunes that function's records (after an enable,
// and while it has a live window). A bounded loop: a backlog larger than
// chunk*maxChunks is finished by the next sweep. The runner-supplied `at` is
// never used here: a runner clock far off must not keep records for ever or
// delete them early.
func (r *Repository) PruneTraceLog(ctx context.Context, functionID string, before time.Time, chunk, maxChunks int) (int64, error) {
	if chunk <= 0 {
		chunk = TracePruneChunk
	}
	if maxChunks <= 0 {
		maxChunks = DefaultTraceLimits().PruneChunks()
	}
	var total int64
	for range maxChunks {
		var tag int64
		var err error
		if functionID == "" {
			ct, e := r.pool.Exec(ctx, `
				DELETE FROM fng_trace_log WHERE id IN (
				    SELECT id FROM fng_trace_log WHERE created_at < $1 LIMIT $2)`, before, chunk)
			tag, err = ct.RowsAffected(), e
		} else {
			ct, e := r.pool.Exec(ctx, `
				DELETE FROM fng_trace_log WHERE id IN (
				    SELECT id FROM fng_trace_log WHERE function_id = $1 AND created_at < $2 LIMIT $3)`,
				functionID, before, chunk)
			tag, err = ct.RowsAffected(), e
		}
		if err != nil {
			return total, err
		}
		total += tag
		if tag < int64(chunk) {
			break
		}
	}
	return total, nil
}

// CountTraceLog counts a function's records (tests and diagnostics).
func (r *Repository) CountTraceLog(ctx context.Context, functionID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM fng_trace_log WHERE function_id = $1`, functionID).Scan(&n)
	return n, err
}
