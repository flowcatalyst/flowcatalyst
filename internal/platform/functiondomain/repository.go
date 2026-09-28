package functiondomain

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repository for fng_domains. Hand-rolled
// pgx SQL (no sqlc queries file) — the same "dynamic queries" precedent
// internal/platform/function/repository.go's Versions section documents:
// a small, self-contained aggregate with no shared query file to collide
// with concurrent work.
type Repository struct {
	pool *pgxpool.Pool
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

const domainColumns = `id, zone, client_id, created_by, created_at, updated_at`

type domainRow struct {
	ID        string
	Zone      string
	ClientID  *string
	CreatedBy *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// rowToDomain converts domainRow to FunctionDomain — a plain type
// conversion, not a field-by-field literal, because the two structs share
// identical field names, order and types.
func rowToDomain(r domainRow) FunctionDomain { return FunctionDomain(r) }

// FindByID loads a domain claim by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*FunctionDomain, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+domainColumns+` FROM fng_domains WHERE id = $1`, id)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[domainRow])
	d, err := repocommon.One(row, err, "function domain repo")
	if d == nil || err != nil {
		return nil, err
	}
	out := rowToDomain(*d)
	return &out, nil
}

// FindByZone loads a domain claim by its unique zone.
func (r *Repository) FindByZone(ctx context.Context, zone string) (*FunctionDomain, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+domainColumns+` FROM fng_domains WHERE zone = $1`, zone)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[domainRow])
	d, err := repocommon.One(row, err, "function domain repo")
	if d == nil || err != nil {
		return nil, err
	}
	out := rowToDomain(*d)
	return &out, nil
}

// FindAll returns every claimed domain — used by the overlap check, which
// must consider claims regardless of the caller's own reach (a different
// client's claim can still conflict with a new one).
func (r *Repository) FindAll(ctx context.Context) ([]FunctionDomain, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+domainColumns+` FROM fng_domains ORDER BY zone`)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[domainRow])
	if err != nil {
		return nil, err
	}
	out := make([]FunctionDomain, 0, len(collected))
	for _, row := range collected {
		out = append(out, rowToDomain(row))
	}
	return out, nil
}

// ListFilters drives FindWithFilters — the same AccessibleClientIDs
// reach-scoping rule as function.ListFilters.
type ListFilters struct {
	AccessibleClientIDs *[]string
}

// FindWithFilters returns claimed domains visible under f, ordered by zone.
func (r *Repository) FindWithFilters(ctx context.Context, f ListFilters) ([]FunctionDomain, error) {
	var flt repocommon.Filter
	if f.AccessibleClientIDs != nil {
		flt.Clause("(client_id IS NULL OR client_id = ANY($%d))", *f.AccessibleClientIDs)
	}
	q := `SELECT ` + domainColumns + ` FROM fng_domains` + flt.Where() + ` ORDER BY zone`
	rows, err := r.pool.Query(ctx, q, flt.Args()...)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[domainRow])
	if err != nil {
		return nil, err
	}
	out := make([]FunctionDomain, 0, len(collected))
	for _, row := range collected {
		out = append(out, rowToDomain(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[FunctionDomain].
func (r *Repository) Persist(ctx context.Context, d *FunctionDomain, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `
		INSERT INTO fng_domains (id, zone, client_id, created_by, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6)
		ON CONFLICT (id) DO UPDATE SET
			zone = EXCLUDED.zone, client_id = EXCLUDED.client_id, updated_at = EXCLUDED.updated_at`,
		d.ID, d.Zone, d.ClientID, d.CreatedBy, d.CreatedAt, time.Now().UTC())
	return err
}

// Delete removes the domain claim row.
func (r *Repository) Delete(ctx context.Context, d *FunctionDomain, tx *usecasepgx.DbTx) error {
	_, err := tx.Inner().Exec(ctx, `DELETE FROM fng_domains WHERE id = $1`, d.ID)
	return err
}

// FindCoveringZone reports whether some zone claimed by clientID (nil =
// platform) covers hostname (plan: "a route's hostname must be covered by a
// zone claimed by the function's client, or, for a platform-owned function,
// a platform zone with null client"). Used by the route write path
// (function/operations/routes.go) for the ROUTE_HOST_NOT_CLAIMED check.
func (r *Repository) FindCoveringZone(ctx context.Context, hostname string, clientID *string) (zone string, ok bool, err error) {
	var rows pgx.Rows
	if clientID == nil {
		rows, err = r.pool.Query(ctx, `SELECT zone FROM fng_domains WHERE client_id IS NULL`)
	} else {
		rows, err = r.pool.Query(ctx, `SELECT zone FROM fng_domains WHERE client_id = $1`, *clientID)
	}
	if err != nil {
		return "", false, err
	}
	zones, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", false, err
	}
	for _, z := range zones {
		if Covers(z, hostname) {
			return z, true, nil
		}
	}
	return "", false, nil
}

// ListDistinctRouteHostnames returns every distinct hostname any function
// route currently uses. Queries fng_routes directly by raw SQL rather than
// importing internal/platform/function (which would create an import
// cycle: function's route-coverage check already imports this package to
// find a covering zone) — a plain hostname string is all the DOMAIN_IN_USE
// check (delete.go) needs.
func (r *Repository) ListDistinctRouteHostnames(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT DISTINCT hostname FROM fng_routes`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
