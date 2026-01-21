package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/platform/auth/local"
	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/principal"
	"go.flowcatalyst.tech/internal/platform/principal/operations"
)

// PrincipalAdminHandler handles principal administration endpoints using UseCases
type PrincipalAdminHandler struct {
	principalRepo   principal.Repository
	clientRepo      client.Repository
	passwordService *local.PasswordService

	// UseCases
	createUserUseCase         *operations.CreateUserUseCase
	updateUserUseCase         *operations.UpdateUserUseCase
	activateUserUseCase       *operations.ActivateUserUseCase
	deactivateUserUseCase     *operations.DeactivateUserUseCase
	deleteUserUseCase         *operations.DeleteUserUseCase
	assignRolesUseCase        *operations.AssignRolesUseCase
	grantClientAccessUseCase  *operations.GrantClientAccessUseCase
	revokeClientAccessUseCase *operations.RevokeClientAccessUseCase
}

// NewPrincipalAdminHandler creates a new principal admin handler with UseCases
func NewPrincipalAdminHandler(
	principalRepo principal.Repository,
	clientRepo client.Repository,
	uow common.UnitOfWork,
) *PrincipalAdminHandler {
	return &PrincipalAdminHandler{
		principalRepo:             principalRepo,
		clientRepo:                clientRepo,
		passwordService:           local.NewPasswordService(),
		createUserUseCase:         operations.NewCreateUserUseCase(principalRepo, uow),
		updateUserUseCase:         operations.NewUpdateUserUseCase(principalRepo, uow),
		activateUserUseCase:       operations.NewActivateUserUseCase(principalRepo, uow),
		deactivateUserUseCase:     operations.NewDeactivateUserUseCase(principalRepo, uow),
		deleteUserUseCase:         operations.NewDeleteUserUseCase(principalRepo, uow),
		assignRolesUseCase:        operations.NewAssignRolesUseCase(principalRepo, uow),
		grantClientAccessUseCase:  operations.NewGrantClientAccessUseCase(principalRepo, clientRepo, uow),
		revokeClientAccessUseCase: operations.NewRevokeClientAccessUseCase(principalRepo, clientRepo, uow),
	}
}

// Routes returns the router for principal admin endpoints
func (h *PrincipalAdminHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/activate", h.Activate)
	r.Post("/{id}/deactivate", h.Deactivate)
	r.Post("/{id}/roles", h.AssignRoles)
	r.Delete("/{id}/roles/{roleId}", h.RemoveRole)
	r.Post("/{id}/clients", h.GrantClientAccess)
	r.Delete("/{id}/clients/{clientId}", h.RevokeClientAccess)
	r.Post("/{id}/reset-password", h.ResetPassword)

	return r
}

// PrincipalDTO represents a principal for API responses
type PrincipalDTO struct {
	ID            string                       `json:"id"`
	Type          principal.PrincipalType      `json:"type"`
	Scope         principal.UserScope          `json:"scope"`
	Name          string                       `json:"name"`
	Active        bool                         `json:"active"`
	ClientID      string                       `json:"clientId,omitempty"`
	ApplicationID string                       `json:"applicationId,omitempty"`
	Email         string                       `json:"email,omitempty"`
	EmailVerified bool                         `json:"emailVerified,omitempty"`
	IdpType       principal.IdpType            `json:"idpType,omitempty"`
	Roles         []principal.RoleAssignment   `json:"roles,omitempty"`
	CreatedAt     string                       `json:"createdAt"`
	UpdatedAt     string                       `json:"updatedAt"`
	LastLoginAt   string                       `json:"lastLoginAt,omitempty"`
}

// CreatePrincipalRequest represents a request to create a principal
type CreatePrincipalRequest struct {
	Type     principal.PrincipalType `json:"type"`
	Scope    principal.UserScope     `json:"scope"`
	Name     string                  `json:"name"`
	Email    string                  `json:"email"`
	Password string                  `json:"password,omitempty"`
	ClientID string                  `json:"clientId,omitempty"`
}

// UpdatePrincipalRequest represents a request to update a principal
type UpdatePrincipalRequest struct {
	Name  string            `json:"name,omitempty"`
	Scope principal.UserScope `json:"scope,omitempty"`
}

// AssignRolesRequest represents a request to assign roles
type AssignRolesRequest struct {
	RoleIDs []string `json:"roleIds"`
}

// GrantClientAccessRequest represents a request to grant client access
type GrantClientAccessRequest struct {
	ClientID  string `json:"clientId"`
	ExpiresAt string `json:"expiresAt,omitempty"`
}

// ResetPasswordRequest represents a request to reset password
type ResetPasswordRequest struct {
	NewPassword string `json:"newPassword"`
}

// List handles GET /api/admin/platform/principals
func (h *PrincipalAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	// Optional filters
	clientID := r.URL.Query().Get("clientId")
	principalType := r.URL.Query().Get("type")
	activeOnly := r.URL.Query().Get("active") == "true"

	skip := int64((page - 1) * pageSize)

	var principals []*principal.Principal
	var err error

	if clientID != "" {
		principals, err = h.principalRepo.FindByClientID(r.Context(), clientID, skip, int64(pageSize))
	} else if principalType != "" {
		principals, err = h.principalRepo.FindByType(r.Context(), principal.PrincipalType(principalType), skip, int64(pageSize))
	} else if activeOnly {
		principals, err = h.principalRepo.FindActive(r.Context(), skip, int64(pageSize))
	} else {
		principals, err = h.principalRepo.FindAll(r.Context(), skip, int64(pageSize))
	}

	if err != nil {
		slog.Error("Failed to list principals", "error", err)
		WriteInternalError(w, "Failed to list principals")
		return
	}

	dtos := make([]PrincipalDTO, len(principals))
	for i, p := range principals {
		dtos[i] = toPrincipalDTO(p)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

// Get handles GET /api/admin/platform/principals/{id}
func (h *PrincipalAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	p, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// Create handles POST /api/admin/platform/principals
func (h *PrincipalAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreatePrincipalRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Name == "" {
		WriteBadRequest(w, "Name is required")
		return
	}
	if req.Type == "" {
		req.Type = principal.PrincipalTypeUser
	}
	if req.Scope == "" {
		req.Scope = principal.UserScopeClient
	}

	p := &principal.Principal{
		Type:     req.Type,
		Scope:    req.Scope,
		Name:     req.Name,
		Active:   true,
		ClientID: req.ClientID,
	}

	// Handle user identity for USER type
	if req.Type == principal.PrincipalTypeUser && req.Email != "" {
		email := local.NormalizeEmail(req.Email)
		domain := local.ExtractEmailDomain(email)

		p.UserIdentity = &principal.UserIdentity{
			Email:       email,
			EmailDomain: domain,
			IdpType:     principal.IdpTypeInternal,
		}

		// Set password if provided
		if req.Password != "" {
			if err := h.passwordService.ValidatePasswordStrength(req.Password); err != nil {
				WriteBadRequest(w, "Password does not meet requirements")
				return
			}
			hash, err := h.passwordService.HashPassword(req.Password)
			if err != nil {
				slog.Error("Failed to hash password", "error", err)
				WriteInternalError(w, "Failed to create user")
				return
			}
			p.UserIdentity.PasswordHash = hash
		}
	}

	if err := h.principalRepo.Insert(r.Context(), p); err != nil {
		if err == principal.ErrDuplicateEmail {
			WriteConflict(w, "Email already exists")
			return
		}
		slog.Error("Failed to create principal", "error", err)
		WriteInternalError(w, "Failed to create principal")
		return
	}

	WriteJSON(w, http.StatusCreated, toPrincipalDTO(p))
}

// Update handles PUT /api/admin/platform/principals/{id}
func (h *PrincipalAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdatePrincipalRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	p, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	if req.Name != "" {
		p.Name = req.Name
	}
	if req.Scope != "" {
		p.Scope = req.Scope
	}

	if err := h.principalRepo.Update(r.Context(), p); err != nil {
		slog.Error("Failed to update principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to update principal")
		return
	}

	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// Delete handles DELETE /api/admin/platform/principals/{id}
func (h *PrincipalAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.principalRepo.Delete(r.Context(), id); err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to delete principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete principal")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Activate handles POST /api/admin/platform/principals/{id}/activate
func (h *PrincipalAdminHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.principalRepo.SetActive(r.Context(), id, true); err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to activate principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to activate principal")
		return
	}

	p, _ := h.principalRepo.FindByID(r.Context(), id)
	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// Deactivate handles POST /api/admin/platform/principals/{id}/deactivate
func (h *PrincipalAdminHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.principalRepo.SetActive(r.Context(), id, false); err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to deactivate principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to deactivate principal")
		return
	}

	p, _ := h.principalRepo.FindByID(r.Context(), id)
	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// AssignRoles handles POST /api/admin/platform/principals/{id}/roles
func (h *PrincipalAdminHandler) AssignRoles(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AssignRolesRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	p, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	// Add new roles
	now := time.Now()
	for _, roleID := range req.RoleIDs {
		found := false
		for _, existing := range p.Roles {
			if existing.RoleID == roleID {
				found = true
				break
			}
		}
		if !found {
			p.Roles = append(p.Roles, principal.RoleAssignment{
				RoleID:     roleID,
				AssignedAt: now,
			})
		}
	}

	if err := h.principalRepo.Update(r.Context(), p); err != nil {
		slog.Error("Failed to update principal roles", "error", err, "id", id)
		WriteInternalError(w, "Failed to assign roles")
		return
	}

	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// RemoveRole handles DELETE /api/admin/platform/principals/{id}/roles/{roleId}
func (h *PrincipalAdminHandler) RemoveRole(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	roleID := chi.URLParam(r, "roleId")

	p, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	// Remove role
	newRoles := make([]principal.RoleAssignment, 0, len(p.Roles))
	for _, role := range p.Roles {
		if role.RoleID != roleID {
			newRoles = append(newRoles, role)
		}
	}
	p.Roles = newRoles

	if err := h.principalRepo.Update(r.Context(), p); err != nil {
		slog.Error("Failed to update principal roles", "error", err, "id", id)
		WriteInternalError(w, "Failed to remove role")
		return
	}

	WriteJSON(w, http.StatusOK, toPrincipalDTO(p))
}

// GrantClientAccess handles POST /api/admin/platform/principals/{id}/clients
func (h *PrincipalAdminHandler) GrantClientAccess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req GrantClientAccessRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.ClientID == "" {
		WriteBadRequest(w, "Client ID is required")
		return
	}

	// Verify principal exists
	_, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	// Verify client exists
	_, err = h.clientRepo.FindByID(r.Context(), req.ClientID)
	if err != nil {
		if err == client.ErrNotFound {
			WriteNotFound(w, "Client not found")
			return
		}
		slog.Error("Failed to get client", "error", err, "clientId", req.ClientID)
		WriteInternalError(w, "Failed to get client")
		return
	}

	grant := &client.ClientAccessGrant{
		PrincipalID: id,
		ClientID:    req.ClientID,
	}

	if req.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
		if err == nil {
			grant.ExpiresAt = expiresAt
		}
	}

	if err := h.clientRepo.GrantAccess(r.Context(), grant); err != nil {
		slog.Error("Failed to grant access", "error", err, "id", id, "clientId", req.ClientID)
		WriteInternalError(w, "Failed to grant access")
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]string{"message": "Access granted"})
}

// RevokeClientAccess handles DELETE /api/admin/platform/principals/{id}/clients/{clientId}
func (h *PrincipalAdminHandler) RevokeClientAccess(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	clientID := chi.URLParam(r, "clientId")

	if err := h.clientRepo.RevokeAccess(r.Context(), id, clientID); err != nil {
		slog.Error("Failed to revoke access", "error", err, "id", id, "clientId", clientID)
		WriteInternalError(w, "Failed to revoke access")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ResetPassword handles POST /api/admin/platform/principals/{id}/reset-password
func (h *PrincipalAdminHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req ResetPasswordRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.NewPassword == "" {
		WriteBadRequest(w, "New password is required")
		return
	}

	if err := h.passwordService.ValidatePasswordStrength(req.NewPassword); err != nil {
		WriteBadRequest(w, "Password does not meet requirements")
		return
	}

	p, err := h.principalRepo.FindByID(r.Context(), id)
	if err != nil {
		if err == principal.ErrNotFound {
			WriteNotFound(w, "Principal not found")
			return
		}
		slog.Error("Failed to get principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to get principal")
		return
	}

	if p.UserIdentity == nil {
		WriteBadRequest(w, "Principal does not have password authentication")
		return
	}

	hash, err := h.passwordService.HashPassword(req.NewPassword)
	if err != nil {
		slog.Error("Failed to hash password", "error", err)
		WriteInternalError(w, "Failed to reset password")
		return
	}

	p.UserIdentity.PasswordHash = hash

	if err := h.principalRepo.Update(r.Context(), p); err != nil {
		slog.Error("Failed to update principal", "error", err, "id", id)
		WriteInternalError(w, "Failed to reset password")
		return
	}

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Password reset successfully"})
}

// toPrincipalDTO converts a Principal to PrincipalDTO
func toPrincipalDTO(p *principal.Principal) PrincipalDTO {
	dto := PrincipalDTO{
		ID:            p.ID,
		Type:          p.Type,
		Scope:         p.Scope,
		Name:          p.Name,
		Active:        p.Active,
		ClientID:      p.ClientID,
		ApplicationID: p.ApplicationID,
		Roles:         p.Roles,
		CreatedAt:     p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:     p.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if p.UserIdentity != nil {
		dto.Email = p.UserIdentity.Email
		dto.EmailVerified = p.UserIdentity.EmailVerified
		dto.IdpType = p.UserIdentity.IdpType
		if !p.UserIdentity.LastLoginAt.IsZero() {
			dto.LastLoginAt = p.UserIdentity.LastLoginAt.Format("2006-01-02T15:04:05Z")
		}
	}

	return dto
}
