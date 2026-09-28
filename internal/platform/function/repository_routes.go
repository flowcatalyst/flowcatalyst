package function

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
)

// This file is the fng_routes sibling of repository_control.go's Versions
// section: hand-rolled pgx SQL, not sqlc, for the same reason — a small,
// self-contained set of columns with no shared query file to collide with
// concurrent work on internal/sqlc/queries/function.sql.

const routeColumns = `id, function_id, hostname, path_prefix, alias, created_by, created_at, updated_at`

type routeRow struct {
	ID         string
	FunctionID string `db:"function_id"`
	Hostname   string
	PathPrefix string `db:"path_prefix"`
	Alias      *string
	CreatedBy  *string   `db:"created_by"`
	CreatedAt  time.Time `db:"created_at"`
	UpdatedAt  time.Time `db:"updated_at"`
}

// rowToRoute converts routeRow to Route — a plain type conversion, not a
// field-by-field literal, because the two structs share identical field
// names, order and types (only the db tags differ).
func rowToRoute(r routeRow) Route { return Route(r) }

// FindRouteByID loads a route by its own row id.
func (r *Repository) FindRouteByID(ctx context.Context, id string) (*Route, error) {
	return r.getRoute(ctx, `id = $1`, id)
}

// FindRouteByHostPrefix loads a route by its (hostname, pathPrefix) key,
// regardless of which function owns it — the identity PutRoute upserts by,
// and the uniqueness check every other function's route must not collide
// with (ROUTE_TAKEN).
func (r *Repository) FindRouteByHostPrefix(ctx context.Context, hostname, pathPrefix string) (*Route, error) {
	return r.getRoute(ctx, `hostname = $1 AND path_prefix = $2`, hostname, pathPrefix)
}

func (r *Repository) getRoute(ctx context.Context, where string, args ...any) (*Route, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+routeColumns+` FROM fng_routes WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[routeRow])
	rr, err := repocommon.One(row, err, "function route repo")
	if rr == nil || err != nil {
		return nil, err
	}
	out := rowToRoute(*rr)
	return &out, nil
}

// ListRoutesByFunction returns every route owned by functionID, ordered by
// (hostname, path_prefix) for a stable listing.
func (r *Repository) ListRoutesByFunction(ctx context.Context, functionID string) ([]Route, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+routeColumns+` FROM fng_routes
		WHERE function_id = $1 ORDER BY hostname, path_prefix`, functionID)
	if err != nil {
		return nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[routeRow])
	if err != nil {
		return nil, err
	}
	out := make([]Route, 0, len(collected))
	for _, row := range collected {
		out = append(out, rowToRoute(row))
	}
	return out, nil
}

// UpsertRouteTx inserts or updates a route row by its own id, inside tx.
func (r *Repository) UpsertRouteTx(ctx context.Context, rt *Route, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO fng_routes (`+routeColumns+`)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			hostname = EXCLUDED.hostname, path_prefix = EXCLUDED.path_prefix,
			alias = EXCLUDED.alias, updated_at = EXCLUDED.updated_at`,
		rt.ID, rt.FunctionID, rt.Hostname, rt.PathPrefix, rt.Alias, rt.CreatedBy, rt.CreatedAt, rt.UpdatedAt)
	return err
}

// DeleteRouteTx removes one route, scoped to functionID (defensive: a
// caller can never delete another function's route by id).
func (r *Repository) DeleteRouteTx(ctx context.Context, functionID, id string, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `DELETE FROM fng_routes WHERE id = $1 AND function_id = $2`, id, functionID)
	return err
}

// routeWithAddressRow is ListRoutesByPool's row shape: every fng_routes
// column plus the owning function's address (the control document's Route
// needs the address, not the function id — plan §3/§8.3).
type routeWithAddressRow struct {
	routeRow
	Address string
}

// ListRoutesByPool returns every route owned by a function whose runner
// pool is pool, with each route's owning function address — the exact set
// the control document's Desired.Routes needs for that pool (plan §8: "fill
// Desired.Routes with the routes of the pool's functions").
func (r *Repository) ListRoutesByPool(ctx context.Context, pool string) ([]Route, []string, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT rt.id, rt.function_id, rt.hostname, rt.path_prefix, rt.alias,
		       rt.created_by, rt.created_at, rt.updated_at, f.address AS address
		FROM fng_routes rt
		JOIN fng_functions f ON f.id = rt.function_id
		WHERE COALESCE(f.pool, 'default') = $1
		ORDER BY rt.hostname, rt.path_prefix`, pool)
	if err != nil {
		return nil, nil, err
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[routeWithAddressRow])
	if err != nil {
		return nil, nil, err
	}
	routes := make([]Route, 0, len(collected))
	addresses := make([]string, 0, len(collected))
	for _, row := range collected {
		routes = append(routes, rowToRoute(row.routeRow))
		addresses = append(addresses, row.Address)
	}
	return routes, addresses, nil
}
