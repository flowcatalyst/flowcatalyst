// Package api wires the HTTP routes for the role subdomain via
// danielgtaylor/huma/v2.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps for the role handlers. Permissions is the catalog
// repo (iam_permissions); leave nil if catalog endpoints aren't wired.
type State struct {
	Repo        *role.Repository
	Permissions *role.PermissionRepo
	UoW         *usecasepgx.UnitOfWork
}

const tag = "roles"

// Register mounts the role endpoints on the supplied huma API.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listRoles", "/api/roles", "List roles", s.list)
	apiroute.Post(g, "createRole", "/api/roles", "Create a role", http.StatusCreated, s.create)
	apiroute.Get(g, "getRole", "/api/roles/{id}", "Get a role by id", s.getByID)
	apiroute.Put(g, "updateRole", "/api/roles/{id}", "Update a role", http.StatusNoContent, s.update)
	apiroute.Delete(g, "deleteRole", "/api/roles/{id}", "Delete a role", http.StatusNoContent, s.delete)
	// Role lookups by code/source/application + filter options.
	apiroute.Get(g, "getRoleByCode", "/api/roles/by-code/{code}", "Get a role by name (code)", s.byCode)
	apiroute.Get(g, "getRolesBySource", "/api/roles/by-source/{source}", "List roles by source (CODE | DATABASE | SDK)", s.bySource)
	apiroute.Get(g, "getRolesByApplication", "/api/roles/by-application/{applicationId}", "List roles for an application", s.byApplication)
	apiroute.Get(g, "getRoleApplicationFilters", "/api/roles/filters/applications", "List distinct application codes used by roles", s.applicationFilters)
	// Permission grants on a role.
	apiroute.Get(g, "listRolePermissions", "/api/roles/{roleName}/permissions", "List permissions granted to a role", s.listRolePermissions)
	apiroute.Post(g, "grantRolePermission", "/api/roles/{roleName}/permissions/{permission}", "Grant a permission to a role", http.StatusOK, s.grantPermission)
	// SDK-compatibility alias: the Laravel/Rust client grants a permission by
	// POSTing {permission} in the body to /permissions (rather than naming it
	// in the path). Same grant operation.
	apiroute.Post(g, "grantRolePermissionByBody", "/api/roles/{roleName}/permissions", "Grant a permission to a role (SDK; permission in body)", http.StatusOK, s.grantPermissionByBody)
	apiroute.Delete(g, "revokeRolePermission", "/api/roles/{roleName}/permissions/{permission}", "Revoke a permission from a role", http.StatusOK, s.revokePermission)
	// Permission catalog.
	apiroute.Get(g, "listPermissions", "/api/roles/permissions", "List the platform permission catalog", s.listPermissions)
	apiroute.Get(g, "getPermission", "/api/roles/permissions/{permission}", "Get a single permission catalog entry", s.getPermission)
	apiroute.Delete(g, "deletePermission", "/api/roles/permissions/{permission}", "Delete a permission from the catalog", http.StatusNoContent, s.deletePermission)
}

// ── Handlers ──────────────────────────────────────────────────────────

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[RoleListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[RoleListResponse]{Body: RoleListResponse{Roles: out, Total: len(out)}}, nil
}

type getInput struct {
	ID string `path:"id" doc:"Role id (TSID)"`
}

func (s *State) getByID(ctx context.Context, in *getInput) (*apicommon.Out[RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[RoleResponse]{Body: fromEntity(r)}, nil
}

// resolveRole loads a role by TSID id, falling back to its name. The SPA
// addresses roles by id; the Laravel/Rust SDK addresses them by name on the
// same /api/roles/{…} routes — so the {id} handlers accept either. TSIDs and
// role names don't overlap, so the fallback is unambiguous.
func (s *State) resolveRole(ctx context.Context, idOrName string) (*role.Role, error) {
	r, err := s.Repo.FindByID(ctx, idOrName)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if r == nil {
		r, err = s.Repo.FindByName(ctx, idOrName)
		if err != nil {
			return nil, usecase.Internal("REPO", "find_by_name failed", err)
		}
	}
	if r == nil {
		return nil, httperror.NotFound("Role", idOrName)
	}
	return r, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateRoleRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateRole(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().RoleID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateRoleRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateRole(ctx, s.Repo, s.UoW, in.Body.toCommand(r.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteRole(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: r.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── by-code / by-source / by-application / filters ─────────────────────

type byCodeInput struct {
	Code string `path:"code"`
}

func (s *State) byCode(ctx context.Context, in *byCodeInput) (*apicommon.Out[RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.Repo.FindByName(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_name failed", err)
	}
	if r == nil {
		return nil, httperror.NotFound("Role", in.Code)
	}
	return &apicommon.Out[RoleResponse]{Body: fromEntity(r)}, nil
}

type bySourceInput struct {
	Source string `path:"source"`
}

// Out[[]RoleResponse] renders a bare JSON array `[...]`, the Rust shape
// for /api/roles/by-application/{id} and /api/roles/by-source/{source}.
func (s *State) bySource(ctx context.Context, in *bySourceInput) (*apicommon.Out[[]RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindBySource(ctx, role.ParseSource(in.Source))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_source failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[[]RoleResponse]{Body: out}, nil
}

type byApplicationInput struct {
	ApplicationID string `path:"applicationId"`
}

func (s *State) byApplication(ctx context.Context, in *byApplicationInput) (*apicommon.Out[[]RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindByApplicationID(ctx, in.ApplicationID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_application_id failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[[]RoleResponse]{Body: out}, nil
}

func (s *State) applicationFilters(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[ApplicationFilterListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	codes, err := s.Repo.ApplicationCodes(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "application_codes failed", err)
	}
	return &apicommon.Out[ApplicationFilterListResponse]{Body: ApplicationFilterListResponse{ApplicationCodes: codes}}, nil
}

// ── per-role permission grants ──────────────────────────────────────────

type roleNameInput struct {
	RoleName string `path:"roleName"`
}

func (s *State) listRolePermissions(ctx context.Context, in *roleNameInput) (*apicommon.Out[RolePermissionListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.Repo.FindByName(ctx, in.RoleName)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_name failed", err)
	}
	if r == nil {
		return nil, httperror.NotFound("Role", in.RoleName)
	}
	perms := append([]string(nil), r.Permissions...)
	return &apicommon.Out[RolePermissionListResponse]{Body: RolePermissionListResponse{Permissions: perms}}, nil
}

type rolePermissionGrantInput struct {
	RoleName   string `path:"roleName"`
	Permission string `path:"permission"`
}

func (s *State) grantPermission(ctx context.Context, in *rolePermissionGrantInput) (*apicommon.Out[RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.GrantPermission(ctx, s.Repo, s.UoW, operations.GrantPermissionCommand{
		RoleName: in.RoleName, Permission: in.Permission,
	}, ec); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.RoleName)
	if err != nil {
		return nil, err
	}
	// Return the updated role (1:1 with Rust grant_permission → RoleResponse).
	return &apicommon.Out[RoleResponse]{Body: fromEntity(r)}, nil
}

type rolePermissionGrantBodyInput struct {
	RoleName string `path:"roleName"`
	Body     GrantPermissionRequest
}

// grantPermissionByBody backs POST /api/roles/{roleName}/permissions with the
// permission in the body (SDK shape). Delegates to the same grant operation as
// the path-param variant.
func (s *State) grantPermissionByBody(ctx context.Context, in *rolePermissionGrantBodyInput) (*apicommon.Out[RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.GrantPermission(ctx, s.Repo, s.UoW, operations.GrantPermissionCommand{
		RoleName: in.RoleName, Permission: in.Body.Permission,
	}, ec); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.RoleName)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[RoleResponse]{Body: fromEntity(r)}, nil
}

func (s *State) revokePermission(ctx context.Context, in *rolePermissionGrantInput) (*apicommon.Out[RoleResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RevokePermission(ctx, s.Repo, s.UoW, operations.RevokePermissionCommand{
		RoleName: in.RoleName, Permission: in.Permission,
	}, ec); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.RoleName)
	if err != nil {
		return nil, err
	}
	// Return the updated role (1:1 with Rust revoke_permission → RoleResponse).
	return &apicommon.Out[RoleResponse]{Body: fromEntity(r)}, nil
}

// ── permission catalog ──────────────────────────────────────────────────

func (s *State) listPermissions(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[PermissionListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	if s.Permissions == nil {
		return &apicommon.Out[PermissionListResponse]{Body: PermissionListResponse{Permissions: []PermissionResponse{}, Total: 0}}, nil
	}
	rows, err := s.Permissions.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "permission_find_all failed", err)
	}
	out := apicommon.MapSlice(rows, permissionToResponse)
	return &apicommon.Out[PermissionListResponse]{Body: PermissionListResponse{Permissions: out, Total: len(out)}}, nil
}

type permissionPathInput struct {
	Permission string `path:"permission"`
}

func (s *State) getPermission(ctx context.Context, in *permissionPathInput) (*apicommon.Out[PermissionResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	if s.Permissions == nil {
		return nil, httperror.NotFound("Permission", in.Permission)
	}
	p, err := s.Permissions.FindByCode(ctx, in.Permission)
	if err != nil {
		return nil, usecase.Internal("REPO", "permission_find_by_code failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Permission", in.Permission)
	}
	return &apicommon.Out[PermissionResponse]{Body: permissionToResponse(p)}, nil
}

func (s *State) deletePermission(ctx context.Context, in *permissionPathInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteRoles(ac); err != nil {
		return nil, err
	}
	if s.Permissions == nil {
		return nil, httperror.NotFound("Permission", in.Permission)
	}
	if err := s.Permissions.DeleteByCode(ctx, in.Permission); err != nil {
		return nil, usecase.Internal("REPO", "permission_delete failed", err)
	}
	return &apicommon.Empty{}, nil
}
