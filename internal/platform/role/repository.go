package role

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Tables: iam_roles +
// iam_role_permissions (many-to-many). Permissions are replaced
// wholesale by Persist.
type Repository struct{ q *dbq.Queries }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads a role with hydrated permissions.
func (r *Repository) FindByID(ctx context.Context, id string) (*Role, error) {
	row, err := r.q.RoleFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("role repo: %w", err)
	}
	return r.hydrateOne(ctx, rowToRole(row))
}

// FindByName loads a role by unique name.
func (r *Repository) FindByName(ctx context.Context, name string) (*Role, error) {
	row, err := r.q.RoleFindByName(ctx, name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("role repo: %w", err)
	}
	return r.hydrateOne(ctx, rowToRole(row))
}

// FindAll returns every role with permissions hydrated.
func (r *Repository) FindAll(ctx context.Context) ([]Role, error) {
	rows, err := r.q.RoleFindAll(ctx)
	if err != nil {
		return nil, err
	}
	bare := make([]Role, 0, len(rows))
	for _, row := range rows {
		bare = append(bare, *rowToRole(row))
	}
	return r.hydrateAll(ctx, bare)
}

// Persist implements usecasepgx.Persist[Role]. Replaces the role
// permissions wholesale.
func (r *Repository) Persist(ctx context.Context, role *Role, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.RoleUpsert(ctx, dbq.RoleUpsertParams{
		ID:              role.ID,
		ApplicationID:   role.ApplicationID,
		Name:            role.Name,
		DisplayName:     role.DisplayName,
		Description:     role.Description,
		ApplicationCode: nullIfEmpty(role.ApplicationCode),
		Source:          string(role.Source),
		ClientManaged:   role.ClientManaged,
		CreatedAt:       role.CreatedAt,
		UpdatedAt:       time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("role persist: %w", err)
	}
	if err := q.RolePermissionsClear(ctx, role.ID); err != nil {
		return err
	}
	for _, p := range role.Permissions {
		if err := q.RolePermissionInsert(ctx, dbq.RolePermissionInsertParams{
			RoleID:     role.ID,
			Permission: p,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the role; iam_role_permissions cascades.
func (r *Repository) Delete(ctx context.Context, role *Role, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).RoleDelete(ctx, role.ID)
}

// ── private helpers ───────────────────────────────────────────────────────

func (r *Repository) hydrateOne(ctx context.Context, role *Role) (*Role, error) {
	out, err := r.hydrateAll(ctx, []Role{*role})
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

func (r *Repository) hydrateAll(ctx context.Context, roles []Role) ([]Role, error) {
	if len(roles) == 0 {
		return roles, nil
	}
	ids := make([]string, len(roles))
	for i, r := range roles {
		ids[i] = r.ID
	}
	rows, err := r.q.RolePermissionsForRoles(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string][]string, len(roles))
	for _, row := range rows {
		byID[row.RoleID] = append(byID[row.RoleID], row.Permission)
	}
	for i := range roles {
		perms := byID[roles[i].ID]
		sort.Strings(perms)
		if perms == nil {
			perms = []string{}
		}
		roles[i].Permissions = perms
	}
	return roles, nil
}

func rowToRole(row dbq.IamRole) *Role {
	return &Role{
		ID:              row.ID,
		ApplicationID:   row.ApplicationID,
		Name:            row.Name,
		DisplayName:     row.DisplayName,
		Description:     row.Description,
		ApplicationCode: derefOrEmpty(row.ApplicationCode),
		Source:          ParseSource(row.Source),
		ClientManaged:   row.ClientManaged,
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
		Permissions:     []string{},
	}
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
