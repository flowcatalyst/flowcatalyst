// Package audit holds the audit log entity
// and repository (read API + a direct batch-ingest Insert).
//
// During platform mutations, rows are written by the UoW Sink
// (platformsink.Sink.WriteAudit) — not by a use case in this package. The
// repository's read API backs the audit-trail screens; its Insert backs the
// SDK/outbox batch audit-ingest endpoint (POST /api/audit-logs/batch).
package audit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
)

// Log is the audit log entity. Schema matches aud_logs.
type Log struct {
	ID            string          `json:"id"`
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	Operation     string          `json:"operation"`
	OperationJSON json.RawMessage `json:"operationJson,omitempty"`
	PrincipalID   *string         `json:"principalId,omitempty"`
	PrincipalName *string         `json:"principalName,omitempty"`
	ApplicationID *string         `json:"applicationId,omitempty"`
	ClientID      *string         `json:"clientId,omitempty"`
	PerformedAt   time.Time       `json:"performedAt"`
}

// Repository is the read-only audit log repo.
type Repository struct {
	pool *pgxpool.Pool // retained for DistinctValues (dynamic column)
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// Insert writes a single audit-log row to aud_logs. The subdomain is otherwise
// read-only — platform mutations write audit rows through the UoW Sink
// (platformsink.WriteAudit). This direct insert backs the SDK/outbox batch
// audit-ingest endpoint (POST /api/audit-logs/batch) —
// a plain insert outside the UoW. The column set
// matches WriteAudit + migrations 006/009.
func (r *Repository) Insert(ctx context.Context, l *Log) error {
	return r.InsertBatch(ctx, []*Log{l})
}

// InsertBatch writes audit rows as one pipelined pgx batch — a single round
// trip and implicit transaction, so a 100-item ingest batch is all-or-nothing
// instead of 100 sequential auto-committed inserts.
func (r *Repository) InsertBatch(ctx context.Context, logs []*Log) error {
	if len(logs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, l := range logs {
		var opJSON any
		if len(l.OperationJSON) > 0 {
			opJSON = []byte(l.OperationJSON)
		}
		batch.Queue(
			`INSERT INTO aud_logs
			     (id, entity_type, entity_id, operation,
			      operation_json, principal_id, application_id,
			      client_id, performed_at)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)`,
			l.ID, l.EntityType, l.EntityID, l.Operation,
			opJSON, l.PrincipalID, l.ApplicationID, l.ClientID, l.PerformedAt)
	}
	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range logs {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert aud_logs: %w", err)
		}
	}
	return nil
}

// FilterParams is the query DTO for list endpoints.
type FilterParams struct {
	EntityType  *string
	EntityID    *string
	PrincipalID *string
	ClientID    *string
	Since       *time.Time
	Until       *time.Time
	Limit       int
	Offset      int

	// AccessibleClientIDs, when non-nil, confines the result to rows whose
	// client_id is one of the set (rows with no client are platform records and
	// are excluded); nil means no confinement (anchor). It is honoured by
	// routing the query through the hand-rolled FindWithCursor path, which
	// supports only EntityType/EntityID/PrincipalID/ClientID/Limit: Since,
	// Until and Offset are ignored when it is set (no confined caller uses
	// them).
	AccessibleClientIDs *[]string
}

// Cursor is the keyset position for cursor pagination —
// keyset on (performed_at, id) DESC.
type Cursor struct {
	PerformedAt time.Time
	ID          string
}

// CursorFilterParams is the query DTO for the cursor-paginated list endpoint.
// EntityType/Operation are scalar equality filters; ApplicationIDs/ClientIDs
// are IN-list filters (sent as CSV by the SPA). After is the keyset position
// from a previous page (nil for the first page). Limit is the fetch size and
// should already include the +1 over-fetch used to compute hasMore.
type CursorFilterParams struct {
	EntityType     *string
	EntityID       *string
	PrincipalID    *string
	Operation      *string
	ApplicationIDs []string
	ClientIDs      []string
	After          *Cursor
	Limit          int

	// AccessibleClientIDs, when non-nil, confines the result to rows whose
	// client_id is one of the set; rows with no client are excluded. ClientIDs
	// can only narrow within it. nil means no confinement (anchor).
	AccessibleClientIDs *[]string
}

// confineToClients adds the tenant-confinement condition: client_id must be
// in the set (a NULL client_id never is). An empty set matches nothing.
func confineToClients(f *repocommon.Filter, accessible *[]string) {
	if accessible == nil {
		return
	}
	ids := *accessible
	if ids == nil {
		ids = []string{}
	}
	f.Clause("a.client_id = ANY($%d)", ids)
}

// FindWithCursor returns audit logs matching the filters, ordered by
// (performed_at, id) DESC, starting strictly after the given cursor. It is
// hand-rolled (rather than sqlc-generated) because the variadic IN-lists and
// the keyset comparison can't be expressed in the existing static query.
func (r *Repository) FindWithCursor(ctx context.Context, p CursorFilterParams) ([]Log, error) {
	limit := p.Limit
	if limit <= 0 || limit > 501 {
		limit = 101
	}

	var f repocommon.Filter
	f.EqPtr("a.entity_type", p.EntityType)
	f.EqPtr("a.entity_id", p.EntityID)
	f.EqPtr("a.principal_id", p.PrincipalID)
	f.EqPtr("a.operation", p.Operation)
	f.Any("a.application_id", p.ApplicationIDs)
	f.Any("a.client_id", p.ClientIDs)
	confineToClients(&f, p.AccessibleClientIDs)
	if p.After != nil {
		// Keyset: rows strictly before the cursor in (performed_at, id) DESC.
		ph1 := f.Arg(p.After.PerformedAt)
		f.Clause(fmt.Sprintf("(a.performed_at, a.id) < ($%d, $%%d)", ph1), p.After.ID)
	}
	lim := f.Arg(int32(limit))

	q := `SELECT a.id, a.entity_type, a.entity_id, a.operation, a.operation_json,
       a.principal_id, p.name AS principal_name,
       a.application_id, a.client_id, a.performed_at
FROM aud_logs a
LEFT JOIN iam_principals p ON p.id = a.principal_id` + f.Where() +
		fmt.Sprintf(" ORDER BY a.performed_at DESC, a.id DESC LIMIT $%d", lim)

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, fmt.Errorf("audit FindWithCursor: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.AuditFindByIDRow])
	if err != nil {
		return nil, err
	}
	out := make([]Log, 0, len(collected))
	for _, row := range collected {
		out = append(out, Log{
			ID:            row.ID,
			EntityType:    row.EntityType,
			EntityID:      row.EntityID,
			Operation:     row.Operation,
			OperationJSON: row.OperationJson,
			PrincipalID:   row.PrincipalID,
			PrincipalName: row.PrincipalName,
			ApplicationID: row.ApplicationID,
			ClientID:      row.ClientID,
			PerformedAt:   row.PerformedAt,
		})
	}
	return out, nil
}

// FindWithFilters returns audit logs matching non-nil filters, ordered by
// most recent first.
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]Log, error) {
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if p.AccessibleClientIDs != nil {
		cp := CursorFilterParams{
			EntityType: p.EntityType, EntityID: p.EntityID, PrincipalID: p.PrincipalID,
			AccessibleClientIDs: p.AccessibleClientIDs, Limit: limit,
		}
		if p.ClientID != nil {
			cp.ClientIDs = []string{*p.ClientID}
		}
		return r.FindWithCursor(ctx, cp)
	}
	rows, err := r.q.AuditFindWithFilters(ctx, dbq.AuditFindWithFiltersParams{
		EntityType:  p.EntityType,
		EntityID:    p.EntityID,
		PrincipalID: p.PrincipalID,
		ClientID:    p.ClientID,
		Since:       p.Since,
		Until:       p.Until,
		Lim:         int32(limit),
		Off:         int32(p.Offset),
	})
	if err != nil {
		return nil, fmt.Errorf("audit FindWithFilters: %w", err)
	}
	out := make([]Log, 0, len(rows))
	for _, row := range rows {
		out = append(out, Log{
			ID:            row.ID,
			EntityType:    row.EntityType,
			EntityID:      row.EntityID,
			Operation:     row.Operation,
			OperationJSON: row.OperationJson,
			PrincipalID:   row.PrincipalID,
			PrincipalName: row.PrincipalName,
			ApplicationID: row.ApplicationID,
			ClientID:      row.ClientID,
			PerformedAt:   row.PerformedAt,
		})
	}
	return out, nil
}

// FindByID returns one audit log.
func (r *Repository) FindByID(ctx context.Context, id string) (*Log, error) {
	row, err := r.q.AuditFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Log{
		ID:            row.ID,
		EntityType:    row.EntityType,
		EntityID:      row.EntityID,
		Operation:     row.Operation,
		OperationJSON: row.OperationJson,
		PrincipalID:   row.PrincipalID,
		PrincipalName: row.PrincipalName,
		ApplicationID: row.ApplicationID,
		ClientID:      row.ClientID,
		PerformedAt:   row.PerformedAt,
	}, nil
}

// DistinctValues lists distinct non-null values for a whitelisted column.
// Used by the facet endpoints (/api/audit-logs/entity-types, /operations,
// /application-ids, /client-ids). Stays hand-rolled because the column
// name is dynamic — sqlc can't express that.
//
// accessibleClientIDs confines the values to those of rows whose client_id is in
// the set (nil: every row, for an anchor).
func (r *Repository) DistinctValues(ctx context.Context, column string, limit int, accessibleClientIDs *[]string) ([]string, error) {
	allowed := map[string]bool{
		"entity_type": true, "operation": true,
		"application_id": true, "client_id": true,
	}
	if !allowed[column] {
		return nil, fmt.Errorf("audit repo: column %q not allowed", column)
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	scope, args := "", []any{limit}
	if accessibleClientIDs != nil {
		ids := *accessibleClientIDs
		if ids == nil {
			ids = []string{}
		}
		scope = " AND client_id = ANY($2)"
		args = append(args, ids)
	}
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT DISTINCT %s FROM aud_logs
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

// AccessibleClients is the set of clients ac's audit reads are confined to: nil
// for an anchor (every row), otherwise the clients it reaches. Rows with no
// client are platform records and are in no such set. The view permission says
// what ac may read; this says for which tenants.
func AccessibleClients(ac *auth.AuthContext) *[]string {
	if ac.IsAnchor() {
		return nil
	}
	clients := ac.Clients
	if clients == nil {
		clients = []string{}
	}
	return &clients
}

// CanSee reports whether ac may read the audit row l under AccessibleClients.
func CanSee(ac *auth.AuthContext, l *Log) bool {
	return ac.IsAnchor() || (l.ClientID != nil && ac.CanAccessClient(*l.ClientID))
}
