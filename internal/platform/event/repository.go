package event

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// Repository owns the msg_events (write) + msg_events_read (read)
// tables. Writes happen via the UoW sink (platformsink.Sink); this
// repository exposes batch-ingest (from consumer apps via SDK outbox)
// and the read API.
type Repository struct {
	pool *pgxpool.Pool
	// payloads resolves the data of an event of the new shape whose payload
	// was offloaded, for the reads of one event. nil: such data is
	// unavailable (payloadstore.ErrStoreNotConfigured); every other event
	// reads as without it.
	payloads *payloadstore.Service
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

// UsePayloads gives the repository the payload service the reads of one event
// resolve offloaded data through (docs/payload-offload-design.md). Called once
// at startup, before the repository serves anything; returns r.
func (r *Repository) UsePayloads(s *payloadstore.Service) *Repository {
	r.payloads = s
	return r
}

// Payloads is the repository's payload service: what decides the largest
// payload accepted and the request caps derived from it
// (payloadstore.Service.MaxPayloadBytes). Nil for a nil repository or one
// given none, which the service's methods answer with the defaults.
func (r *Repository) Payloads() *payloadstore.Service {
	if r == nil {
		return nil
	}
	return r.payloads
}

// The shape of the event reads (docs/payload-offload-design.md, section 10,
// step 1). An event row of the legacy shape (payload_id NULL) has its data in
// its own data column; one of the new shape has it in the msg_payloads row its
// payload_id names, under the event's own created_at. Every read selects its
// rows first, in a subquery with its own filters, order and limit, and only
// then joins each row to its payload row by primary key (withPayload), so the
// join cannot change how the rows are found and costs one primary-key probe
// per row returned. A list never calls the payload store: data held in the
// database comes back as stored, offloaded data is left out (DataOffloaded).
// A read of one event resolves it (resolveData).
const (
	// readCols are msg_events_read's columns, in scanRow's order.
	readCols = `id, spec_version, type, source, subject, time, data,
		deduplication_id, client_id, message_group, correlation_id,
		causation_id, created_at, application, subdomain, aggregate,
		projected_at, payload_id`
	// rawCols are msg_events' columns, in scanRawRow's order.
	rawCols = `id, spec_version, type, source, subject, time, data,
		deduplication_id, client_id, message_group, correlation_id,
		causation_id, context_data, created_at, payload_id`
	// payloadJoin joins the rows of the subquery (alias e) to their payload
	// rows. LATERAL keeps it a probe per row whatever the planner thinks of
	// the tables' sizes; a legacy row skips it.
	payloadJoin = `
		  LEFT JOIN LATERAL (SELECT p.data, p.ref, p.size FROM msg_payloads p
		                      WHERE e.payload_id IS NOT NULL AND p.id = e.payload_id AND p.created_at = e.created_at) p ON TRUE`
)

// withPayload wraps inner, a SELECT of readCols or rawCols, so each of its rows
// is followed by its payload row's data, ref and size; order is the outer
// ORDER BY (the inner one's, on alias e), or "".
func withPayload(inner, order string) string {
	return `SELECT e.*, p.data, p.ref, p.size FROM (` + inner + `) e` + payloadJoin + order
}

// setData fills the event's data from what a read selected: the legacy data
// column, or the joined payload row of an event of the new shape (whose data
// column is not used). Data held in the payload row is the producer's bytes.
func (e *Event) setData(legacy []byte, payloadID, data, ref *string, size *int32) {
	if st := payloadstore.FromJoin(payloadID, data, ref, size); st != nil {
		e.StoredData = st
		e.DataSize = st.Size
		switch {
		case st.Data != nil:
			e.Data = json.RawMessage(*st.Data)
		case st.Offloaded():
			e.DataOffloaded = true
		}
		return
	}
	if len(legacy) > 0 {
		e.Data = json.RawMessage(legacy)
		e.DataSize = len(legacy)
	}
}

// resolveData produces the data of one event of the new shape whose payload
// row did not hold it (offloaded, or the row is gone): read from the payload
// store through its cache. When that fails the event keeps no data and is
// marked DataUnavailable; the failure is logged at error, never the data.
// Nothing is called for any other event.
func (r *Repository) resolveData(ctx context.Context, e *Event) {
	if e == nil || e.StoredData == nil || e.StoredData.Data != nil {
		return
	}
	b, err := payloadstore.ResolveWith(ctx, r.payloads, *e.StoredData)
	e.DataOffloaded = false
	if err != nil {
		e.DataUnavailable = true
		slog.ErrorContext(ctx, "event data unavailable", "event_id", e.ID, "payload_id", e.StoredData.ID, "err", err)
		return
	}
	e.Data = json.RawMessage(b)
}

// InsertBatch writes a batch of events to msg_events. Used by the
// POST /api/events/batch endpoint that consumer apps' outbox processors
// send to. Idempotent via deduplication_id: an event whose deduplication_id
// is already stored, or repeats one earlier in the same batch, is dropped
// and not counted.
//
// It is Ingest for a caller that does not report per event: the events that
// can be stored are, and the first event refused (see Ingest) is returned as
// the error, wrapping the payloadstore sentinel that says why. A caller of
// one event maps that error; a caller of several that must answer per event
// uses Ingest.
func (r *Repository) InsertBatch(ctx context.Context, events []Event) (int, error) {
	res, err := r.Ingest(ctx, events)
	if err != nil {
		return res.Inserted, err
	}
	for _, refused := range res.Refused {
		if refused != nil {
			return res.Inserted, refused
		}
	}
	return res.Inserted, nil
}

// IngestResult is what Ingest did with a batch.
type IngestResult struct {
	// Inserted is the number of events stored (a dropped duplicate is not
	// counted).
	Inserted int
	// Refused is, in the order of the events given, why an event was not
	// stored: nil for one that was stored or dropped as a duplicate.
	// Only the payload table's writes refuse an event
	// (FC_PAYLOAD_TABLE_WRITES): an error wrapping payloadstore.ErrTooLarge
	// or payloadstore.ErrNotText (permanent) or
	// payloadstore.ErrStoreUnavailable (retryable).
	Refused []error
}

// Ingest writes a batch of events to msg_events, reporting per event. See
// InsertBatch for the deduplication.
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
//
// The shape written (docs/payload-offload-design.md, section 10) follows the
// payload service's switch (payloadstore.Service.TableWrites):
//
//   - off (or no payload service): the data in msg_events.data, as JSONB,
//     exactly as before.
//   - on: the data of each event that survives the duplicate check is
//     prepared by the payload service (payloadstore.Service.Prepare: the
//     bytes as received, offloaded above the threshold, the store written
//     before anything is inserted and never inside a transaction), and the
//     event row is written with payload_id and a NULL data in the same
//     statement as its payload row, which is inserted only when the event
//     row was (so a dropped conflict leaves no payload row). An event the
//     payload service refuses is reported in Refused and the rest are
//     stored. An event with no data is given the data "{}", as the legacy
//     shape stores it, so what its subscribers receive does not change.
func (r *Repository) Ingest(ctx context.Context, events []Event) (IngestResult, error) {
	p, err := r.PrepareIngest(ctx, events)
	if err != nil {
		return IngestResult{Refused: make([]error, len(events))}, err
	}
	return p.Insert(ctx, r.pool)
}

// PreparedIngest is a batch of events PrepareIngest has checked and prepared:
// the statements that insert them, not yet sent. Insert sends them, once.
type PreparedIngest struct {
	res   IngestResult
	batch *pgx.Batch
	// newShape says, per queued statement, whether it is the payload
	// table's (it answers a row count).
	newShape []bool
}

// BatchSender is what Insert sends the statements through: the pool, or a
// transaction of the caller's.
type BatchSender interface {
	SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults
}

// PrepareIngest is Ingest up to the insert: the duplicate lookup, the
// payloads' preparation (the payload store written, outside any transaction)
// and the statements. Insert then stores the events, in a transaction of the
// caller's when it needs them stored with something it checks there (the
// ingestion runtime API holds the ingestion's lease row while it inserts).
func (r *Repository) PrepareIngest(ctx context.Context, events []Event) (*PreparedIngest, error) {
	p := &PreparedIngest{res: IngestResult{Refused: make([]error, len(events))}, batch: &pgx.Batch{}}
	if len(events) == 0 {
		return p, nil
	}
	kept, err := r.dropDuplicates(ctx, events)
	if err != nil {
		return nil, err
	}
	if len(kept) == 0 {
		return p, nil
	}
	var prepared []payloadstore.Prepared
	if r.payloads.TableWrites() {
		items := make([]payloadstore.Item, len(kept))
		for k, i := range kept {
			e := &events[i]
			items[k] = payloadstore.Item{ClientID: deref(e.ClientID), CreatedAt: e.CreatedAt, Data: []byte(eventData(e.Data))}
		}
		// The store is written here, before the insert and outside any
		// transaction.
		prepared = r.payloads.Prepare(ctx, items)
	}
	for k, i := range kept {
		e := &events[i]
		ctxJSON, err := json.Marshal(e.Context)
		if err != nil {
			return nil, fmt.Errorf("marshal context: %w", err)
		}
		t := e.Time
		if t.IsZero() {
			t = e.CreatedAt
		}
		if prepared != nil {
			pp := prepared[k]
			if pp.Err != nil {
				p.res.Refused[i] = fmt.Errorf("event %s: %w", e.ID, pp.Err)
				continue
			}
			args := append([]any{
				e.ID, e.SpecVersion, e.Type, e.Source, e.Subject, t,
				e.CorrelationID, e.CausationID, e.DeduplicationID, e.MessageGroup,
				e.ClientID, ctxJSON, e.CreatedAt,
			}, pp.Row.InsertArgs()...)
			p.batch.Queue(insertEventWithPayloadSQL, args...)
			p.newShape = append(p.newShape, true)
			continue
		}
		// Column set matches the corrected platformsink.Sink shape.
		// Bare ON CONFLICT DO NOTHING (no conflict target, so no index
		// inference needed across migration profiles): a dedup collision
		// drops just that row instead of aborting the whole pipelined
		// batch, and retried batches are cheap no-ops.
		p.batch.Queue(
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
		p.newShape = append(p.newShape, false)
	}
	return p, nil
}

// Insert sends the prepared statements through db (the pool, or the caller's
// transaction) and answers what Ingest answers: the events stored, and those
// the payload service refused.
func (p *PreparedIngest) Insert(ctx context.Context, db BatchSender) (IngestResult, error) {
	res := p.res
	if len(p.newShape) == 0 {
		return res, nil
	}
	br := db.SendBatch(ctx, p.batch)
	defer br.Close()
	for _, newShape := range p.newShape {
		if newShape {
			var n int
			if err := br.QueryRow().Scan(&n); err != nil {
				return res, err
			}
			res.Inserted += n
			continue
		}
		tag, err := br.Exec()
		if err != nil {
			return res, err
		}
		if tag.RowsAffected() > 0 {
			res.Inserted++
		}
	}
	return res, nil
}

// insertEventWithPayloadSQL inserts an event of the new shape and its payload
// row in one statement: the event row with payload_id and no data, and the
// payload row only when the event row was inserted (the bare ON CONFLICT DO
// NOTHING may drop it), so a dropped event leaves no payload row behind. It
// answers the number of event rows inserted (0 or 1). $1 to $13 are the
// event's columns, $14 to $18 payloadstore.Row.InsertArgs (id, created_at,
// data, ref, size), prepared under the event's created_at ($13 and $15 are
// the same instant).
const insertEventWithPayloadSQL = `WITH e AS (
	INSERT INTO msg_events
	     (id, spec_version, type, source, subject, time,
	      correlation_id, causation_id, deduplication_id, message_group,
	      client_id, context_data, created_at, payload_id)
	 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, $13, $14)
	 ON CONFLICT DO NOTHING
	 RETURNING 1
), p AS (
	INSERT INTO msg_payloads (id, created_at, data, ref, size)
	SELECT $14::varchar, $15::timestamptz, $16::text, $17::text, $18::integer FROM e
)
SELECT count(*)::int FROM e`

// eventData is the data an event is stored with: its own, or "{}" when it has
// none (as rawJSON stores the legacy shape).
func eventData(rm json.RawMessage) []byte {
	if len(rm) == 0 {
		return []byte("{}")
	}
	return rm
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// dropDuplicates returns the positions of the events to insert: all but
// those whose deduplication_id is already stored or repeats one earlier in
// the batch (first occurrence wins). Events with no deduplication_id are
// always kept.
func (r *Repository) dropDuplicates(ctx context.Context, events []Event) ([]int, error) {
	all := make([]int, len(events))
	for i := range events {
		all[i] = i
	}
	var ids []string
	for _, e := range events {
		if e.DeduplicationID != "" {
			ids = append(ids, e.DeduplicationID)
		}
	}
	if len(ids) == 0 {
		return all, nil
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
	kept := make([]int, 0, len(events))
	for i, e := range events {
		if e.DeduplicationID != "" {
			if _, dup := seen[e.DeduplicationID]; dup {
				continue
			}
			seen[e.DeduplicationID] = struct{}{}
		}
		kept = append(kept, i)
	}
	return kept, nil
}

// FindByID loads an event from the read table. `context` isn't denormalised
// into msg_events_read (only msg_events carries it) — Context comes back
// as an empty slice. Use the dedicated raw endpoint if you need it.
//
// A read of one event: offloaded data is resolved from the payload store
// (DataUnavailable when that fails; the event is still returned).
func (r *Repository) FindByID(ctx context.Context, id string) (*Event, error) {
	e, err := r.fetchOne(ctx, withPayload(`SELECT `+readCols+` FROM msg_events_read WHERE id = $1`, ""), id)
	if err != nil {
		return nil, err
	}
	r.resolveData(ctx, e)
	return e, nil
}

// Tenant scoping for a caller confined to a set of clients (non-anchor).
// Visible: an event of one of the caller's clients, or a client-less event of
// an ordinary application. NOT visible: a client-less event of the platform
// itself — the control-plane events the platform writes about its own
// operation (user created, role changed, ...), which carry no client and are
// about every tenant. They are told apart by their type: the platform's own
// application code is "platform", the first segment of the type, which the
// projection stores in msg_events_read.application and msg_events carries only
// in type. Both predicates ride on the rows the client/created_at conditions
// already select; neither needs a new column or index.
const (
	// readScopeClause scopes msg_events_read; one placeholder: the client ids.
	readScopeClause = "(client_id = ANY($%d) OR (client_id IS NULL AND application IS DISTINCT FROM 'platform'))"
	// rawScopeClause scopes msg_events (no application column).
	rawScopeClause = "(client_id = ANY($%d) OR (client_id IS NULL AND split_part(type, ':', 1) <> 'platform'))"
)

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

	// AccessibleClientIDs: a non-nil pointer scopes results to events whose
	// client_id is in the set plus client-less events of ordinary
	// applications (the platform's own client-less control-plane events are
	// excluded); nil means no access scoping (anchor).
	// Mirrors scheduledjob.FilterParams — enforced in SQL so the caller's
	// clientId/clientIds filters can only ever narrow within the
	// principal's tenants, never reach across them.
	AccessibleClientIDs *[]string

	// AccessibleApplications: a non-nil pointer scopes results to events whose
	// application (the first segment of the type) is one of the codes; nil
	// means no application scoping. Set for a caller confined to its own
	// applications (F20). Applied in SQL, so no caller filter can widen it.
	AccessibleApplications *[]string

	// Cursor keeps only events after it in the list's order (created_at,
	// then id, newest first) — the keyset cursor of a paged list (the /ui
	// events page's "Older").
	Cursor *ListCursor
}

// ListCursor is a position in the list's (created_at, id) order: the last
// row of the page before.
type ListCursor struct {
	CreatedAt time.Time
	ID        string
}

// FindWithFilters returns events from the read table matching non-nil
// filters, ordered most-recent first. Data held in the database is returned;
// offloaded data is left out (DataOffloaded) and the payload store is never
// called.
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]Event, error) {
	q, args := ListSQL(p)
	rows, err := r.pool.Query(ctx, q, args...)
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

// ListSQL renders FindWithFilters' statement for p: the page of
// msg_events_read (filters, order, limit) in a subquery, then each of its rows
// joined to its payload row (withPayload). Exported for plan tests.
func ListSQL(p FilterParams) (string, []any) {
	var f repocommon.Filter
	f.EqPtr("type", p.Type)
	f.Any("type", p.Types)
	f.EqPtr("source", p.Source)
	f.EqPtr("subject", p.Subject)
	f.EqPtr("client_id", p.ClientID)
	f.Any("client_id", p.ClientIDs)
	if p.AccessibleClientIDs != nil {
		f.Clause(readScopeClause, *p.AccessibleClientIDs)
	}
	if p.AccessibleApplications != nil {
		f.Clause("application = ANY($%d)", *p.AccessibleApplications)
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
	if p.Cursor != nil {
		// The plain range first, so the created_at index and partition
		// pruning apply; the OR breaks ties at the same instant by id.
		at, id := f.Arg(p.Cursor.CreatedAt), f.Arg(p.Cursor.ID)
		f.Cond(fmt.Sprintf("created_at <= $%d AND (created_at < $%d OR id < $%d)", at, at, id))
	}
	// id breaks ties at the same instant, so the order is total and a
	// keyset cursor neither skips nor repeats a row.
	q := `SELECT ` + readCols + ` FROM msg_events_read` + f.Where() + " ORDER BY created_at DESC, id DESC"
	// A size above the cap is clamped to it; no size is the default 100.
	limit := min(p.Limit, 1000)
	if limit <= 0 {
		limit = 100
	}
	q += fmt.Sprintf(" LIMIT $%d", f.Arg(limit))
	if p.Offset > 0 {
		q += fmt.Sprintf(" OFFSET $%d", f.Arg(p.Offset))
	}
	// The page first, then its payload rows; the store is never called.
	return withPayload(q, " ORDER BY e.created_at DESC, e.id DESC"), f.Args()
}

// FindRecentRaw returns the most-recent `limit` rows from the write-side
// msg_events table, including context_data. Powers the debug raw-event
// view (GET /bff/debug/events) which needs the un-projected envelope —
// the read table drops context_data. Ordered most-recent first. Offloaded
// data is left out (DataOffloaded); the payload store is never called.
//
// accessibleClientIDs scopes the rows exactly as FilterParams.AccessibleClientIDs
// scopes the list (nil: every row, for an anchor).
func (r *Repository) FindRecentRaw(ctx context.Context, limit int, accessibleClientIDs *[]string) ([]Event, error) {
	limit = min(limit, 1000)
	if limit <= 0 {
		limit = 100
	}
	var f repocommon.Filter
	if accessibleClientIDs != nil {
		f.Clause(rawScopeClause, *accessibleClientIDs)
	}
	rows, err := r.pool.Query(ctx,
		withPayload(`SELECT `+rawCols+` FROM msg_events`+f.Where()+
			fmt.Sprintf(` ORDER BY created_at DESC LIMIT $%d`, f.Arg(limit)), ` ORDER BY e.created_at DESC`),
		f.Args()...)
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
// Unscoped: for callers that are not serving a tenant read (the function
// control plane reading back its own emit). A read on behalf of a caller uses
// FindRawByIDScoped.
// Returns (nil, nil) when no such row exists.
func (r *Repository) FindRawByID(ctx context.Context, id string) (*Event, error) {
	return r.FindRawByIDScoped(ctx, id, nil)
}

// FindRawByIDScoped is FindRawByID confined as FindRecentRaw is: with a non-nil
// accessibleClientIDs a row outside the scope is (nil, nil), the same answer as
// a missing id. A read of one event: offloaded data is resolved from the
// payload store (DataUnavailable when that fails; the event is still
// returned).
func (r *Repository) FindRawByIDScoped(ctx context.Context, id string, accessibleClientIDs *[]string) (*Event, error) {
	f := repocommon.Filter{}
	f.Eq("id", id)
	if accessibleClientIDs != nil {
		f.Clause(rawScopeClause, *accessibleClientIDs)
	}
	e, err := r.fetchOneRaw(ctx, withPayload(`SELECT `+rawCols+` FROM msg_events`+f.Where(), ""), f.Args()...)
	if err != nil {
		return nil, err
	}
	r.resolveData(ctx, e)
	return e, nil
}

// fetchOneRaw reads the first msg_events row sql selects (rawCols, joined by
// withPayload); (nil, nil) when there is none.
func (r *Repository) fetchOneRaw(ctx context.Context, sql string, args ...any) (*Event, error) {
	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("event repo: %w", err)
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
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

// scanRawRow scans a write-side msg_events row, including context_data, and
// its joined payload row (rawCols, then withPayload's three columns).
func scanRawRow(rows pgx.Rows) (*Event, error) {
	var e Event
	var dataBytes, ctxBytes []byte
	var subject, dedupID *string
	var payloadID, pData, pRef *string
	var pSize *int32
	if err := rows.Scan(&e.ID, &e.SpecVersion, &e.Type, &e.Source, &subject,
		&e.Time, &dataBytes, &dedupID, &e.ClientID, &e.MessageGroup,
		&e.CorrelationID, &e.CausationID, &ctxBytes, &e.CreatedAt,
		&payloadID, &pData, &pRef, &pSize); err != nil {
		return nil, err
	}
	if subject != nil {
		e.Subject = *subject
	}
	if dedupID != nil {
		e.DeduplicationID = *dedupID
	}
	e.setData(dataBytes, payloadID, pData, pRef, pSize)
	e.Context = []ContextEntry{}
	if len(ctxBytes) > 0 {
		_ = json.Unmarshal(ctxBytes, &e.Context)
		if e.Context == nil {
			e.Context = []ContextEntry{}
		}
	}
	return &e, nil
}

// ContextByID returns an event's context data from the write-side msg_events
// table (msg_events_read has no context column): empty when the row has none
// or is no longer there (pruned partitions). The detail view fills it in
// after reading the projection.
func (r *Repository) ContextByID(ctx context.Context, id string) ([]ContextEntry, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `SELECT context_data FROM msg_events WHERE id = $1 LIMIT 1`, id).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return []ContextEntry{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("event repo: context_by_id: %w", err)
	}
	out := []ContextEntry{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &out)
		if out == nil {
			out = []ContextEntry{}
		}
	}
	return out, nil
}

// FindByDeduplicationID returns the most recently stored msg_events row
// carrying deduplicationID, or nil if none exists. Used by callers that
// need to report back an idempotent write's existing id — the function
// control plane's emit endpoint (docs/function-runner-plan.md §8.3)
// ingests through InsertBatch, this repository's own dedup path, and then
// looks the row back up here to answer with the id either way, so a retried
// emit with the same dedupId is idempotent the same way POST /api/events is.
//
// Its caller needs the id only, so offloaded data is not resolved (the
// payload store is never called; DataOffloaded says it was left out).
func (r *Repository) FindByDeduplicationID(ctx context.Context, deduplicationID string) (*Event, error) {
	if deduplicationID == "" {
		return nil, nil
	}
	return r.fetchOneRaw(ctx, withPayload(`SELECT `+rawCols+` FROM msg_events WHERE deduplication_id = $1
		   ORDER BY created_at DESC LIMIT 1`, ""), deduplicationID)
}

// IDsByDeduplicationID maps each of deduplicationIDs that has a stored
// msg_events row to the id of the earliest such row. An id with no row is
// absent from the map. One query, for a caller that must answer, after
// InsertBatch, which event each of a batch's records is (its own, or one an
// earlier request stored).
func (r *Repository) IDsByDeduplicationID(ctx context.Context, deduplicationIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(deduplicationIDs))
	if len(deduplicationIDs) == 0 {
		return out, nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT DISTINCT ON (deduplication_id) deduplication_id, id
		   FROM msg_events WHERE deduplication_id = ANY($1)
		   ORDER BY deduplication_id, created_at ASC`, deduplicationIDs)
	if err != nil {
		return nil, fmt.Errorf("event repo: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var dedup, id string
		if err := rows.Scan(&dedup, &id); err != nil {
			return nil, fmt.Errorf("event repo: %w", err)
		}
		out[dedup] = id
	}
	return out, rows.Err()
}

// DistinctValues lists up to `limit` distinct non-null values for the
// given column, of the events a caller with accessibleClientIDs can see
// (nil: every event, for an anchor). Used to populate the frontend's filter-options dropdowns
// (event types, sources, client IDs). The column name is hardcoded by
// the caller — no untrusted SQL.
func (r *Repository) DistinctValues(ctx context.Context, column string, limit int, accessibleClientIDs *[]string) ([]string, error) {
	allowed := map[string]bool{
		"type": true, "source": true, "subject": true,
		"client_id": true, "correlation_id": true,
		"application": true, "subdomain": true, "aggregate": true,
		// principal_id has no backing column on msg_events_read.
	}
	if !allowed[column] {
		return nil, fmt.Errorf("event repo: column %q not allowed for DistinctValues", column)
	}
	limit = min(limit, 1000)
	if limit <= 0 {
		limit = 200
	}
	// SECURITY: tenant scoping as the list's (FilterParams.AccessibleClientIDs):
	// a non-nil set keeps the caller's own tenants plus client-less events of
	// ordinary applications, so a client user never sees another tenant's
	// values nor the platform's control-plane events'.
	scope, args := "", []any{limit}
	if accessibleClientIDs != nil {
		scope = " AND " + fmt.Sprintf(readScopeClause, 2)
		args = append(args, *accessibleClientIDs)
	}
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT DISTINCT %s FROM msg_events_read
		              WHERE %s IS NOT NULL%s ORDER BY 1 LIMIT $1`, column, column, scope),
		args...)
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

// scanRow scans a msg_events_read row and its joined payload row (readCols,
// then withPayload's three columns).
func scanRow(rows pgx.Rows) (*Event, error) {
	var e Event
	var dataBytes []byte
	// spec_version is nullable in the schema (like subject/dedup): scan via
	// a pointer so one NULL row can't 500 the whole list query.
	var specVersion, subject, dedupID *string
	var payloadID, pData, pRef *string
	var pSize *int32
	if err := rows.Scan(&e.ID, &specVersion, &e.Type, &e.Source, &subject,
		&e.Time, &dataBytes, &dedupID, &e.ClientID, &e.MessageGroup,
		&e.CorrelationID, &e.CausationID, &e.CreatedAt,
		&e.Application, &e.Subdomain, &e.Aggregate, &e.ProjectedAt,
		&payloadID, &pData, &pRef, &pSize); err != nil {
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
	e.setData(dataBytes, payloadID, pData, pRef, pSize)
	e.Context = []ContextEntry{}
	return &e, nil
}

func rawJSON(rm json.RawMessage) any {
	if len(rm) == 0 {
		return []byte("{}")
	}
	return []byte(rm)
}

// IsPlatformControlPlane reports whether a client-less event read from
// msg_events_read is one the platform wrote about its own operation: its
// application (the first segment of the type) is the platform's own. Mirrors
// the readScopeClause exclusion for by-id reads.
func IsPlatformControlPlane(e *Event) bool {
	return e != nil && e.ClientID == nil && e.Application != nil && *e.Application == "platform"
}
