package bff

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RolesState holds the deps the BFF role endpoints reach into.
//
// Applications is optional — when nil, /filters/applications returns
// an empty options array (matches Rust's behaviour when the
// application_repo is not wired).
type RolesState struct {
	Roles        *role.Repository
	Applications *application.Repository
	// Permissions is the writable permission catalogue (iam_permissions). When
	// nil, the catalogue endpoints fall back to builtins + role-derived
	// permissions only and POST /permissions is unavailable.
	Permissions *role.PermissionRepo
	UoW         *usecasepgx.UnitOfWork
}

// RegisterRoles mounts the dashboard's `/bff/roles/*` endpoints.
//
// Mirrors `crates/fc-platform/src/shared/bff_roles_api.rs`. Response
// shapes match Rust's BffRoleResponse / BffRoleListResponse /
// BffApplicationOptionsResponse / BffPermissionListResponse exactly:
// camelCase fields, ISO-8601 timestamps as strings, items wrapped in
// `{items, total}`.
func RegisterRoles(r chi.Router, s *RolesState) {
	r.Route("/bff/roles", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Post("/sync-platform", s.syncPlatform)
		r.Get("/filters/applications", s.filterApplications)
		r.Get("/permissions", s.listPermissions)
		r.Post("/permissions", s.createPermission)
		r.Get("/permissions/{permission}", s.getPermission)
		r.Get("/{roleName}", s.get)
		r.Put("/{roleName}", s.update)
		r.Delete("/{roleName}", s.delete)
	})
}

// ── Wire DTOs ────────────────────────────────────────────────────────────

// bffRoleResponse matches Rust's BffRoleResponse exactly. Fields are
// camelCase per the frontend's expectation.
type bffRoleResponse struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	ShortName       string   `json:"shortName"`
	DisplayName     string   `json:"displayName"`
	Description     *string  `json:"description,omitempty"`
	Permissions     []string `json:"permissions"`
	ApplicationCode string   `json:"applicationCode"`
	Source          string   `json:"source"`
	ClientManaged   bool     `json:"clientManaged"`
	CreatedAt       string   `json:"createdAt"`
	UpdatedAt       string   `json:"updatedAt"`
}

type bffRoleListResponse struct {
	Items []bffRoleResponse `json:"items"`
	Total int               `json:"total"`
}

type bffApplicationOption struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

type bffApplicationOptionsResponse struct {
	Options []bffApplicationOption `json:"options"`
}

type bffPermissionResponse struct {
	Permission  string `json:"permission"`
	Application string `json:"application"`
	Context     string `json:"context"`
	Aggregate   string `json:"aggregate"`
	Action      string `json:"action"`
	Description string `json:"description"`
}

type bffPermissionListResponse struct {
	Items []bffPermissionResponse `json:"items"`
	Total int                     `json:"total"`
}

type bffCreateRoleRequest struct {
	ApplicationCode string   `json:"applicationCode"`
	RoleName        string   `json:"roleName"`
	DisplayName     string   `json:"displayName"`
	Description     *string  `json:"description,omitempty"`
	Permissions     []string `json:"permissions"`
	ClientManaged   bool     `json:"clientManaged"`
}

type bffUpdateRoleRequest struct {
	DisplayName   *string   `json:"displayName,omitempty"`
	Description   *string   `json:"description,omitempty"`
	ClientManaged *bool     `json:"clientManaged,omitempty"`
	Permissions   *[]string `json:"permissions,omitempty"`
}

type createdResponse struct {
	ID string `json:"id"`
}

// syncPlatformRolesResponse matches Rust's SyncPlatformRolesResponse.
type syncPlatformRolesResponse struct {
	Created uint32 `json:"created"`
	Updated uint32 `json:"updated"`
	Removed uint32 `json:"removed"`
	Total   uint32 `json:"total"`
}

// ── Handlers ─────────────────────────────────────────────────────────────

// GET /bff/roles?application=&source=
//
// Filters are applied in-memory after FindAll. Cheap given typical
// row counts (<100). Repo doesn't expose FindByApplication /
// FindBySource yet — when those land, swap to dispatch.
func (s *RolesState) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	applicationFilter := q.Get("application")
	sourceFilter := q.Get("source")

	rows, err := s.Roles.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list roles failed", err))
		return
	}
	out := make([]bffRoleResponse, 0, len(rows))
	for _, role := range rows {
		if applicationFilter != "" && role.ApplicationCode != applicationFilter {
			continue
		}
		if sourceFilter != "" && !strings.EqualFold(string(role.Source), sourceFilter) {
			continue
		}
		out = append(out, toBffRole(role))
	}
	writeJSON(w, http.StatusOK, bffRoleListResponse{Items: out, Total: len(out)})
}

// GET /bff/roles/filters/applications
//
// Active applications only — mirrors Rust's application_repo.find_active().
func (s *RolesState) filterApplications(w http.ResponseWriter, r *http.Request) {
	options := []bffApplicationOption{}
	if s.Applications != nil {
		active := "true"
		apps, err := s.Applications.FindWithFilters(r.Context(), nil, &active)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "list applications failed", err))
			return
		}
		for _, a := range apps {
			options = append(options, bffApplicationOption{
				ID:   a.ID,
				Code: a.Code,
				Name: a.Name,
			})
		}
	}
	writeJSON(w, http.StatusOK, bffApplicationOptionsResponse{Options: options})
}

// GET /bff/roles/permissions?application=
//
// Without an application filter, returns the full catalogue (the static
// platform builtins plus every permission declared across non-platform roles)
// — this powers the global Permissions page. With ?application=platform it
// returns the builtins; with any other code it returns just that
// application's permissions, sourced from its roles. A non-platform
// application has no catalogue of its own — its permissions live only inside
// its roles — so editing/creating one of its roles must scope here by code
// rather than fall back to the platform builtins.
func (s *RolesState) listPermissions(w http.ResponseWriter, r *http.Request) {
	perms, err := s.permissionCatalog(r.Context(), r.URL.Query().Get("application"))
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list permissions failed", err))
		return
	}
	writeJSON(w, http.StatusOK, bffPermissionListResponse{Items: perms, Total: len(perms)})
}

// GET /bff/roles/permissions/{permission}
func (s *RolesState) getPermission(w http.ResponseWriter, r *http.Request) {
	wanted := chi.URLParam(r, "permission")
	catalog, err := s.permissionCatalog(r.Context(), "")
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "lookup permission failed", err))
		return
	}
	for _, p := range catalog {
		if p.Permission == wanted {
			writeJSON(w, http.StatusOK, p)
			return
		}
	}
	httperror.Write(w, httperror.NotFound("Permission", wanted))
}

// permissionCatalog returns the permission catalogue for an application code:
//   - ""         → every application: the platform builtins plus the distinct
//     permissions declared across all non-platform roles.
//   - "platform" → the static platform builtins.
//   - other code → the distinct permissions declared across that application's
//     roles (the app has no catalogue of its own).
func (s *RolesState) permissionCatalog(ctx context.Context, app string) ([]bffPermissionResponse, error) {
	var base []bffPermissionResponse
	switch app {
	case "platform":
		base = builtinPermissions()
	case "":
		derived, err := s.rolePermissions(ctx, "")
		if err != nil {
			return nil, err
		}
		base = append(builtinPermissions(), derived...)
	default:
		derived, err := s.rolePermissions(ctx, app)
		if err != nil {
			return nil, err
		}
		base = derived
	}
	// Merge in the persistent catalogue (iam_permissions), scoped to the same
	// application filter and deduped by code. Catalogue rows survive SDK role
	// re-syncs, so a permission created via POST /permissions stays available
	// even when no role references it yet.
	catalog, err := s.catalogPermissions(ctx, app)
	if err != nil {
		return nil, err
	}
	return mergePermissions(base, catalog), nil
}

// catalogPermissions reads the persistent permission catalogue
// (iam_permissions), mapping each row to the BFF shape via its four-segment
// code and optionally scoping to one application. Returns nil when no
// catalogue repo is wired.
func (s *RolesState) catalogPermissions(ctx context.Context, app string) ([]bffPermissionResponse, error) {
	if s.Permissions == nil {
		return nil, nil
	}
	rows, err := s.Permissions.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := []bffPermissionResponse{}
	for _, p := range rows {
		entry, ok := parsePermission(p.Permission)
		if !ok {
			continue
		}
		if app != "" && entry.Application != app {
			continue
		}
		if p.Description != nil {
			entry.Description = *p.Description
		}
		out = append(out, entry)
	}
	return out, nil
}

// mergePermissions concatenates two permission lists, dropping later entries
// whose code already appeared (first occurrence wins, so builtins/role-derived
// descriptions take precedence over catalogue duplicates).
func mergePermissions(base, extra []bffPermissionResponse) []bffPermissionResponse {
	seen := make(map[string]struct{}, len(base)+len(extra))
	out := make([]bffPermissionResponse, 0, len(base)+len(extra))
	for _, p := range base {
		if _, ok := seen[p.Permission]; ok {
			continue
		}
		seen[p.Permission] = struct{}{}
		out = append(out, p)
	}
	for _, p := range extra {
		if _, ok := seen[p.Permission]; ok {
			continue
		}
		seen[p.Permission] = struct{}{}
		out = append(out, p)
	}
	return out
}

// permSegment bounds each part of a permission code to a lowercase token so the
// assembled "application:context:aggregate:action" string is well-formed. Kept
// in sync with the frontend's add-permission validation.
var permSegment = regexp.MustCompile(`^[a-z0-9-]+$`)

type bffCreatePermissionRequest struct {
	Application string `json:"application"`
	Context     string `json:"context"`
	Aggregate   string `json:"aggregate"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

// POST /bff/roles/permissions
//
// Creates (or idempotently updates) a permission in the persistent catalogue.
// Anchor-gated, matching role creation: anyone who can manage roles can define
// permissions. The four segments form the canonical code
// "application:context:aggregate:action".
func (s *RolesState) createPermission(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	if s.Permissions == nil {
		httperror.Write(w, usecase.Internal("CONFIG", "permission catalogue not configured", nil))
		return
	}
	var body bffCreatePermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	app := strings.TrimSpace(body.Application)
	cx := strings.TrimSpace(body.Context)
	agg := strings.TrimSpace(body.Aggregate)
	act := strings.TrimSpace(body.Action)
	for _, seg := range []string{app, cx, agg, act} {
		if !permSegment.MatchString(seg) {
			httperror.Write(w, httperror.BadRequest("INVALID_PERMISSION",
				"application, context, aggregate and action must each be lowercase letters, numbers or hyphens"))
			return
		}
	}
	code := app + ":" + cx + ":" + agg + ":" + act
	var desc *string
	if d := strings.TrimSpace(body.Description); d != "" {
		desc = &d
	}
	if err := s.Permissions.Upsert(r.Context(), role.Permission{Permission: code, Description: desc}); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "create permission failed", err))
		return
	}
	entry, _ := parsePermission(code)
	if desc != nil {
		entry.Description = *desc
	}
	writeJSON(w, http.StatusCreated, entry)
}

// rolePermissions collects the distinct, well-formed (4-segment) permissions
// declared across roles, optionally restricted to one application code.
// Platform-coded roles are skipped — the platform catalogue is the static
// builtin set — so the all-applications case can append role-derived
// permissions to the builtins without duplicating platform entries.
func (s *RolesState) rolePermissions(ctx context.Context, appCode string) ([]bffPermissionResponse, error) {
	rows, err := s.Roles.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := []bffPermissionResponse{}
	for _, role := range rows {
		if role.ApplicationCode == "platform" {
			continue
		}
		if appCode != "" && role.ApplicationCode != appCode {
			continue
		}
		for _, p := range role.Permissions {
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			entry, ok := parsePermission(p)
			if !ok {
				continue
			}
			if appCode != "" && entry.Application != appCode {
				continue
			}
			out = append(out, entry)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Permission < out[j].Permission })
	return out, nil
}

// parsePermission splits a 4-segment permission string
// (application:context:aggregate:action) into a catalogue entry. Returns
// ok=false for malformed strings (any arity other than four segments, e.g. a
// differently-shaped wildcard), which are then skipped.
func parsePermission(p string) (bffPermissionResponse, bool) {
	parts := strings.Split(p, ":")
	if len(parts) != 4 {
		return bffPermissionResponse{}, false
	}
	return bffPermissionResponse{
		Permission:  p,
		Application: parts[0],
		Context:     parts[1],
		Aggregate:   parts[2],
		Action:      parts[3],
	}, true
}

// GET /bff/roles/{roleName}
//
// Names contain a colon ("platform:admin"); ids don't. Frontend uses
// either interchangeably.
func (s *RolesState) get(w http.ResponseWriter, r *http.Request) {
	role, err := s.resolveRole(r, chi.URLParam(r, "roleName"))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toBffRole(*role))
}

// POST /bff/roles
func (s *RolesState) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	var body bffCreateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	cmd := operations.CreateCommand{
		ApplicationCode: body.ApplicationCode,
		RoleName:        body.RoleName,
		DisplayName:     body.DisplayName,
		Description:     body.Description,
		Permissions:     body.Permissions,
		ClientManaged:   body.ClientManaged,
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecaseop.Run(r.Context(), s.UoW, operations.CreateRole(s.Roles), cmd, ec)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, createdResponse{ID: event.RoleID})
}

// PUT /bff/roles/{roleName}
func (s *RolesState) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	var body bffUpdateRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	role, err := s.resolveRole(r, chi.URLParam(r, "roleName"))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	cmd := operations.UpdateCommand{
		ID:            role.ID,
		DisplayName:   body.DisplayName,
		Description:   body.Description,
		ClientManaged: body.ClientManaged,
	}
	if body.Permissions != nil {
		cmd.Permissions = *body.Permissions
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(r.Context(), s.UoW, operations.UpdateRole(s.Roles), cmd, ec); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /bff/roles/sync-platform
//
// Re-runs the code-defined role sync. Mirrors Rust's
// RoleSyncService::sync_code_defined_roles via the new SyncPlatformRoles
// use case: per-row Created / Updated / Deleted events fire alongside
// the RolesSynced rollup, all in one transaction. Anchor-only.
func (s *RolesState) syncPlatform(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	ev, err := usecaseop.Run(r.Context(), s.UoW, operations.SyncPlatformRoles(s.Roles, seed.PlatformRoles()), operations.SyncPlatformRolesCommand{}, ec)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, syncPlatformRolesResponse{
		Created: ev.Created,
		Updated: ev.Updated,
		Removed: ev.Removed,
		Total:   ev.Total,
	})
}

// DELETE /bff/roles/{roleName}
func (s *RolesState) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	role, err := s.resolveRole(r, chi.URLParam(r, "roleName"))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	cmd := operations.DeleteCommand{ID: role.ID}
	if _, err := usecaseop.Run(r.Context(), s.UoW, operations.DeleteRole(s.Roles), cmd, ec); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Helpers ──────────────────────────────────────────────────────────────

// resolveRole dispatches between FindByName (colon-containing) and
// FindByID. Returns a typed NotFound error when missing.
func (s *RolesState) resolveRole(r *http.Request, key string) (*role.Role, error) {
	// chi hands back the raw path segment, so a role identifier's ':' arrives
	// percent-encoded as %3A (role ids are "{appCode}:{roleName}", e.g.
	// "logistics_portal:administrator"). Decode it before dispatching, or the
	// colon check below fails and FindByID is tried with a bogus encoded key —
	// surfacing as "Role not found: logistics_portal%3Aadministrator".
	if decoded, derr := url.PathUnescape(key); derr == nil {
		key = decoded
	}
	var (
		out *role.Role
		err error
	)
	if strings.Contains(key, ":") {
		out, err = s.Roles.FindByName(r.Context(), key)
	} else {
		out, err = s.Roles.FindByID(r.Context(), key)
	}
	if err != nil {
		return nil, usecase.Internal("REPO", "find role failed", err)
	}
	if out == nil {
		return nil, httperror.NotFound("Role", key)
	}
	return out, nil
}

func toBffRole(r role.Role) bffRoleResponse {
	shortName := r.Name
	if idx := strings.LastIndex(r.Name, ":"); idx >= 0 && idx+1 < len(r.Name) {
		shortName = r.Name[idx+1:]
	}
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return bffRoleResponse{
		ID:              r.ID,
		Name:            r.Name,
		ShortName:       shortName,
		DisplayName:     r.DisplayName,
		Description:     r.Description,
		Permissions:     perms,
		ApplicationCode: r.ApplicationCode,
		Source:          string(r.Source),
		ClientManaged:   r.ClientManaged,
		CreatedAt:       r.CreatedAt.Format("2006-01-02T15:04:05.000000Z07:00"),
		UpdatedAt:       r.UpdatedAt.Format("2006-01-02T15:04:05.000000Z07:00"),
	}
}

// ── Permissions registry ─────────────────────────────────────────────────

// builtinPermissions ports `get_builtin_permissions` from
// bff_roles_api.rs. The catalog is static — the dashboard renders it
// directly. Must stay in lockstep with `internal/platform/seed/permissions.go`
// (the actual permission strings the platform recognises) until the
// two are merged.
func builtinPermissions() []bffPermissionResponse {
	out := []bffPermissionResponse{}
	// IAM
	out = appendPerm(out, "platform", "iam", "user", "view", "View users")
	out = appendPerm(out, "platform", "iam", "user", "create", "Create users")
	out = appendPerm(out, "platform", "iam", "user", "update", "Update users")
	out = appendPerm(out, "platform", "iam", "user", "delete", "Delete users")
	out = appendPerm(out, "platform", "iam", "role", "view", "View roles")
	out = appendPerm(out, "platform", "iam", "role", "create", "Create roles")
	out = appendPerm(out, "platform", "iam", "role", "update", "Update roles")
	out = appendPerm(out, "platform", "iam", "role", "delete", "Delete roles")
	out = appendPerm(out, "platform", "iam", "permission", "view", "View permissions")
	out = appendPerm(out, "platform", "iam", "service-account", "view", "View service accounts")
	out = appendPerm(out, "platform", "iam", "service-account", "create", "Create service accounts")
	out = appendPerm(out, "platform", "iam", "service-account", "update", "Update service accounts")
	out = appendPerm(out, "platform", "iam", "service-account", "delete", "Delete service accounts")
	out = appendPerm(out, "platform", "iam", "idp", "manage", "Manage identity providers")
	// Admin
	out = appendPerm(out, "platform", "admin", "client", "view", "View clients")
	out = appendPerm(out, "platform", "admin", "client", "create", "Create clients")
	out = appendPerm(out, "platform", "admin", "client", "update", "Update clients")
	out = appendPerm(out, "platform", "admin", "client", "delete", "Delete clients")
	out = appendPerm(out, "platform", "admin", "application", "view", "View applications")
	out = appendPerm(out, "platform", "admin", "application", "create", "Create applications")
	out = appendPerm(out, "platform", "admin", "application", "update", "Update applications")
	out = appendPerm(out, "platform", "admin", "application", "delete", "Delete applications")
	out = appendPerm(out, "platform", "admin", "config", "view", "View platform config")
	out = appendPerm(out, "platform", "admin", "config", "update", "Update platform config")
	// Messaging
	out = appendPerm(out, "platform", "messaging", "event", "view", "View events")
	out = appendPerm(out, "platform", "messaging", "event", "view-raw", "View raw event data")
	out = appendPerm(out, "platform", "messaging", "event-type", "view", "View event types")
	out = appendPerm(out, "platform", "messaging", "event-type", "create", "Create event types")
	out = appendPerm(out, "platform", "messaging", "event-type", "update", "Update event types")
	out = appendPerm(out, "platform", "messaging", "event-type", "delete", "Delete event types")
	out = appendPerm(out, "platform", "messaging", "subscription", "view", "View subscriptions")
	out = appendPerm(out, "platform", "messaging", "subscription", "create", "Create subscriptions")
	out = appendPerm(out, "platform", "messaging", "subscription", "update", "Update subscriptions")
	out = appendPerm(out, "platform", "messaging", "subscription", "delete", "Delete subscriptions")
	out = appendPerm(out, "platform", "messaging", "dispatch-job", "view", "View dispatch jobs")
	out = appendPerm(out, "platform", "messaging", "dispatch-job", "view-raw", "View raw dispatch job data")
	out = appendPerm(out, "platform", "messaging", "dispatch-job", "create", "Create dispatch jobs")
	out = appendPerm(out, "platform", "messaging", "dispatch-job", "retry", "Retry dispatch jobs")
	out = appendPerm(out, "platform", "messaging", "dispatch-pool", "view", "View dispatch pools")
	out = appendPerm(out, "platform", "messaging", "dispatch-pool", "create", "Create dispatch pools")
	out = appendPerm(out, "platform", "messaging", "dispatch-pool", "update", "Update dispatch pools")
	out = appendPerm(out, "platform", "messaging", "dispatch-pool", "delete", "Delete dispatch pools")
	sort.Slice(out, func(i, j int) bool { return out[i].Permission < out[j].Permission })
	return out
}

func appendPerm(out []bffPermissionResponse, app, ctx, agg, action, desc string) []bffPermissionResponse {
	return append(out, bffPermissionResponse{
		Permission:  app + ":" + ctx + ":" + agg + ":" + action,
		Application: app,
		Context:     ctx,
		Aggregate:   agg,
		Action:      action,
		Description: desc,
	})
}
