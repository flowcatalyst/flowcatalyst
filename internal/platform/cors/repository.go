package cors

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed cors repository. Table: tnt_cors_allowed_origins.
type Repository struct{ q *dbq.Queries }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads by primary key.
func (r *Repository) FindByID(ctx context.Context, id string) (*AllowedOrigin, error) {
	row, err := r.q.CorsOriginFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cors repo: %w", err)
	}
	return rowToOrigin(row), nil
}

// FindByOrigin loads by unique origin string.
func (r *Repository) FindByOrigin(ctx context.Context, origin string) (*AllowedOrigin, error) {
	row, err := r.q.CorsOriginFindByOrigin(ctx, origin)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cors repo: %w", err)
	}
	return rowToOrigin(row), nil
}

// FindAll returns every origin ordered by origin string.
func (r *Repository) FindAll(ctx context.Context) ([]AllowedOrigin, error) {
	rows, err := r.q.CorsOriginFindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]AllowedOrigin, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToOrigin(row))
	}
	return out, nil
}

// GetAllowedOrigins returns just the origin strings (for the chi CORS middleware).
func (r *Repository) GetAllowedOrigins(ctx context.Context) ([]string, error) {
	return r.q.CorsOriginListStrings(ctx)
}

// Persist implements usecasepgx.Persist[AllowedOrigin].
func (r *Repository) Persist(ctx context.Context, o *AllowedOrigin, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).CorsOriginUpsert(ctx, dbq.CorsOriginUpsertParams{
		ID:          o.ID,
		Origin:      o.Origin,
		Description: o.Description,
		CreatedBy:   o.CreatedBy,
		CreatedAt:   o.CreatedAt,
		UpdatedAt:   time.Now().UTC(),
	})
}

// Delete removes the origin.
func (r *Repository) Delete(ctx context.Context, o *AllowedOrigin, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).CorsOriginDelete(ctx, o.ID)
}

func rowToOrigin(row dbq.TntCorsAllowedOrigin) *AllowedOrigin {
	return &AllowedOrigin{
		ID:          row.ID,
		Origin:      row.Origin,
		Description: row.Description,
		CreatedBy:   row.CreatedBy,
		CreatedAt:   row.CreatedAt,
		UpdatedAt:   row.UpdatedAt,
	}
}
