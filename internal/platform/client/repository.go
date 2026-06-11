package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Table: tnt_clients.
// Notes are stored as JSONB on the row (matches Rust schema).
//
// All SQL lives in internal/sqlc/queries/client.sql and goes through
// the sqlc-generated *dbq.Queries. Compile-time schema checks live
// there — adding a column means regenerating dbq, not editing this file.
type Repository struct {
	q *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Client, error) {
	res, err := r.q.ClientFindByID(ctx, id)
	row, err := repocommon.One(res, err, "client repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToClient(*row)
}

// FindByIdentifier loads by unique identifier (URL-safe slug).
func (r *Repository) FindByIdentifier(ctx context.Context, identifier string) (*Client, error) {
	res, err := r.q.ClientFindByIdentifier(ctx, identifier)
	row, err := repocommon.One(res, err, "client repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToClient(*row)
}

// Search performs a case-insensitive prefix match on name + identifier.
// Used by the frontend's "find a client" autocomplete. Returns at most 50.
func (r *Repository) Search(ctx context.Context, term string) ([]Client, error) {
	rows, err := r.q.ClientSearch(ctx, "%"+term+"%")
	if err != nil {
		return nil, err
	}
	return rowsToClients(rows)
}

// FindAll returns every client.
func (r *Repository) FindAll(ctx context.Context) ([]Client, error) {
	rows, err := r.q.ClientFindAll(ctx)
	if err != nil {
		return nil, err
	}
	return rowsToClients(rows)
}

// Persist implements usecasepgx.Persist[Client].
func (r *Repository) Persist(ctx context.Context, c *Client, tx *usecasepgx.DbTx) error {
	notesJSON, err := json.Marshal(c.Notes)
	if err != nil {
		return fmt.Errorf("marshal notes: %w", err)
	}
	return r.q.WithTx(tx.Inner()).ClientUpsert(ctx, dbq.ClientUpsertParams{
		ID:              c.ID,
		Name:            c.Name,
		Identifier:      c.Identifier,
		Status:          string(c.Status),
		StatusReason:    c.StatusReason,
		StatusChangedAt: c.StatusChangedAt,
		Notes:           notesJSON,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       time.Now().UTC(),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, c *Client, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ClientDelete(ctx, c.ID)
}

// rowToClient projects a sqlc-generated row onto the aggregate's Client.
func rowToClient(row dbq.TntClient) (*Client, error) {
	c := Client{
		ID:              row.ID,
		Name:            row.Name,
		Identifier:      row.Identifier,
		Status:          ParseStatus(row.Status),
		StatusReason:    row.StatusReason,
		StatusChangedAt: row.StatusChangedAt,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		Notes:           []Note{},
	}
	if len(row.Notes) > 0 {
		if err := json.Unmarshal(row.Notes, &c.Notes); err != nil {
			return nil, fmt.Errorf("unmarshal notes: %w", err)
		}
	}
	return &c, nil
}

func rowsToClients(rows []dbq.TntClient) ([]Client, error) {
	out := make([]Client, 0, len(rows))
	for _, r := range rows {
		c, err := rowToClient(r)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}
