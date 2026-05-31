// Package audit is the port of fc-platform/src/audit. Audit log entity
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
// audit-ingest endpoint (POST /api/audit-logs/batch), mirroring Rust
// audit_log_repo.insert (a plain insert outside the UoW). The column set
// matches WriteAudit + migrations 006/009.
func (r *Repository) Insert(ctx context.Context, l *Log) error {
	var opJSON any
	if len(l.OperationJSON) > 0 {
		opJSON = []byte(l.OperationJSON)
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO aud_logs
		     (id, entity_type, entity_id, operation,
		      operation_json, principal_id, application_id,
		      client_id, performed_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)`,
		l.ID, l.EntityType, l.EntityID, l.Operation,
		opJSON, l.PrincipalID, l.ApplicationID, l.ClientID, l.PerformedAt)
	if err != nil {
		return fmt.Errorf("insert aud_logs: %w", err)
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
}

// Cursor is the keyset position for cursor pagination, mirroring the Rust
// platform's keyset on (performed_at, id) DESC.
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

	var (
		sb   []byte
		args []any
	)
	add := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	sb = append(sb, []byte(`SELECT a.id, a.entity_type, a.entity_id, a.operation, a.operation_json,
       a.principal_id, p.name AS principal_name,
       a.application_id, a.client_id, a.performed_at
FROM aud_logs a
LEFT JOIN iam_principals p ON p.id = a.principal_id
WHERE 1=1`)...)

	if p.EntityType != nil {
		sb = append(sb, []byte(" AND a.entity_type = "+add(*p.EntityType))...)
	}
	if p.EntityID != nil {
		sb = append(sb, []byte(" AND a.entity_id = "+add(*p.EntityID))...)
	}
	if p.PrincipalID != nil {
		sb = append(sb, []byte(" AND a.principal_id = "+add(*p.PrincipalID))...)
	}
	if p.Operation != nil {
		sb = append(sb, []byte(" AND a.operation = "+add(*p.Operation))...)
	}
	if len(p.ApplicationIDs) > 0 {
		sb = append(sb, []byte(" AND a.application_id = ANY("+add(p.ApplicationIDs)+")")...)
	}
	if len(p.ClientIDs) > 0 {
		sb = append(sb, []byte(" AND a.client_id = ANY("+add(p.ClientIDs)+")")...)
	}
	if p.After != nil {
		// Keyset: rows strictly before the cursor in (performed_at, id) DESC.
		ph1 := add(p.After.PerformedAt)
		ph2 := add(p.After.ID)
		sb = append(sb, []byte(" AND (a.performed_at, a.id) < ("+ph1+", "+ph2+")")...)
	}

	sb = append(sb, []byte(" ORDER BY a.performed_at DESC, a.id DESC LIMIT "+add(int32(limit)))...)

	rows, err := r.pool.Query(ctx, string(sb), args...)
	if err != nil {
		return nil, fmt.Errorf("audit FindWithCursor: %w", err)
	}
	defer rows.Close()
	out := make([]Log, 0, limit)
	for rows.Next() {
		var l Log
		if err := rows.Scan(
			&l.ID,
			&l.EntityType,
			&l.EntityID,
			&l.Operation,
			&l.OperationJSON,
			&l.PrincipalID,
			&l.PrincipalName,
			&l.ApplicationID,
			&l.ClientID,
			&l.PerformedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// FindWithFilters returns audit logs matching non-nil filters, ordered by
// most recent first.
func (r *Repository) FindWithFilters(ctx context.Context, p FilterParams) ([]Log, error) {
	limit := p.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
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
func (r *Repository) DistinctValues(ctx context.Context, column string, limit int) ([]string, error) {
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
	rows, err := r.pool.Query(ctx,
		fmt.Sprintf(`SELECT DISTINCT %s FROM aud_logs
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
