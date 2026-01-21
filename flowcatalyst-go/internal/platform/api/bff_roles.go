package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/permission"
	"go.flowcatalyst.tech/internal/platform/role"
	"go.flowcatalyst.tech/internal/platform/role/operations"
)

// RoleBffHandler handles BFF endpoints for roles and permissions
// @Description Web-optimized role and permission endpoints with string IDs for JavaScript clients
type RoleBffHandler struct {
	roleRepo        role.Repository
	permissionRepo  permission.Repository
	applicationRepo *application.Repository
	unitOfWork      common.UnitOfWork

	// UseCases
	createUseCase *operations.CreateRoleUseCase
	updateUseCase *operations.UpdateRoleUseCase
	deleteUseCase *operations.DeleteRoleUseCase
}

// NewRoleBffHandler creates a new role BFF handler
func NewRoleBffHandler(
	roleRepo role.Repository,
	permissionRepo permission.Repository,
	applicationRepo *application.Repository,
	uow common.UnitOfWork,
) *RoleBffHandler {
	return &RoleBffHandler{
		roleRepo:        roleRepo,
		permissionRepo:  permissionRepo,
		applicationRepo: applicationRepo,
		unitOfWork:      uow,
		createUseCase:   operations.NewCreateRoleUseCase(roleRepo, uow),
		updateUseCase:   operations.NewUpdateRoleUseCase(roleRepo, uow),
		deleteUseCase:   operations.NewDeleteRoleUseCase(roleRepo, uow),
	}
}

// Routes returns the router for role BFF endpoints
func (h *RoleBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/filters/applications", h.GetApplications)
	r.Get("/permissions", h.ListPermissions)
	r.Get("/permissions/{code}", h.GetPermission)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)

	return r
}

// === BFF DTOs - IDs as Strings for JavaScript precision ===

// BffRoleResponse represents a role in BFF API responses
// @Description Role with string IDs for JavaScript client precision
type BffRoleResponse struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	ShortName   string   `json:"shortName,omitempty"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope"`
	Permissions []string `json:"permissions"`
	BuiltIn     bool     `json:"builtIn"`
	CreatedAt   string   `json:"createdAt,omitempty"`
	UpdatedAt   string   `json:"updatedAt,omitempty"`
}

// BffRoleListResponse wraps a list of roles
type BffRoleListResponse struct {
	Items []BffRoleResponse `json:"items"`
	Total int               `json:"total"`
}

// BffPermissionResponse represents a permission in BFF responses
type BffPermissionResponse struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
	Application string `json:"application,omitempty"`
	Context     string `json:"context,omitempty"`
	Aggregate   string `json:"aggregate,omitempty"`
	Action      string `json:"action,omitempty"`
}

// BffPermissionListResponse wraps a list of permissions
type BffPermissionListResponse struct {
	Items []BffPermissionResponse `json:"items"`
	Total int                     `json:"total"`
}

// BffApplicationOption represents an application for filter dropdowns
type BffApplicationOption struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

// BffApplicationOptionsResponse wraps application filter options
type BffApplicationOptionsResponse struct {
	Options []BffApplicationOption `json:"options"`
}

// CreateRoleRequest represents a request to create a role
type CreateRoleBffRequest struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope"`
	Permissions []string `json:"permissions,omitempty"`
}

// UpdateRoleRequest represents a request to update a role
type UpdateRoleBffRequest struct {
	Name        string   `json:"name,omitempty"`
	Description string   `json:"description,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
}

// toBffRoleResponse converts a Role to BFF response format
func toBffRoleResponse(r *role.Role) BffRoleResponse {
	// Extract short name from code (e.g., "platform:admin" -> "admin")
	shortName := r.Code
	if idx := strings.LastIndex(r.Code, ":"); idx >= 0 {
		shortName = r.Code[idx+1:]
	}

	return BffRoleResponse{
		ID:          r.ID,
		Code:        r.Code,
		Name:        r.Name,
		ShortName:   shortName,
		Description: r.Description,
		Scope:       r.Scope,
		Permissions: r.Permissions,
		BuiltIn:     r.BuiltIn,
		CreatedAt:   formatTimeRFC3339(r.CreatedAt),
		UpdatedAt:   formatTimeRFC3339(r.UpdatedAt),
	}
}

func formatTimeRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// toBffPermissionResponse converts a Permission to BFF response format
func toBffPermissionResponse(p *permission.Permission) BffPermissionResponse {
	// Parse code to extract application, context, aggregate, action
	// Format: application:context:aggregate:action
	parts := strings.Split(p.Code, ":")
	var app, ctx, agg, action string
	if len(parts) > 0 {
		app = parts[0]
	}
	if len(parts) > 1 {
		ctx = parts[1]
	}
	if len(parts) > 2 {
		agg = parts[2]
	}
	if len(parts) > 3 {
		action = parts[3]
	}

	return BffPermissionResponse{
		ID:          p.ID,
		Code:        p.Code,
		Name:        p.Name,
		Description: p.Description,
		Category:    p.Category,
		Application: app,
		Context:     ctx,
		Aggregate:   agg,
		Action:      action,
	}
}

// List handles GET /bff/roles
// @Summary List all roles (BFF)
// @Description Returns all roles with optional filtering
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param scope query string false "Filter by scope (ANCHOR, PARTNER, CLIENT)"
// @Param builtIn query string false "Filter by built-in status (true, false)"
// @Success 200 {object} BffRoleListResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles [get]
func (h *RoleBffHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	scope := r.URL.Query().Get("scope")
	builtInStr := r.URL.Query().Get("builtIn")

	// Get all roles
	roles, err := h.roleRepo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to list roles", "error", err)
		WriteInternalError(w, "Failed to list roles")
		return
	}

	// Apply filters
	var filtered []*role.Role
	for _, r := range roles {
		// Scope filter
		if scope != "" && r.Scope != scope {
			continue
		}

		// BuiltIn filter
		if builtInStr != "" {
			if builtInStr == "true" && !r.BuiltIn {
				continue
			}
			if builtInStr == "false" && r.BuiltIn {
				continue
			}
		}

		filtered = append(filtered, r)
	}

	// Convert to BFF response
	responses := make([]BffRoleResponse, len(filtered))
	for i, r := range filtered {
		responses[i] = toBffRoleResponse(r)
	}

	WriteJSON(w, http.StatusOK, BffRoleListResponse{
		Items: responses,
		Total: len(responses),
	})
}

// Get handles GET /bff/roles/{id}
// @Summary Get role by ID (BFF)
// @Description Returns a single role by its ID
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Success 200 {object} BffRoleResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles/{id} [get]
func (h *RoleBffHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	rol, err := h.roleRepo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get role", "error", err, "id", id)
		WriteInternalError(w, "Failed to get role")
		return
	}
	if rol == nil {
		WriteNotFound(w, "Role not found")
		return
	}

	WriteJSON(w, http.StatusOK, toBffRoleResponse(rol))
}

// GetApplications handles GET /bff/roles/filters/applications
// @Summary Get applications for role filter
// @Description Returns available applications for filtering roles
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Success 200 {object} BffApplicationOptionsResponse
// @Router /bff/roles/filters/applications [get]
func (h *RoleBffHandler) GetApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	apps, err := h.applicationRepo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to get applications", "error", err)
		WriteInternalError(w, "Failed to get applications")
		return
	}

	// Filter to active applications only
	options := make([]BffApplicationOption, 0, len(apps))
	for _, app := range apps {
		if app.Active {
			options = append(options, BffApplicationOption{
				ID:   app.ID,
				Code: app.Code,
				Name: app.Name,
			})
		}
	}

	WriteJSON(w, http.StatusOK, BffApplicationOptionsResponse{Options: options})
}

// Create handles POST /bff/roles
// @Summary Create a new role (BFF)
// @Description Creates a new role
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param request body CreateRoleBffRequest true "Role details"
// @Success 201 {object} BffRoleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles [post]
func (h *RoleBffHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateRoleBffRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.CreateRoleCommand{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Scope:       req.Scope,
		Permissions: req.Permissions,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Type assert to get RoleID
	createdEvent, ok := result.Value().(*events.RoleCreated)
	if !ok {
		WriteInternalError(w, "Failed to process created role")
		return
	}

	// Fetch the created role
	rol, err := h.roleRepo.FindByID(r.Context(), createdEvent.RoleID)
	if err != nil || rol == nil {
		WriteInternalError(w, "Failed to fetch created role")
		return
	}

	WriteJSON(w, http.StatusCreated, toBffRoleResponse(rol))
}

// Update handles PUT /bff/roles/{id}
// @Summary Update a role (BFF)
// @Description Updates an existing role
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Param request body UpdateRoleBffRequest true "Updated role details"
// @Success 200 {object} BffRoleResponse
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse "Cannot modify built-in role"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles/{id} [put]
func (h *RoleBffHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateRoleBffRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.UpdateRoleCommand{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the updated role
	rol, err := h.roleRepo.FindByID(r.Context(), id)
	if err != nil || rol == nil {
		WriteInternalError(w, "Failed to fetch updated role")
		return
	}

	WriteJSON(w, http.StatusOK, toBffRoleResponse(rol))
}

// Delete handles DELETE /bff/roles/{id}
// @Summary Delete a role (BFF)
// @Description Deletes a role
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Success 204 "No Content"
// @Failure 403 {object} ErrorResponse "Cannot delete built-in role"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles/{id} [delete]
func (h *RoleBffHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.DeleteRoleCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.deleteUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListPermissions handles GET /bff/roles/permissions
// @Summary List all permissions (BFF)
// @Description Returns all available permissions
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Success 200 {object} BffPermissionListResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles/permissions [get]
func (h *RoleBffHandler) ListPermissions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	permissions, err := h.permissionRepo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to list permissions", "error", err)
		WriteInternalError(w, "Failed to list permissions")
		return
	}

	// Convert to BFF response
	responses := make([]BffPermissionResponse, len(permissions))
	for i, p := range permissions {
		responses[i] = toBffPermissionResponse(p)
	}

	WriteJSON(w, http.StatusOK, BffPermissionListResponse{
		Items: responses,
		Total: len(responses),
	})
}

// GetPermission handles GET /bff/roles/permissions/{code}
// @Summary Get permission by code (BFF)
// @Description Returns a single permission by its code
// @Tags BFF - Roles
// @Accept json
// @Produce json
// @Param code path string true "Permission code"
// @Success 200 {object} BffPermissionResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/roles/permissions/{code} [get]
func (h *RoleBffHandler) GetPermission(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	perm, err := h.permissionRepo.FindByCode(r.Context(), code)
	if err != nil {
		slog.Error("Failed to get permission", "error", err, "code", code)
		WriteInternalError(w, "Failed to get permission")
		return
	}
	if perm == nil {
		WriteNotFound(w, "Permission not found")
		return
	}

	WriteJSON(w, http.StatusOK, toBffPermissionResponse(perm))
}
