// Package api provides HTTP handlers for the platform API
package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/application"
	appops "go.flowcatalyst.tech/internal/platform/application/operations"
	"go.flowcatalyst.tech/internal/platform/common"
)

// ApplicationAdminHandler handles Application admin operations using UseCases
type ApplicationAdminHandler struct {
	repo *application.Repository

	// UseCases
	createUseCase     *appops.CreateApplicationUseCase
	updateUseCase     *appops.UpdateApplicationUseCase
	deactivateUseCase *appops.DeactivateApplicationUseCase
	provisionUseCase  *appops.ProvisionApplicationUseCase
}

// NewApplicationAdminHandler creates a new Application admin handler with UseCases
func NewApplicationAdminHandler(
	repo *application.Repository,
	uow common.UnitOfWork,
) *ApplicationAdminHandler {
	return &ApplicationAdminHandler{
		repo:              repo,
		createUseCase:     appops.NewCreateApplicationUseCase(repo, uow),
		updateUseCase:     appops.NewUpdateApplicationUseCase(repo, uow),
		deactivateUseCase: appops.NewDeactivateApplicationUseCase(repo, uow),
		provisionUseCase:  appops.NewProvisionApplicationUseCase(repo, uow),
	}
}

// CreateApplicationRequest represents a request to create an Application
type CreateApplicationRequest struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
	Type           string `json:"type,omitempty"` // APPLICATION or INTEGRATION
}

// UpdateApplicationRequest represents a request to update an Application
type UpdateApplicationRequest struct {
	Name           string `json:"name,omitempty"`
	Description    string `json:"description,omitempty"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
}

// ApplicationResponse represents an Application in API responses
type ApplicationResponse struct {
	ID               string    `json:"id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	DefaultBaseURL   string    `json:"defaultBaseUrl,omitempty"`
	IconURL          string    `json:"iconUrl,omitempty"`
	Type             string    `json:"type"`
	ServiceAccountID string    `json:"serviceAccountId,omitempty"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// ApplicationListResponse wraps a list of applications
type ApplicationListResponse struct {
	Applications []ApplicationResponse `json:"applications"`
	Total        int                   `json:"total"`
}

// toResponse converts an Application to its API response form
func toApplicationResponse(a *application.Application) ApplicationResponse {
	appType := "APPLICATION"
	if a.Type != "" {
		appType = string(a.Type)
	}
	return ApplicationResponse{
		ID:               a.ID,
		Code:             a.Code,
		Name:             a.Name,
		Description:      a.Description,
		DefaultBaseURL:   a.DefaultBaseURL,
		IconURL:          a.IconURL,
		Type:             appType,
		ServiceAccountID: a.ServiceAccountID,
		Active:           a.Active,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
}

// List handles GET /api/admin/platform/applications
func (h *ApplicationAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	apps, err := h.repo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to list applications", "error", err)
		WriteInternalError(w, "Failed to list applications")
		return
	}

	// Filter by activeOnly if requested
	activeOnly := r.URL.Query().Get("activeOnly") == "true"
	appType := r.URL.Query().Get("type")

	var filtered []*application.Application
	for _, app := range apps {
		if activeOnly && !app.Active {
			continue
		}
		if appType != "" && string(app.Type) != appType {
			continue
		}
		filtered = append(filtered, app)
	}

	response := make([]ApplicationResponse, len(filtered))
	for i, a := range filtered {
		response[i] = toApplicationResponse(a)
	}

	WriteJSON(w, http.StatusOK, ApplicationListResponse{
		Applications: response,
		Total:        len(response),
	})
}

// Get handles GET /api/admin/platform/applications/{id}
func (h *ApplicationAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	app, err := h.repo.FindByID(ctx, id)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "id", id)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	WriteJSON(w, http.StatusOK, toApplicationResponse(app))
}

// GetByCode handles GET /api/admin/platform/applications/by-code/{code}
func (h *ApplicationAdminHandler) GetByCode(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := chi.URLParam(r, "code")

	app, err := h.repo.FindByCode(ctx, code)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "code", code)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	WriteJSON(w, http.StatusOK, toApplicationResponse(app))
}

// Create handles POST /api/admin/platform/applications
func (h *ApplicationAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req CreateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Code == "" {
		WriteBadRequest(w, "code is required")
		return
	}
	if req.Name == "" {
		WriteBadRequest(w, "name is required")
		return
	}

	// Check for duplicate code
	existing, err := h.repo.FindByCode(ctx, req.Code)
	if err != nil {
		slog.Error("Failed to check for existing application", "error", err)
		WriteInternalError(w, "Failed to create application")
		return
	}
	if existing != nil {
		WriteBadRequest(w, "Application with this code already exists")
		return
	}

	// Determine application type
	appType := application.ApplicationTypeApplication
	if req.Type != "" {
		if req.Type != "APPLICATION" && req.Type != "INTEGRATION" {
			WriteBadRequest(w, "type must be APPLICATION or INTEGRATION")
			return
		}
		appType = application.ApplicationType(req.Type)
	}

	app := &application.Application{
		ID:             tsid.Generate(),
		Code:           req.Code,
		Name:           req.Name,
		Description:    req.Description,
		DefaultBaseURL: req.DefaultBaseURL,
		IconURL:        req.IconURL,
		Type:           appType,
		Active:         true,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.repo.Insert(ctx, app); err != nil {
		slog.Error("Failed to create application", "error", err)
		WriteInternalError(w, "Failed to create application")
		return
	}

	slog.Info("Application created", "id", app.ID, "code", app.Code, "name", app.Name)

	WriteJSON(w, http.StatusCreated, toApplicationResponse(app))
}

// Update handles PUT /api/admin/platform/applications/{id}
func (h *ApplicationAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	app, err := h.repo.FindByID(ctx, id)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "id", id)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	var req UpdateApplicationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Apply updates
	if req.Name != "" {
		app.Name = req.Name
	}
	if req.Description != "" {
		app.Description = req.Description
	}
	if req.DefaultBaseURL != "" {
		app.DefaultBaseURL = req.DefaultBaseURL
	}
	if req.IconURL != "" {
		app.IconURL = req.IconURL
	}

	if err := h.repo.Update(ctx, app); err != nil {
		slog.Error("Failed to update application", "error", err, "id", id)
		WriteInternalError(w, "Failed to update application")
		return
	}

	slog.Info("Application updated", "id", app.ID, "code", app.Code)

	WriteJSON(w, http.StatusOK, toApplicationResponse(app))
}

// Activate handles POST /api/admin/platform/applications/{id}/activate
func (h *ApplicationAdminHandler) Activate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	app, err := h.repo.FindByID(ctx, id)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "id", id)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	if app.Active {
		WriteJSON(w, http.StatusOK, map[string]string{"message": "Application already active"})
		return
	}

	app.Active = true
	if err := h.repo.Update(ctx, app); err != nil {
		slog.Error("Failed to activate application", "error", err, "id", id)
		WriteInternalError(w, "Failed to activate application")
		return
	}

	slog.Info("Application activated", "id", app.ID, "code", app.Code)

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Application activated"})
}

// Deactivate handles POST /api/admin/platform/applications/{id}/deactivate
func (h *ApplicationAdminHandler) Deactivate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	app, err := h.repo.FindByID(ctx, id)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "id", id)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	if !app.Active {
		WriteJSON(w, http.StatusOK, map[string]string{"message": "Application already deactivated"})
		return
	}

	app.Active = false
	if err := h.repo.Update(ctx, app); err != nil {
		slog.Error("Failed to deactivate application", "error", err, "id", id)
		WriteInternalError(w, "Failed to deactivate application")
		return
	}

	slog.Info("Application deactivated", "id", app.ID, "code", app.Code)

	WriteJSON(w, http.StatusOK, map[string]string{"message": "Application deactivated"})
}

// Delete handles DELETE /api/admin/platform/applications/{id}
func (h *ApplicationAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := chi.URLParam(r, "id")

	app, err := h.repo.FindByID(ctx, id)
	if err != nil {
		slog.Error("Failed to get application", "error", err, "id", id)
		WriteInternalError(w, "Failed to get application")
		return
	}
	if app == nil {
		WriteNotFound(w, "Application not found")
		return
	}

	// Application must be deactivated before deletion
	if app.Active {
		WriteBadRequest(w, "Application must be deactivated before deletion")
		return
	}

	if err := h.repo.Delete(ctx, id); err != nil {
		slog.Error("Failed to delete application", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete application")
		return
	}

	slog.Info("Application deleted", "id", id)

	w.WriteHeader(http.StatusNoContent)
}
