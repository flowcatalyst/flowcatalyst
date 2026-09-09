package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// Repository owns the msg_events (write) + msg_events_read (read)
// tables. Writes happen via the UoW sink (platformsink.Sink); this
// repository exposes batch-ingest (from consumer apps via SDK outbox)
// and the read API.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// InsertBatch writes a batch of events to msg_events. Used by the
// POST /api/events/batch endpoint that consumer apps' outbox processors
// send to. Idempotent via deduplication_id: an event whose deduplication_id
// is already stored, or repeats one earlier in the same batch, is dropped
// and not counted.
//
// The unique index on msg_events is (deduplication_id, created_at) because
// the table is partitioned by created_at and Postgres requires the partition
// key in every unique index. ON CONFLICT therefore only catches a repeat
// that carries the identical created_at — which a retry of the same batch
// never does, since created_at is stamped per request. So the real dedup
// is the lookup below; ON CONFLICT DO NOTHING stays as the last line of
// defence. There is a window between the lookup and the insert in which two
// concurrent requests with the same deduplication_id both land; the outbox
// retry that dedup exists for is sequential, so that window is accepted.
func (r *Repository) InsertBatch(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	events, err := r.dropDuplicates(ctx, events)
	if err != nil {
		return 0, err
	}
	if len(events) == 0 {
		return 0, nil
	}
	batch := &pgx.Batch{}
	for _, e := range events {
		ctxJSON, err := json.Marshal(e.Context)
		if err != nil {
			return 0, fmt.Errorf("marshal context: %w", err)
		}
		t := e.Time
		if t.IsZero() {
			t = e.CreatedAt
		}
		// Column set matches the corrected platformsink.Sink shape.
		// Bare ON CONFLICT DO NOTHING (no conflict target, so no index
		// inference needed across migration profiles): a dedup collision
		// drops just that row instead of aborting the whole pipelined
		// batch, and retried batches are cheap no-ops.
		batch.Queue(
			`INSERT INTO msg_events
			     (id, spec_version, type, source, subject, time, data,
			      correlation_id, causation_id, deduplication_id, message_group,
			      client_id, context_data, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13::jsonb, $14)
			 ON CONFLICT DO NOTHING`,
			e.ID, e.SpecVersion, e.Type, e.Source, e.Subject,
			t, rawJSON(e.Data),
			e.CorrelationID, e.CausationID, e.DeduplicationID, e.MessageGroup,
			e.ClientID, ctxJSON, e.CreatedAt)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	inserted := 0
	for range events {
		tag, err := br.Exec()
		if err != nil {
			return inserted, err
		}
		if tag.RowsAffected() > 0 {
			inserted++
		}
	}
	return inserted, nil
}

// dropDuplicates removes events whose deduplication_id is already stored
// or repeats one earlier in the batch (first occurrence wins). Events with
// no deduplication_id are always kept.
func (r *Repository) dropDuplicates(ctx context.Context, events []Event) ([]Event, error) {
	var ids []string
	for _, e := range events {
		if e.DeduplicationID != "" {
			ids = append(ids, e.DeduplicationID)
		}
	}
	if len(ids) == 0 {
		return events, nil
	}
	seen := make(map[string]struct{}, len(ids))
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT deduplication_id FROM msg_events WHERE deduplication_id = ANY($1)`, ids)
	if err != nil {
		return nil, fmt.Errorf("dedup lookup: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("dedup lookup scan: %w", err)
		}
		seen[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dedup lookup: %w", err)
	}
	kept := events[:0:0]
	for _, e := range events {
		if e.DeduplicationID != "" {
			if _, dup := seen[e.DeduplicationID]; dup {
				continue
			}
			seen[e.DeduplicationID] = struct{}{}
		}
		kept = append(kept, e)
	}
	return kept, nil
}

// FindByID loads an event from the read table. `context` isn't denormalised
// into msg_events_read (only msg_events carries it) — Context comes back
// as an empty slice. Use the dedicated raw endpoint if you need it.
func (r *Repository) FindByID(ctx context.Context, id string) (*Event, error) {
	return r.fetchOne(ctx,
		`SELECT id, spec_version, type, source, subject, time, data,
		        deduplication_id, client_id, message_group, correlation_id,
		        causation_id, created_at, application, subdomain, aggregate,
		        projected_at
		   FROM msg_events_read WHERE id = $1`, id)
}

// FilterParams is the query DTO for list endpoints. `PrincipalID` is
// not yet wired (no backing column on msg_events_read).
//
// The plural slice fields back the SPA's CSV multi-filters
// (clientIds/applications/subdomains/aggregates/types). When set they
// build `col = ANY($n)` conditions; the singular fields stay for the SDK
// callers and build `col = $n`.
type FilterParams struct {
	Type          *string
	Source        *string
	Subject       *string
	ClientID      *string
	PrincipalID   *string
	CorrelationID *string
	Since         *time.Time
	Until         *time.Time
	Limit         int
	Offset        int

	// CSV multi-filters from the SPA.
	ClientIDs    []string
	Applications []string
	Subdomains   []string
	Aggregates   []string
	Types        []string

	// AccessibleClientIDs: a non-nil pointer scopes results to
	// platform-scoped events (client_id IS NULL) plus events whose
	// client_id is in the set; nil means no access scoping (anchor).
	// Mirrors scheduledjob.FilterParams — enforced in SQL so the caller's
	// clientId/clientIds filters can only ever narrow within the
	// principal's tenants, never reach across them.
	AccessibleClientIDs *[]string
}

// FindWithFilters returns events from the read table matching non-nil
// filters, ordered most-recent first.
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]Event, error) {
	var f repocommon.Filter
	f.EqPtr("type", p.Type)
	f.Any("type", p.Types)
	f.EqPtr("source", p.Source)
	f.EqPtr("subject", p.Subject)
	f.EqPtr("client_id", p.ClientID)
	f.Any("client_id", p.ClientIDs)
	if p.AccessibleClientIDs != nil {
		f.Clause("(client_id IS NULL OR client_id = ANY($%d))", *p.AccessibleClientIDs)
	}
	f.Any("application", p.Applications)
	f.Any("subdomain", p.Subdomains)
	f.Any("aggregate", p.Aggregates)
	// PrincipalID filter dropped — no backing column on msg_events_read.
	f.EqPtr("correlation_id", p.CorrelationID)
	if p.Since != nil {
		f.Clause("created_at >= $%d", *p.Since)
	}
	if p.Until != nil {
		f.Clause("created_at <= $%d", *p.Until)
	}
	q := `SELECT id, spec_version, type, source, subject, time, data,
		     deduplication_id, client_id, message_group, correlation_id,
		     causation_id, created_at, application, subdomain, aggregate,
		     projected_at
		  FROM msg_events_read` + f.Where() + " ORDER BY created_at DESC"
	limit := p.Limit
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	q += fmt.Sprintf(" LIMIT $%d", f.Arg(limit))
	if p.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", f.Arg(p.Offset))
	}
	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// FindRecentRaw returns the most-recent `limit` rows from the write-side
// msg_events table, including context_data. Powers the debug raw-event
// view (GET /bff/debug/events) which needs the un-projected envelope —
// the read table drops context_data. Ordered most-recent first.
func (r *Repository) FindRecentRaw(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx,
		`SELECT id, spec_version, type, source, subject, time, data,
		        deduplication_id, client_id, message_group, correlation_id,
		        causation_id, context_data, created_at
		   FROM msg_events
		  ORDER BY created_at DESC
		  LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		e, err := scanRawRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

// FindRawByID loads one write-side msg_events row, including
// context_data — the detail counterpart of FindRecentRaw (GET
// /bff/debug/events/{id}). Unlike FindByID (the read-table lookup, which
// drops context_data) this is what the debug detail route needs so its
// response is the exact same shape as one row of the debug list.
//
// Deliberately NOT client-scoped, matching FindRecentRaw / listDebugRaw:
// the list this mirrors already hands every raw row to any holder of
// event:view-raw, so scoping only the detail would be inconsistent without
// adding any real protection. Returns (nil, nil) when no such row exists.
func (r *Repository) FindRawByID(ctx context.Context, id string) (*Event, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, spec_version, type, source, subject, time, data,
		        deduplication_id, client_id, message_group, correlation_id,
		        causation_id, context_data, created_at
		   FROM msg_events WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("event repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	e, err := scanRawRow(rows)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return e, rows.Err()
}

// scanRawRow scans a write-side msg_events row, including context_data.
func scanRawRow(rows pgx.Rows) (*Event, error) {
	var e Event
	var dataBytes, ctxBytes []byte
	var subject, dedupID *string
	if err := rows.Scan(&e.ID, &e.SpecVersion, &e.Type, &e.Source, &subject,
		&e.Time, &dataBytes, &dedupID, &e.ClientID, &e.MessageGroup,
		&e.CorrelationID, &e.CausationID, &ctxBytes, &e.CreatedAt); err != nil {
		return nil, err
	}
	if subject != nil {
		e.Subject = *subject
	}
	if dedupID != nil {
		e.DeduplicationID = *dedupID
	}
	if len(dataBytes) > 0 {
		e.Data = json.RawMessage(dataBytes)
	}
	e.Context = []ContextEntry{}
	if len(ctxBytes) > 0 {
		_ = json.Unmarshal(ctxBytes, &e.Context)
		if e.Context == nil {
			e.Context = []ContextEntry{}
		}
	}
	return &e, nil
}

// DistinctValues lists up to `limit` distinct non-null values for the
// given column. Used to populate the frontend's filter-options dropdowns
// (event types, sources, client IDs). The column name is hardcoded by
// the caller — no untrusted SQL.
func (r *Repository) DistinctValues(ctx context.Context, column string, limit int) ([]string, error) {
	allowed := map[string]bool{
		"type": true, "source": true, "subject": true,
		"client_id": true, "correlation_id": true,
		"application": true, "subdomain": true, "aggregate": true,
		// principal_id has no backing column on msg_events_read.
	}
	if !allowed[column] {
		return nil, fmt.Errorf("event repo: column %q not allowed for DistinctValues", column)
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT DISTINCT %s FROM msg_events_read
		              WHERE %s IS NOT NULL ORDER BY 1 LIMIT $1`, column, column),
		limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *Repository) fetchOne(ctx context.Context, sql string, args ...any) (*Event, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("event repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, nil
	}
	e, err := scanRow(rows)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return e, rows.Err()
}

func scanRow(rows pgx.Rows) (*Event, error) {
	var e Event
	var dataBytes []byte
	// spec_version is nullable in the schema (like subject/dedup): scan via
	// a pointer so one NULL row can't 500 the whole list query.
	var specVersion, subject, dedupID *string
	if err := rows.Scan(&e.ID, &specVersion, &e.Type, &e.Source, &subject,
		&e.Time, &dataBytes, &dedupID, &e.ClientID, &e.MessageGroup,
		&e.CorrelationID, &e.CausationID, &e.CreatedAt,
		&e.Application, &e.Subdomain, &e.Aggregate, &e.ProjectedAt); err != nil {
		return nil, err
	}
	if specVersion != nil {
		e.SpecVersion = *specVersion
	}
	if subject != nil {
		e.Subject = *subject
	}
	if dedupID != nil {
		e.DeduplicationID = *dedupID
	}
	if len(dataBytes) > 0 {
		e.Data = json.RawMessage(dataBytes)
	}
	e.Context = []ContextEntry{}
	return &e, nil
}

func rawJSON(rm json.RawMessage) any {
	if len(rm) == 0 {
		return []byte("{}")
	}
	return []byte(rm)
}
