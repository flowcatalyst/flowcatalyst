package connection

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
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
	row, err := r.q.ConnectionFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("connection repo: %w", err)
	}
	return rowToConnection(row), nil
}

// FindByCodeAndClient locates by (code, client_id). clientID may be nil.
func (r *Repository) FindByCodeAndClient(ctx context.Context, code string, clientID *string) (*Connection, error) {
	var (
		row dbq.MsgConnection
		err error
	)
	if clientID != nil {
		row, err = r.q.ConnectionFindByCodeClient(ctx, dbq.ConnectionFindByCodeClientParams{
			Code: code, ClientID: clientID,
		})
	} else {
		row, err = r.q.ConnectionFindByCodeAnchor(ctx, code)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("connection repo: %w", err)
	}
	return rowToConnection(row), nil
}

// FindAll returns every connection.
func (r *Repository) FindAll(ctx context.Context) ([]Connection, error) {
	rows, err := r.q.ConnectionFindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToConnection(row))
	}
	return out, nil
}

// FindWithFilters returns connections matching supplied non-nil filters.
// Hand-rolled dynamic query — see docs/sqlc.md.
func (r *Repository) FindWithFilters(ctx context.Context, status, clientID *string) ([]Connection, error) {
	const baseSelect = `SELECT id, code, name, description, external_id, status,
		service_account_id, client_id, client_identifier, created_at, updated_at
		FROM msg_connections`
	q := baseSelect
	args := []any{}
	conds := []string{}
	if status != nil {
		args = append(args, *status)
		conds = append(conds, fmt.Sprintf("status = $%d", len(args)))
	}
	if clientID != nil {
		args = append(args, *clientID)
		conds = append(conds, fmt.Sprintf("client_id = $%d", len(args)))
	}
	if len(conds) > 0 {
		q += ` WHERE ` + strings.Join(conds, ` AND `)
	}
	q += ` ORDER BY code`
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Connection
	for rows.Next() {
		var row dbq.MsgConnection
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description, &row.ExternalID, &row.Status,
			&row.ServiceAccountID, &row.ClientID, &row.ClientIdentifier,
			&row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, *rowToConnection(row))
	}
	return out, rows.Err()
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
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, c *Connection, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ConnectionDelete(ctx, c.ID)
}

func rowToConnection(row dbq.MsgConnection) *Connection {
	return &Connection{
		ID:               row.ID,
		Code:             row.Code,
		Name:             row.Name,
		Description:      row.Description,
		ExternalID:       row.ExternalID,
		Status:           ParseStatus(row.Status),
		ServiceAccountID: row.ServiceAccountID,
		ClientID:         row.ClientID,
		ClientIdentifier: row.ClientIdentifier,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
