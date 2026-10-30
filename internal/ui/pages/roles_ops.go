package pages

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// RolesCommands are the role operations the roles page runs; RolesOps runs the
// use cases the SPA's BFF endpoints run.
type RolesCommands interface {
	Create(ctx context.Context, cmd operations.CreateCommand) (string, error)
	Update(ctx context.Context, cmd operations.UpdateCommand) error
	Delete(ctx context.Context, id string) error
	// SyncPlatform re-runs the platform role sync (the SPA dashboard's "Sync
	// Platform Roles").
	SyncPlatform(ctx context.Context) (RolesSyncResult, error)
}

// RolesSyncResult is the sync's tally (the BFF's sync-platform answer).
type RolesSyncResult struct {
	Created, Updated, Removed, Total int
}

// RolesOps runs the role use cases through the unit of work, exactly as
// internal/platform/shared/bff/roles.go does.
type RolesOps struct {
	Repo *role.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateRole and returns the new role's id.
func (o RolesOps) Create(ctx context.Context, cmd operations.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, operations.CreateRole(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.RoleID, nil
}

// Update runs UpdateRole.
func (o RolesOps) Update(ctx context.Context, cmd operations.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.UpdateRole(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteRole.
func (o RolesOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.DeleteRole(o.Repo), operations.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// SyncPlatform syncs the platform's code-defined roles, removing the ones no
// longer listed.
func (o RolesOps) SyncPlatform(ctx context.Context) (RolesSyncResult, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, operations.SyncPlatformRoles(o.Repo, seed.PlatformRoles()), operations.SyncPlatformRolesCommand{}, auth.NewExecutionContext(ctx))
	if err != nil {
		return RolesSyncResult{}, err
	}
	return RolesSyncResult{Created: int(ev.Created), Updated: int(ev.Updated), Removed: int(ev.Removed), Total: int(ev.Total)}, nil
}

// PermissionCatalogue is the permission catalogue the roles and permissions
// pages read, and the permissions page defines new entries in.
type PermissionCatalogue interface {
	// Catalogue is GET /bff/roles/permissions for an application code ("" for
	// every application).
	Catalogue(ctx context.Context, app string) ([]bff.PermissionEntry, error)
	// Define writes a permission into the catalogue (idempotent by code).
	Define(ctx context.Context, code string, description *string) error
}

// PermissionStore reads the catalogue as the BFF does: the platform builtins,
// the permissions declared by roles and the persistent catalogue table.
type PermissionStore struct {
	Roles   *role.Repository
	Catalog *role.PermissionRepo
}

// Catalogue implements PermissionCatalogue.
func (s PermissionStore) Catalogue(ctx context.Context, app string) ([]bff.PermissionEntry, error) {
	return bff.PermissionCatalog(ctx, s.Roles, s.Catalog, app)
}

// Define implements PermissionCatalogue.
func (s PermissionStore) Define(ctx context.Context, code string, description *string) error {
	if s.Catalog == nil {
		return errors.New("permission catalogue not configured")
	}
	return s.Catalog.Upsert(ctx, role.Permission{Permission: code, Description: description})
}
