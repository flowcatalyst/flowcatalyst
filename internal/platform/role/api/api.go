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
	huma.Register(api, huma.Operation{
		OperationID:   "listRoles",
		Method:        http.MethodGet,
		Path:          "/api/roles",
		Summary:       "List roles",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createRole",
		Method:        http.MethodPost,
		Path:          "/api/roles",
		Summary:       "Create a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getRole",
		Method:        http.MethodGet,
		Path:          "/api/roles/{id}",
		Summary:       "Get a role by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateRole",
		Method:        http.MethodPut,
		Path:          "/api/roles/{id}",
		Summary:       "Update a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteRole",
		Method:        http.MethodDelete,
		Path:          "/api/roles/{id}",
		Summary:       "Delete a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	// Role lookups by code/source/application + filter options.
	huma.Register(api, huma.Operation{
		OperationID:   "getRoleByCode",
		Method:        http.MethodGet,
		Path:          "/api/roles/by-code/{code}",
		Summary:       "Get a role by name (code)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byCode)

	huma.Register(api, huma.Operation{
		OperationID:   "getRolesBySource",
		Method:        http.MethodGet,
		Path:          "/api/roles/by-source/{source}",
		Summary:       "List roles by source (CODE | DATABASE | SDK)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.bySource)

	huma.Register(api, huma.Operation{
		OperationID:   "getRolesByApplication",
		Method:        http.MethodGet,
		Path:          "/api/roles/by-application/{applicationId}",
		Summary:       "List roles for an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byApplication)

	huma.Register(api, huma.Operation{
		OperationID:   "getRoleApplicationFilters",
		Method:        http.MethodGet,
		Path:          "/api/roles/filters/applications",
		Summary:       "List distinct application codes used by roles",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.applicationFilters)

	// Permission grants on a role.
	huma.Register(api, huma.Operation{
		OperationID:   "listRolePermissions",
		Method:        http.MethodGet,
		Path:          "/api/roles/{roleName}/permissions",
		Summary:       "List permissions granted to a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listRolePermissions)

	huma.Register(api, huma.Operation{
		OperationID:   "grantRolePermission",
		Method:        http.MethodPost,
		Path:          "/api/roles/{roleName}/permissions/{permission}",
		Summary:       "Grant a permission to a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.grantPermission)

	// SDK-compatibility alias: the Laravel/Rust client grants a permission by
	// POSTing {permission} in the body to /permissions (rather than naming it
	// in the path). Same grant operation.
	huma.Register(api, huma.Operation{
		OperationID:   "grantRolePermissionByBody",
		Method:        http.MethodPost,
		Path:          "/api/roles/{roleName}/permissions",
		Summary:       "Grant a permission to a role (SDK; permission in body)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.grantPermissionByBody)

	huma.Register(api, huma.Operation{
		OperationID:   "revokeRolePermission",
		Method:        http.MethodDelete,
		Path:          "/api/roles/{roleName}/permissions/{permission}",
		Summary:       "Revoke a permission from a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.revokePermission)

	// Permission catalog.
	huma.Register(api, huma.Operation{
		OperationID:   "listPermissions",
		Method:        http.MethodGet,
		Path:          "/api/roles/permissions",
		Summary:       "List the platform permission catalog",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listPermissions)

	huma.Register(api, huma.Operation{
		OperationID:   "getPermission",
		Method:        http.MethodGet,
		Path:          "/api/roles/permissions/{permission}",
		Summary:       "Get a single permission catalog entry",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getPermission)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePermission",
		Method:        http.MethodDelete,
		Path:          "/api/roles/permissions/{permission}",
		Summary:       "Delete a permission from the catalog",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.deletePermission)
}

// ── Handlers ──────────────────────────────────────────────────────────

type emptyInput struct{}

type listOutput struct {
	Body RoleListResponse
}

// roleArrayOutput renders a bare JSON array `[...]`, the Rust shape for
// /api/roles/by-application/{id} and /api/roles/by-source/{source}.
type roleArrayOutput struct {
	Body []RoleResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]RoleResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: RoleListResponse{Roles: out, Total: len(out)}}, nil
}

type getInput struct {
	ID string `path:"id" doc:"Role id (TSID)"`
}

type getOutput struct {
	Body RoleResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	r, err := s.resolveRole(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &getOutput{Body: fromEntity(r)}, nil
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

type createInput struct {
	Body CreateRoleRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteRoles(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateRole(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().RoleID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateRoleRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
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
	return &emptyOutput{}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
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
	return &emptyOutput{}, nil
}

// ── by-code / by-source / by-application / filters ─────────────────────

type byCodeInput struct {
	Code string `path:"code"`
}

func (s *State) byCode(ctx context.Context, in *byCodeInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(r)}, nil
}

type bySourceInput struct {
	Source string `path:"source"`
}

func (s *State) bySource(ctx context.Context, in *bySourceInput) (*roleArrayOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindBySource(ctx, role.ParseSource(in.Source))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_source failed", err)
	}
	out := make([]RoleResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &roleArrayOutput{Body: out}, nil
}

type byApplicationInput struct {
	ApplicationID string `path:"applicationId"`
}

func (s *State) byApplication(ctx context.Context, in *byApplicationInput) (*roleArrayOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindByApplicationID(ctx, in.ApplicationID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_application_id failed", err)
	}
	out := make([]RoleResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &roleArrayOutput{Body: out}, nil
}

type applicationFiltersOutput struct {
	Body ApplicationFilterListResponse
}

func (s *State) applicationFilters(ctx context.Context, _ *emptyInput) (*applicationFiltersOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	codes, err := s.Repo.ApplicationCodes(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "application_codes failed", err)
	}
	return &applicationFiltersOutput{Body: ApplicationFilterListResponse{ApplicationCodes: codes}}, nil
}

// ── per-role permission grants ──────────────────────────────────────────

type roleNameInput struct {
	RoleName string `path:"roleName"`
}

type rolePermissionListOutput struct {
	Body RolePermissionListResponse
}

func (s *State) listRolePermissions(ctx context.Context, in *roleNameInput) (*rolePermissionListOutput, error) {
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
	return &rolePermissionListOutput{Body: RolePermissionListResponse{Permissions: perms}}, nil
}

type rolePermissionGrantInput struct {
	RoleName   string `path:"roleName"`
	Permission string `path:"permission"`
}

func (s *State) grantPermission(ctx context.Context, in *rolePermissionGrantInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(r)}, nil
}

type rolePermissionGrantBodyInput struct {
	RoleName string `path:"roleName"`
	Body     GrantPermissionRequest
}

// grantPermissionByBody backs POST /api/roles/{roleName}/permissions with the
// permission in the body (SDK shape). Delegates to the same grant operation as
// the path-param variant.
func (s *State) grantPermissionByBody(ctx context.Context, in *rolePermissionGrantBodyInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(r)}, nil
}

func (s *State) revokePermission(ctx context.Context, in *rolePermissionGrantInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(r)}, nil
}

// ── permission catalog ──────────────────────────────────────────────────

type permissionListOutput struct {
	Body PermissionListResponse
}

func (s *State) listPermissions(ctx context.Context, _ *emptyInput) (*permissionListOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadRoles(ac); err != nil {
		return nil, err
	}
	if s.Permissions == nil {
		return &permissionListOutput{Body: PermissionListResponse{Permissions: []PermissionResponse{}, Total: 0}}, nil
	}
	rows, err := s.Permissions.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "permission_find_all failed", err)
	}
	out := make([]PermissionResponse, 0, len(rows))
	for i := range rows {
		out = append(out, permissionToResponse(&rows[i]))
	}
	return &permissionListOutput{Body: PermissionListResponse{Permissions: out, Total: len(out)}}, nil
}

type permissionPathInput struct {
	Permission string `path:"permission"`
}

type permissionOutput struct {
	Body PermissionResponse
}

func (s *State) getPermission(ctx context.Context, in *permissionPathInput) (*permissionOutput, error) {
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
	return &permissionOutput{Body: permissionToResponse(p)}, nil
}

func (s *State) deletePermission(ctx context.Context, in *permissionPathInput) (*emptyOutput, error) {
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
	return &emptyOutput{}, nil
}
