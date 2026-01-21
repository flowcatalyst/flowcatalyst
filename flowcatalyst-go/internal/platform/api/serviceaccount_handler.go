package api

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
	"go.flowcatalyst.tech/internal/platform/serviceaccount/operations"
)

// ServiceAccountHandler handles service account endpoints using UseCases
// @Description Service account management API for machine-to-machine authentication
type ServiceAccountHandler struct {
	repo *serviceaccount.Repository

	// UseCases
	createUseCase          *operations.CreateServiceAccountUseCase
	rotateCredentialsCase  *operations.RotateCredentialsUseCase
	deleteUseCase          *operations.DeleteServiceAccountUseCase
}

// NewServiceAccountHandler creates a new service account handler with UseCases
func NewServiceAccountHandler(
	repo *serviceaccount.Repository,
	uow common.UnitOfWork,
) *ServiceAccountHandler {
	return &ServiceAccountHandler{
		repo:                  repo,
		createUseCase:         operations.NewCreateServiceAccountUseCase(repo, uow),
		rotateCredentialsCase: operations.NewRotateCredentialsUseCase(repo, uow),
		deleteUseCase:         operations.NewDeleteServiceAccountUseCase(repo, uow),
	}
}

// Routes returns the router for service account endpoints
func (h *ServiceAccountHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/rotate", h.RotateCredentials)

	return r
}

// List handles GET /api/admin/platform/service-accounts
// @Summary List all service accounts
// @Description Returns a list of all service accounts in the system
// @Tags Admin - Service Accounts
// @Accept json
// @Produce json
// @Success 200 {array} serviceaccount.ServiceAccount
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/service-accounts [get]
func (h *ServiceAccountHandler) List(w http.ResponseWriter, r *http.Request) {
	accounts, err := h.repo.FindAll(r.Context())
	if err != nil {
		slog.Error("Failed to list service accounts", "error", err)
		WriteInternalError(w, "Failed to list service accounts")
		return
	}
	WriteJSON(w, http.StatusOK, accounts)
}

// Get handles GET /api/admin/platform/service-accounts/{id}
// @Summary Get service account by ID
// @Description Returns a single service account by its ID
// @Tags Admin - Service Accounts
// @Accept json
// @Produce json
// @Param id path string true "Service Account ID"
// @Success 200 {object} serviceaccount.ServiceAccount
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/service-accounts/{id} [get]
func (h *ServiceAccountHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	account, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get service account", "error", err, "id", id)
		WriteInternalError(w, "Failed to get service account")
		return
	}
	if account == nil {
		WriteNotFound(w, "Service account not found")
		return
	}
	WriteJSON(w, http.StatusOK, account)
}

// Create handles POST /api/admin/platform/service-accounts (using UseCase)
// @Summary Create a new service account
// @Description Creates a new service account for machine-to-machine authentication. Returns credentials only on creation.
// @Tags Admin - Service Accounts
// @Accept json
// @Produce json
// @Param request body operations.CreateServiceAccountCommand true "Service account details"
// @Success 201 {object} serviceaccount.ServiceAccount "Created service account with credentials (secret shown only once)"
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Service account with name already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/service-accounts [post]
func (h *ServiceAccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cmd operations.CreateServiceAccountCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Delete handles DELETE /api/admin/platform/service-accounts/{id} (using UseCase)
// @Summary Delete a service account
// @Description Permanently deletes a service account and revokes all credentials
// @Tags Admin - Service Accounts
// @Accept json
// @Produce json
// @Param id path string true "Service Account ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/service-accounts/{id} [delete]
func (h *ServiceAccountHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.DeleteServiceAccountCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.deleteUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// RotateCredentials handles POST /api/admin/platform/service-accounts/{id}/rotate (using UseCase)
// @Summary Rotate service account credentials
// @Description Generates new credentials and invalidates the old ones. Returns new secret only once.
// @Tags Admin - Service Accounts
// @Accept json
// @Produce json
// @Param id path string true "Service Account ID"
// @Success 200 {object} serviceaccount.ServiceAccount "Service account with new credentials (secret shown only once)"
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/admin/platform/service-accounts/{id}/rotate [post]
func (h *ServiceAccountHandler) RotateCredentials(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.RotateCredentialsCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.rotateCredentialsCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}
