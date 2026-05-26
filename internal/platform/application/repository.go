package application

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

// Repository is the Postgres-backed repo. Table: app_applications.
type Repository struct {
	pool *pgxpool.Pool // retained for FindWithFilters
	q    *dbq.Queries
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool, q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Application, error) {
	row, err := r.q.ApplicationFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("application repo: %w", err)
	}
	return rowToApplication(row), nil
}

// FindByCode loads by unique code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*Application, error) {
	row, err := r.q.ApplicationFindByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("application repo: %w", err)
	}
	return rowToApplication(row), nil
}

// FindWithFilters returns apps matching non-nil filters. Hand-rolled
// dynamic query — see docs/sqlc.md.
func (r *Repository) FindWithFilters(ctx context.Context, appType, active *string) ([]Application, error) {
	const baseSelect = `SELECT id, type, code, name, description, icon_url, website,
		logo, logo_mime_type, default_base_url, service_account_id, active,
		created_at, updated_at FROM app_applications`
	q := baseSelect
	args := []any{}
	conds := []string{}
	if appType != nil {
		args = append(args, *appType)
		conds = append(conds, fmt.Sprintf("type = $%d", len(args)))
	}
	if active != nil {
		args = append(args, *active == "true")
		conds = append(conds, fmt.Sprintf("active = $%d", len(args)))
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
	var out []Application
	for rows.Next() {
		var row dbq.AppApplication
		if err := rows.Scan(
			&row.ID, &row.Type, &row.Code, &row.Name, &row.Description, &row.IconUrl,
			&row.Website, &row.Logo, &row.LogoMimeType, &row.DefaultBaseUrl,
			&row.ServiceAccountID, &row.Active, &row.CreatedAt, &row.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, *rowToApplication(row))
	}
	return out, rows.Err()
}

// Persist implements usecasepgx.Persist[Application].
func (r *Repository) Persist(ctx context.Context, a *Application, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ApplicationUpsert(ctx, dbq.ApplicationUpsertParams{
		ID:               a.ID,
		Type:             string(a.Type),
		Code:             a.Code,
		Name:             a.Name,
		Description:      a.Description,
		IconUrl:          a.IconURL,
		Website:          a.Website,
		Logo:             a.Logo,
		LogoMimeType:     a.LogoMimeType,
		DefaultBaseUrl:   a.DefaultBaseURL,
		ServiceAccountID: a.ServiceAccountID,
		Active:           a.Active,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        time.Now().UTC(),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, a *Application, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ApplicationDelete(ctx, a.ID)
}

func rowToApplication(row dbq.AppApplication) *Application {
	return &Application{
		ID:               row.ID,
		Type:             ParseType(row.Type),
		Code:             row.Code,
		Name:             row.Name,
		Description:      row.Description,
		IconURL:          row.IconUrl,
		Website:          row.Website,
		Logo:             row.Logo,
		LogoMimeType:     row.LogoMimeType,
		DefaultBaseURL:   row.DefaultBaseUrl,
		ServiceAccountID: row.ServiceAccountID,
		Active:           row.Active,
		CreatedAt:        row.CreatedAt,
		UpdatedAt:        row.UpdatedAt,
	}
}
