package connection

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
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
	res, err := r.q.ConnectionFindByID(ctx, id)
	row, err := repocommon.One(res, err, "connection repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConnection(*row), nil
}

// FindByCodeAndClient locates by (code, client_id). clientID may be nil.
func (r *Repository) FindByCodeAndClient(ctx context.Context, code string, clientID *string) (*Connection, error) {
	var (
		res dbq.MsgConnection
		err error
	)
	if clientID != nil {
		res, err = r.q.ConnectionFindByCodeClient(ctx, dbq.ConnectionFindByCodeClientParams{
			Code: code, ClientID: clientID,
		})
	} else {
		res, err = r.q.ConnectionFindByCodeAnchor(ctx, code)
	}
	row, err := repocommon.One(res, err, "connection repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConnection(*row), nil
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
	var f repocommon.Filter
	f.EqPtr("status", status)
	f.EqPtr("client_id", clientID)

	q := `SELECT id, code, name, description, external_id, status,
		service_account_id, client_id, client_identifier, created_at, updated_at
		FROM msg_connections` + f.Where() + ` ORDER BY code`

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.MsgConnection])
	if err != nil {
		return nil, err
	}
	var out []Connection
	for _, row := range collected {
		out = append(out, *rowToConnection(row))
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
