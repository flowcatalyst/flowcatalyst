package connection

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Table: msg_connections.
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by primary key.
func (r *Repository) FindByID(ctx context.Context, id string) (*Connection, error) {
	res, err := r.q.ConnectionFindByID(ctx, id)
	row, err := repocommon.One(res, err, "connection repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConnection(*row)
}

// FindByCode locates by (code, application_code, client_id). applicationCode
// and clientID may each independently be nil — "no application" (a shared
// connection) and "no client" (global) are real values in the key, not
// wildcards, so a nil here only matches a row whose column is also NULL.
func (r *Repository) FindByCode(ctx context.Context, code string, applicationCode, clientID *string) (*Connection, error) {
	res, err := r.q.ConnectionFindByCode(ctx, dbq.ConnectionFindByCodeParams{
		Code: code, ApplicationCode: applicationCode, ClientID: clientID,
	})
	row, err := repocommon.One(res, err, "connection repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConnection(*row)
}

// FindAll returns every connection.
func (r *Repository) FindAll(ctx context.Context) ([]Connection, error) {
	rows, err := r.q.ConnectionFindAll(ctx)
	if err != nil {
		return nil, err
	}
	// A corrupted status on any one row fails the WHOLE list read (X-06:
	// "a list containing the row fails too") rather than silently skipping
	// or coercing that row.
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		c, err := rowToConnection(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// FindWithFilters returns connections matching supplied non-nil filters.
// Hand-rolled dynamic query — see docs/sqlc.md.
func (r *Repository) FindWithFilters(ctx context.Context, status, clientID *string) ([]Connection, error) {
	var f repocommon.Filter
	f.EqPtr("status", status)
	f.EqPtr("client_id", clientID)

	q := `SELECT id, code, name, description, external_id, status,
		service_account_id, client_id, client_identifier, created_at, updated_at,
		application_code, source
		FROM msg_connections` + f.Where() + ` ORDER BY code`

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.MsgConnection])
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(collected))
	for _, row := range collected {
		c, err := rowToConnection(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[Connection].
func (r *Repository) Persist(ctx context.Context, c *Connection, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ConnectionUpsert(ctx, dbq.ConnectionUpsertParams{
		ID:               c.ID,
		Code:             c.Code,
		Name:             c.Name,
		Description:      c.Description,
		ExternalID:       c.ExternalID,
		Status:           string(c.Status),
		ServiceAccountID: c.ServiceAccountID,
		ClientID:         c.ClientID,
		ClientIdentifier: c.ClientIdentifier,
		CreatedAt:        c.CreatedAt,
		UpdatedAt:        time.Now().UTC(),
		ApplicationCode:  c.ApplicationCode,
		Source:           string(c.Source),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, c *Connection, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ConnectionDelete(ctx, c.ID)
}

// rowToConnection hydrates the entity from its row. A status or source value
// that isn't one of the known constants (junk written before write-boundary
// validation existed, or a hand-edited row) is a loud read error — never
// round-tripped as-is and never coerced to a default, per the X-06 ruling.
// The row id is logged so the bad row can be found and fixed without a
// debugger.
func rowToConnection(row dbq.MsgConnection) (*Connection, error) {
	status, ok := ParseStatus(row.Status)
	if !ok {
		slog.Error("connection row has unrecognised status", "id", row.ID, "status", row.Status)
		return nil, usecase.Internal("CORRUPT_CONNECTION_STATUS",
			fmt.Sprintf("connection %s has an unrecognised status", row.ID), nil)
	}
	source, ok := ParseSource(row.Source)
	if !ok {
		slog.Error("connection row has unrecognised source", "id", row.ID, "source", row.Source)
		return nil, usecase.Internal("CORRUPT_CONNECTION_SOURCE",
			fmt.Sprintf("connection %s has an unrecognised source", row.ID), nil)
	}
	return &Connection{
		ID:               row.ID,
		Code:             row.Code,
		ApplicationCode:  row.ApplicationCode,
		Name:             row.Name,
		Description:      row.Description,
		ExternalID:       row.ExternalID,
		Status:           status,
		ServiceAccountID: row.ServiceAccountID,
		ClientID:         row.ClientID,
		ClientIdentifier: row.ClientIdentifier,
		Source:           source,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}, nil
}
