package ingestion

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// This file is the ing_trace_windows / ing_trace_log part of the repository:
// hand-rolled pgx SQL, not sqlc, for the same reason the function trace's is
// (a small self-contained surface with a dynamic filter).

const traceWindowColumns = `ingestion_id, window_id, window_secs, sample_every_secs, hard_stop, enabled_by, enabled_at, updated_at`

type traceWindowRow struct {
	IngestionID     string    `db:"ingestion_id"`
	WindowID        string    `db:"window_id"`
	WindowSecs      int32     `db:"window_secs"`
	SampleEverySecs int32     `db:"sample_every_secs"`
	HardStop        time.Time `db:"hard_stop"`
	EnabledBy       *string   `db:"enabled_by"`
	EnabledAt       time.Time `db:"enabled_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

func (w traceWindowRow) window() TraceWindow {
	return TraceWindow{
		IngestionID: w.IngestionID, WindowID: w.WindowID, WindowSecs: int(w.WindowSecs), SampleEverySecs: int(w.SampleEverySecs),
		HardStop: w.HardStop, EnabledBy: w.EnabledBy, EnabledAt: w.EnabledAt, UpdatedAt: w.UpdatedAt,
	}
}

// GetTraceWindow loads an ingestion's window row; nil when it was never
// switched on.
func (r *Repository) GetTraceWindow(ctx context.Context, ingestionID string) (*TraceWindow, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+traceWindowColumns+` FROM ing_trace_windows WHERE ingestion_id = $1`, ingestionID)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: get trace window: %w", err)
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[traceWindowRow])
	rr, err := repocommon.One(row, err, "ingestion repo")
	if rr == nil || err != nil {
		return nil, err
	}
	w := rr.window()
	return &w, nil
}

// ListTraceWindows returns the window rows of the given ingestions (the ingest
// endpoint's accept check, one query per batch).
func (r *Repository) ListTraceWindows(ctx context.Context, ingestionIDs []string) ([]TraceWindow, error) {
	return r.queryTraceWindows(ctx, `ingestion_id = ANY($1)`, ingestionIDs)
}

// ListLiveTraceWindows returns, of the given ingestions, the windows open at
// now: what a poll answer lists as traces.
func (r *Repository) ListLiveTraceWindows(ctx context.Context, ingestionIDs []string, now time.Time) ([]TraceWindow, error) {
	return r.queryTraceWindows(ctx, `ingestion_id = ANY($1) AND hard_stop > $2`, ingestionIDs, now)
}

func (r *Repository) queryTraceWindows(ctx context.Context, where string, args ...any) ([]TraceWindow, error) {
	if ids, ok := args[0].([]string); ok && len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, `SELECT `+traceWindowColumns+` FROM ing_trace_windows WHERE `+where+` ORDER BY ingestion_id`, args...)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list trace windows: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[traceWindowRow])
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: list trace windows: %w", err)
	}
	out := make([]TraceWindow, 0, len(collected))
	for _, row := range collected {
		out = append(out, row.window())
	}
	return out, nil
}

// UpsertTraceWindowTx opens a window, replacing the ingestion's earlier one
// (a new window id, so runtimes start a fresh capture).
func (r *Repository) UpsertTraceWindowTx(ctx context.Context, w *TraceWindow, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO ing_trace_windows (`+traceWindowColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (ingestion_id) DO UPDATE
		   SET window_id         = EXCLUDED.window_id,
		       window_secs       = EXCLUDED.window_secs,
		       sample_every_secs = EXCLUDED.sample_every_secs,
		       hard_stop         = EXCLUDED.hard_stop,
		       enabled_by        = EXCLUDED.enabled_by,
		       enabled_at        = EXCLUDED.enabled_at,
		       updated_at        = EXCLUDED.updated_at`,
		w.IngestionID, w.WindowID, w.WindowSecs, w.SampleEverySecs, w.HardStop, w.EnabledBy, w.EnabledAt, w.UpdatedAt)
	return err
}

// EndTraceWindowTx closes a live window at at: hard_stop becomes at (not
// removed), so records captured just before the switch-off and still queued
// in a runtime are accepted within the ingest grace. It returns the window as
// it is after, nil when the ingestion was never traced, and whether it was
// live before, which is what makes switching off idempotent (and the event
// only for a window that really ended).
func (r *Repository) EndTraceWindowTx(ctx context.Context, ingestionID string, at time.Time, tx pgx.Tx) (w *TraceWindow, wasLive bool, err error) {
	rows, err := tx.Query(ctx, `
		WITH old AS (
		    SELECT ingestion_id, hard_stop > $2 AS was_live FROM ing_trace_windows WHERE ingestion_id = $1 FOR UPDATE
		), upd AS (
		    UPDATE ing_trace_windows t
		       SET hard_stop = CASE WHEN old.was_live THEN $2 ELSE t.hard_stop END,
		           updated_at = CASE WHEN old.was_live THEN $2 ELSE t.updated_at END
		      FROM old WHERE t.ingestion_id = old.ingestion_id
		    RETURNING t.ingestion_id, t.window_id, t.window_secs, t.sample_every_secs, t.hard_stop, t.enabled_by, t.enabled_at, t.updated_at
		)
		SELECT upd.*, old.was_live FROM upd JOIN old USING (ingestion_id)`, ingestionID, at)
	if err != nil {
		return nil, false, err
	}
	type endedRow struct {
		traceWindowRow
		WasLive bool `db:"was_live"`
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[endedRow])
	rr, err := repocommon.One(row, err, "ingestion repo")
	if rr == nil || err != nil {
		return nil, false, err
	}
	out := rr.window()
	return &out, rr.WasLive, nil
}

// InsertTraceRecords appends records in one statement. The caller has already
// applied every limit and assigned the ids; a record whose id is already
// stored is skipped, so a batch the runtime retries is not stored twice.
func (r *Repository) InsertTraceRecords(ctx context.Context, recs []TraceLogRecord) error {
	if len(recs) == 0 {
		return nil
	}
	n := len(recs)
	ids, ings, wins, caps, rts, kinds, levels, msgs := make([]string, n), make([]string, n), make([]string, n), make([]string, n),
		make([]string, n), make([]string, n), make([]string, n), make([]string, n)
	versions := make([]int32, n)
	data := make([]*string, n)
	ats, created := make([]time.Time, n), make([]time.Time, n)
	for i, rec := range recs {
		ids[i], ings[i], wins[i], caps[i], rts[i] = rec.ID, rec.IngestionID, rec.WindowID, rec.CaptureID, rec.RuntimeID
		kinds[i], levels[i], msgs[i], versions[i] = rec.Kind, rec.Level, rec.Message, rec.Version
		ats[i], created[i] = rec.At, rec.CreatedAt
		if rec.Data != nil {
			s := string(rec.Data)
			data[i] = &s
		}
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO ing_trace_log (id, ingestion_id, version, window_id, capture_id, runtime_id,
		                           kind, level, message, data, at, created_at)
		SELECT t.id, t.ingestion_id, t.version, t.window_id, t.capture_id, t.runtime_id,
		       t.kind, t.level, t.message, t.data::jsonb, t.at, t.created_at
		  FROM unnest($1::text[], $2::text[], $3::int[], $4::text[], $5::text[], $6::text[],
		              $7::text[], $8::text[], $9::text[], $10::text[], $11::timestamptz[], $12::timestamptz[])
		       AS t(id, ingestion_id, version, window_id, capture_id, runtime_id,
		            kind, level, message, data, at, created_at)
		ON CONFLICT (id) DO NOTHING`,
		ids, ings, versions, wins, caps, rts, kinds, levels, msgs, data, ats, created)
	if err != nil {
		return fmt.Errorf("ingestion repo: insert trace records: %w", err)
	}
	return nil
}

// QueryTraceLog returns an ingestion's records newest first, keyset-paged on
// (at, id). It returns at most f.Limit rows; ask for one more than the page to
// learn whether another page exists.
func (r *Repository) QueryTraceLog(ctx context.Context, f TraceLogFilter) ([]TraceLogRecord, error) {
	var flt repocommon.Filter
	flt.Eq("ingestion_id", f.IngestionID)
	if f.Kind != "" {
		flt.Eq("kind", f.Kind)
	}
	if f.RuntimeID != "" {
		flt.Eq("runtime_id", f.RuntimeID)
	}
	if f.CaptureID != "" {
		flt.Eq("capture_id", f.CaptureID)
	}
	if f.Since != nil {
		flt.Clause("at >= $%d", *f.Since)
	}
	if f.Until != nil {
		flt.Clause("at <= $%d", *f.Until)
	}
	if f.After != nil {
		at := flt.Arg(f.After.At)
		flt.Cond(fmt.Sprintf("(at, id) < ($%d, $%d)", at, flt.Arg(f.After.ID)))
	}
	limit := f.Limit
	if limit <= 0 {
		limit = DefaultTraceLogLimit
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, ingestion_id, version, window_id, capture_id, runtime_id, kind, level,
		       message, data::text AS data, at, created_at
		  FROM ing_trace_log`+flt.Where()+fmt.Sprintf(`
		 ORDER BY at DESC, id DESC
		 LIMIT $%d`, flt.Arg(limit)), flt.Args()...)
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: query trace log: %w", err)
	}
	type row struct {
		ID          string    `db:"id"`
		IngestionID string    `db:"ingestion_id"`
		Version     int32     `db:"version"`
		WindowID    string    `db:"window_id"`
		CaptureID   string    `db:"capture_id"`
		RuntimeID   string    `db:"runtime_id"`
		Kind        string    `db:"kind"`
		Level       string    `db:"level"`
		Message     string    `db:"message"`
		Data        *string   `db:"data"`
		At          time.Time `db:"at"`
		CreatedAt   time.Time `db:"created_at"`
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[row])
	if err != nil {
		return nil, fmt.Errorf("ingestion repo: query trace log: %w", err)
	}
	out := make([]TraceLogRecord, 0, len(collected))
	for _, c := range collected {
		rec := TraceLogRecord{
			ID: c.ID, IngestionID: c.IngestionID, Version: c.Version, WindowID: c.WindowID, CaptureID: c.CaptureID,
			RuntimeID: c.RuntimeID, Kind: c.Kind, Level: c.Level, Message: c.Message, At: c.At, CreatedAt: c.CreatedAt,
		}
		if c.Data != nil {
			rec.Data = []byte(*c.Data)
		}
		out = append(out, rec)
	}
	return out, nil
}

// CountTraceLog counts an ingestion's records (tests and diagnostics).
func (r *Repository) CountTraceLog(ctx context.Context, ingestionID string) (int64, error) {
	var n int64
	err := r.pool.QueryRow(ctx, `SELECT count(*) FROM ing_trace_log WHERE ingestion_id = $1`, ingestionID).Scan(&n)
	return n, err
}
