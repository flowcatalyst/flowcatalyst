package application

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
	res, err := r.q.ApplicationFindByID(ctx, id)
	row, err := repocommon.One(res, err, "application repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToApplication(*row)
}

// FindByCode loads by unique code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*Application, error) {
	res, err := r.q.ApplicationFindByCode(ctx, code)
	row, err := repocommon.One(res, err, "application repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToApplication(*row)
}

// FindWithFilters returns apps matching non-nil filters. Hand-rolled
// dynamic query — see docs/sqlc.md.
// FindActive returns every active application. Convenience wrapper
// around FindWithFilters used by the developer BFF.
func (r *Repository) FindActive(ctx context.Context) ([]Application, error) {
	active := "true"
	return r.FindWithFilters(ctx, nil, &active)
}

func (r *Repository) FindWithFilters(ctx context.Context, appType, active *string) ([]Application, error) {
	var f repocommon.Filter
	f.EqPtr("type", appType)
	if active != nil {
		f.Eq("active", *active == "true")
	}

	q := `SELECT id, type, code, name, description, icon_url, website,
		logo, logo_mime_type, default_base_url, service_account_id, active,
		created_at, updated_at FROM app_applications` + f.Where() + ` ORDER BY code`

	rows, err := r.pool.Query(ctx, q, f.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[dbq.AppApplication])
	if err != nil {
		return nil, err
	}
	// A corrupted type on any one row fails the WHOLE list read (X-06: "a
	// list containing the row fails too") rather than silently skipping or
	// coercing that row.
	out := make([]Application, 0, len(collected))
	for _, row := range collected {
		a, err := rowToApplication(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, nil
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

// rowToApplication hydrates the entity from its row. A type value that
// isn't one of the known Type constants (junk written before write-boundary
// validation existed, or a hand-edited row) is a loud read error — never
// round-tripped as-is and never coerced to APPLICATION, per the X-06
// ruling. The row id is logged so the bad row can be found and fixed
// without a debugger.
func rowToApplication(row dbq.AppApplication) (*Application, error) {
	typ, ok := ParseType(row.Type)
	if !ok {
		slog.Error("application row has unrecognised type",
			"id", row.ID, "type", row.Type)
		return nil, usecase.Internal("CORRUPT_APPLICATION_TYPE",
			fmt.Sprintf("application %s has an unrecognised type", row.ID), nil)
	}
	return &Application{
		ID:               row.ID,
		Type:             typ,
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
	}, nil
}
