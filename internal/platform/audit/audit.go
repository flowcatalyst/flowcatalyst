// Package audit is the port of fc-platform/src/audit. Audit log entity
// and read-only repository.
//
// Rows are written by the UoW Sink (platformsink.Sink.WriteAudit) — not
// by a use case in this package. This subdomain only provides the read
// API for the platform's audit-trail screens.
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

