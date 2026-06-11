package platformconfig

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo for both Config and Access rows.
// Tables: app_platform_configs + app_platform_config_access.
type Repository struct{ q *dbq.Queries }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads a config row by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Config, error) {
	res, err := r.q.PlatformConfigFindByID(ctx, id)
	row, err := repocommon.One(res, err, "platform_config repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConfig(*row), nil
}

// FindByCoordinate loads a config row by its (app, section, property, scope, clientID) key.
func (r *Repository) FindByCoordinate(ctx context.Context, app, section, property string, scope Scope, clientID *string) (*Config, error) {
	var (
		res dbq.AppPlatformConfig
		err error
	)
	if clientID != nil {
		res, err = r.q.PlatformConfigFindByCoordinateClient(ctx, dbq.PlatformConfigFindByCoordinateClientParams{
			ApplicationCode: app,
			Section:         section,
			Property:        property,
			Scope:           string(scope),
			ClientID:        clientID,
		})
	} else {
		res, err = r.q.PlatformConfigFindByCoordinateAnchor(ctx, dbq.PlatformConfigFindByCoordinateAnchorParams{
			ApplicationCode: app,
			Section:         section,
			Property:        property,
			Scope:           string(scope),
		})
	}
	row, err := repocommon.One(res, err, "platform_config repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToConfig(*row), nil
}

// FindConfigsByApplication returns all configs for an app.
func (r *Repository) FindConfigsByApplication(ctx context.Context, app string) ([]Config, error) {
	rows, err := r.q.PlatformConfigFindByApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	out := make([]Config, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToConfig(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[Config].
func (r *Repository) Persist(ctx context.Context, c *Config, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigUpsert(ctx, dbq.PlatformConfigUpsertParams{
		ID:              c.ID,
		ApplicationCode: c.ApplicationCode,
		Section:         c.Section,
		Property:        c.Property,
		Scope:           string(c.Scope),
		ClientID:        c.ClientID,
		ValueType:       string(c.ValueType),
		Value:           c.Value,
		Description:     c.Description,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       time.Now().UTC(),
	})
}

// Delete implements usecasepgx.Persist[Config].Delete.
func (r *Repository) Delete(ctx context.Context, c *Config, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigDelete(ctx, c.ID)
}

// ── Access repository (sibling table) ─────────────────────────────────────

// FindAccessByID loads an access row.
func (r *Repository) FindAccessByID(ctx context.Context, id string) (*Access, error) {
	res, err := r.q.PlatformConfigAccessFindByID(ctx, id)
	row, err := repocommon.One(res, err, "platform_config_access repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToAccess(*row), nil
}

// FindAccessByRole returns the access row for a (app, role) pair, if any.
func (r *Repository) FindAccessByRole(ctx context.Context, app, role string) (*Access, error) {
	res, err := r.q.PlatformConfigAccessFindByRole(ctx, dbq.PlatformConfigAccessFindByRoleParams{
		ApplicationCode: app, RoleCode: role,
	})
	row, err := repocommon.One(res, err, "platform_config_access repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToAccess(*row), nil
}

// FindAccessByApplication returns all access grants for an app.
func (r *Repository) FindAccessByApplication(ctx context.Context, app string) ([]Access, error) {
	rows, err := r.q.PlatformConfigAccessFindByApplication(ctx, app)
	if err != nil {
		return nil, err
	}
	out := make([]Access, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToAccess(row))
	}
	return out, nil
}

// PersistAccess persists an access row.
func (r *Repository) PersistAccess(ctx context.Context, a *Access, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigAccessUpsert(ctx, dbq.PlatformConfigAccessUpsertParams{
		ID:              a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
		CanRead:         a.CanRead,
		CanWrite:        a.CanWrite,
		CreatedAt:       a.CreatedAt,
	})
}

// DeleteAccess removes an access row.
func (r *Repository) DeleteAccess(ctx context.Context, a *Access, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).PlatformConfigAccessDelete(ctx, a.ID)
}

// HasAccess reports whether the caller (described by app + role set) may
// perform the requested operation. wantWrite=true requires can_write.
func (r *Repository) HasAccess(ctx context.Context, app string, roleCodes []string, wantWrite bool) (bool, error) {
	if len(roleCodes) == 0 {
		return false, nil
	}
	if wantWrite {
		return r.q.PlatformConfigAccessHasWriteByRoles(ctx, dbq.PlatformConfigAccessHasWriteByRolesParams{
			ApplicationCode: app, RoleCodes: roleCodes,
		})
	}
	return r.q.PlatformConfigAccessHasReadByRoles(ctx, dbq.PlatformConfigAccessHasReadByRolesParams{
		ApplicationCode: app, RoleCodes: roleCodes,
	})
}

func rowToConfig(row dbq.AppPlatformConfig) *Config {
	return &Config{
		ID:              row.ID,
		ApplicationCode: row.ApplicationCode,
		Section:         row.Section,
		Property:        row.Property,
		Scope:           ParseScope(row.Scope),
		ClientID:        row.ClientID,
		ValueType:       ParseValueType(row.ValueType),
		Value:           row.Value,
		Description:     row.Description,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}

func rowToAccess(row dbq.AppPlatformConfigAccess) *Access {
	return &Access{
		ID:              row.ID,
		ApplicationCode: row.ApplicationCode,
		RoleCode:        row.RoleCode,
		CanRead:         row.CanRead,
		CanWrite:        row.CanWrite,
		CreatedAt:       row.CreatedAt,
	}
}
