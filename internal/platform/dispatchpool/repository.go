package dispatchpool

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
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
	res, err := r.q.DispatchPoolFindByID(ctx, id)
	row, err := repocommon.One(res, err, "dispatch_pool repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToDispatchPool(*row), nil
}

// FindByCode loads by (code, client_id). clientID may be nil (anchor-scope pool).
func (r *Repository) FindByCode(ctx context.Context, code string, clientID *string) (*DispatchPool, error) {
	var (
		res dbq.MsgDispatchPool
		err error
	)
	if clientID != nil {
		res, err = r.q.DispatchPoolFindByCodeClient(ctx, dbq.DispatchPoolFindByCodeClientParams{
			Code: code, ClientID: clientID,
		})
	} else {
		res, err = r.q.DispatchPoolFindByCodeAnchor(ctx, code)
	}
	row, err := repocommon.One(res, err, "dispatch_pool repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToDispatchPool(*row), nil
}

// FindWithFilters returns pools matching non-nil filters. Hand-rolled
// dynamic query — see docs/sqlc.md "Dynamic queries" for the reasoning.
func (r *Repository) FindWithFilters(ctx context.Context, status, clientID *string) ([]DispatchPool, error) {
	var f repocommon.Filter
	f.EqPtr("status", status)
	f.EqPtr("client_id", clientID)

	q := `SELECT id, code, name, description, rate_limit, concurrency,
		client_id, client_identifier, status, created_at, updated_at
	  FROM msg_dispatch_pools` + f.Where() + ` ORDER BY code`

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.MsgDispatchPool])
	if err != nil {
		return nil, err
	}
	var out []DispatchPool
	for _, row := range collected {
		out = append(out, *rowToDispatchPool(row))
	}
	return out, nil
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
