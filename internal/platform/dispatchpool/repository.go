package dispatchpool

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

// Repository is the Postgres-backed repository. Table: msg_dispatch_pools.
type Repository struct {
	pool *pgxpool.Pool // retained for the dynamic FindWithFilters query
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*DispatchPool, error) {
	row, err := r.q.DispatchPoolFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dispatch_pool repo: %w", err)
	}
	return rowToDispatchPool(row), nil
}

// FindByCode loads by (code, client_id). clientID may be nil (anchor-scope pool).
func (r *Repository) FindByCode(ctx context.Context, code string, clientID *string) (*DispatchPool, error) {
	var (
		row dbq.MsgDispatchPool
		err error
	)
	if clientID != nil {
		row, err = r.q.DispatchPoolFindByCodeClient(ctx, dbq.DispatchPoolFindByCodeClientParams{
			Code: code, ClientID: clientID,
		})
	} else {
		row, err = r.q.DispatchPoolFindByCodeAnchor(ctx, code)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dispatch_pool repo: %w", err)
	}
	return rowToDispatchPool(row), nil
}

// FindWithFilters returns pools matching non-nil filters. Hand-rolled
// dynamic query — see docs/sqlc.md "Dynamic queries" for the reasoning.
func (r *Repository) FindWithFilters(ctx context.Context, status, clientID *string) ([]DispatchPool, error) {
	const baseSelect = `SELECT id, code, name, description, rate_limit, concurrency,
		client_id, client_identifier, status, created_at, updated_at FROM msg_dispatch_pools`
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
	var out []DispatchPool
	for rows.Next() {
		var row dbq.MsgDispatchPool
		if err := rows.Scan(
			&row.ID, &row.Code, &row.Name, &row.Description, &row.RateLimit,
			&row.Concurrency, &row.ClientID, &row.ClientIdentifier, &row.Status,
			&row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, *rowToDispatchPool(row))
	}
	return out, rows.Err()
}

// Persist implements usecasepgx.Persist[DispatchPool].
func (r *Repository) Persist(ctx context.Context, p *DispatchPool, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).DispatchPoolUpsert(ctx, dbq.DispatchPoolUpsertParams{
		ID:               p.ID,
		Code:             p.Code,
		Name:             p.Name,
		Description:      p.Description,
		RateLimit:        p.RateLimit,
		Concurrency:      p.Concurrency,
		ClientID:         p.ClientID,
		ClientIdentifier: p.ClientIdentifier,
		Status:           string(p.Status),
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        time.Now().UTC(),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, p *DispatchPool, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).DispatchPoolDelete(ctx, p.ID)
}

func rowToDispatchPool(row dbq.MsgDispatchPool) *DispatchPool {
	return &DispatchPool{
		ID:               row.ID,
		Code:             row.Code,
		Name:             row.Name,
		Description:      row.Description,
		RateLimit:        row.RateLimit,
		Concurrency:      row.Concurrency,
		ClientID:         row.ClientID,
		ClientIdentifier: row.ClientIdentifier,
		Status:           ParseStatus(row.Status),
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
