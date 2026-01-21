package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/role"
	"go.flowcatalyst.tech/internal/platform/role/operations"
)

// RoleHandler handles role endpoints using UseCases
// @Description Role management API for access control
type RoleHandler struct {
	repo role.Repository

	// UseCases
	createUseCase *operations.CreateRoleUseCase
	updateUseCase *operations.UpdateRoleUseCase
	deleteUseCase *operations.DeleteRoleUseCase
}

// NewRoleHandler creates a new role handler with UseCases
func NewRoleHandler(
	repo role.Repository,
	uow common.UnitOfWork,
) *RoleHandler {
	return &RoleHandler{
		repo:          repo,
		createUseCase: operations.NewCreateRoleUseCase(repo, uow),
		updateUseCase: operations.NewUpdateRoleUseCase(repo, uow),
		deleteUseCase: operations.NewDeleteRoleUseCase(repo, uow),
	}
}

// Routes returns the router for role endpoints
func (h *RoleHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)

	return r
}

// List handles GET /api/admin/platform/roles
// @Summary List all roles
// @Description Returns a list of all roles in the system
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Param scope query string false "Filter by scope (ANCHOR, PARTNER, CLIENT)"
// @Success 200 {array} role.Role
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/roles [get]
func (h *RoleHandler) List(w http.ResponseWriter, r *http.Request) {
	roles, err := h.repo.FindAll(r.Context())
	if err != nil {
		slog.Error("Failed to list roles", "error", err)
		WriteInternalError(w, "Failed to list roles")
		return
	}
	WriteJSON(w, http.StatusOK, roles)
}

// Get handles GET /api/admin/platform/roles/{id}
// @Summary Get role by ID
// @Description Returns a single role by its ID
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Success 200 {object} role.Role
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/roles/{id} [get]
func (h *RoleHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	roleData, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get role", "error", err, "id", id)
		WriteInternalError(w, "Failed to get role")
		return
	}
	if roleData == nil {
		WriteNotFound(w, "Role not found")
		return
	}
	WriteJSON(w, http.StatusOK, roleData)
}

// Create handles POST /api/admin/platform/roles (using UseCase)
// @Summary Create a new role
// @Description Creates a new role with specified permissions
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Param request body operations.CreateRoleCommand true "Role details"
// @Success 201 {object} role.Role
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Role with code already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/roles [post]
func (h *RoleHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cmd operations.CreateRoleCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Update handles PUT /api/admin/platform/roles/{id} (using UseCase)
// @Summary Update a role
// @Description Updates an existing role
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Param request body operations.UpdateRoleCommand true "Updated role details"
// @Success 200 {object} role.Role
// @Failure 400 {object} ErrorResponse
// @Failure 403 {object} ErrorResponse "Cannot modify built-in role"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/roles/{id} [put]
func (h *RoleHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cmd operations.UpdateRoleCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}
	cmd.ID = id

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Delete handles DELETE /api/admin/platform/roles/{id} (using UseCase)
// @Summary Delete a role
// @Description Deletes a role (built-in roles cannot be deleted)
// @Tags Admin - Roles
// @Accept json
// @Produce json
// @Param id path string true "Role ID"
// @Success 204 "No Content"
// @Failure 403 {object} ErrorResponse "Cannot delete built-in role"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Role is in use"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/roles/{id} [delete]
func (h *RoleHandler) Delete(w http.ResponseWriter, r *http.Request) {
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
