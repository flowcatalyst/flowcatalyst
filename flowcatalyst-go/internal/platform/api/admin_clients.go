package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/client/operations"
	"go.flowcatalyst.tech/internal/platform/common"
)

// ClientAdminHandler handles client administration endpoints using UseCases
type ClientAdminHandler struct {
	repo client.Repository

	// UseCases
	createUseCase   *operations.CreateClientUseCase
	updateUseCase   *operations.UpdateClientUseCase
	suspendUseCase  *operations.SuspendClientUseCase
	activateUseCase *operations.ActivateClientUseCase
}

// NewClientAdminHandler creates a new client admin handler with UseCases
func NewClientAdminHandler(
	repo client.Repository,
	uow common.UnitOfWork,
) *ClientAdminHandler {
	return &ClientAdminHandler{
		repo:            repo,
		createUseCase:   operations.NewCreateClientUseCase(repo, uow),
		updateUseCase:   operations.NewUpdateClientUseCase(repo, uow),
		suspendUseCase:  operations.NewSuspendClientUseCase(repo, uow),
		activateUseCase: operations.NewActivateClientUseCase(repo, uow),
	}
}

// Routes returns the router for client admin endpoints
func (h *ClientAdminHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/suspend", h.Suspend)
	r.Post("/{id}/activate", h.Activate)
	r.Post("/{id}/notes", h.AddNote)

	return r
}

// ClientDTO represents a client for API responses
type ClientDTO struct {
	ID              string              `json:"id"`
	Name            string              `json:"name"`
	Identifier      string              `json:"identifier"`
	Status          client.ClientStatus `json:"status"`
	StatusReason    string              `json:"statusReason,omitempty"`
	StatusChangedAt string              `json:"statusChangedAt,omitempty"`
	CreatedAt       string              `json:"createdAt"`
	UpdatedAt       string              `json:"updatedAt"`
}

// CreateClientRequest represents a request to create a client
type CreateClientRequest struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// UpdateClientRequest represents a request to update a client
type UpdateClientRequest struct {
	Name string `json:"name"`
}

// SuspendClientRequest represents a request to suspend a client
type SuspendClientRequest struct {
	Reason string `json:"reason"`
}

// AddNoteRequest represents a request to add a note
type AddNoteRequest struct {
	Text     string `json:"text"`
	Category string `json:"category,omitempty"`
}

// Search handles GET /api/admin/platform/clients/search
func (h *ClientAdminHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		WriteBadRequest(w, "Query parameter 'q' is required")
		return
	}

	clients, err := h.repo.Search(r.Context(), query)
	if err != nil {
		slog.Error("Failed to search clients", "error", err, "query", query)
		WriteInternalError(w, "Failed to search clients")
		return
	}

	// Convert to DTOs
	dtos := make([]ClientDTO, len(clients))
	for i, c := range clients {
		dtos[i] = toClientDTO(c)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

// GetByIdentifier handles GET /api/admin/platform/clients/by-identifier/{identifier}
func (h *ClientAdminHandler) GetByIdentifier(w http.ResponseWriter, r *http.Request) {
	identifier := chi.URLParam(r, "identifier")

	c, err := h.repo.FindByIdentifier(r.Context(), identifier)
	if err != nil {
		slog.Error("Failed to get client by identifier", "error", err, "identifier", identifier)
		WriteInternalError(w, "Failed to get client")
		return
	}
	if c == nil {
		WriteNotFound(w, "Client not found")
		return
	}

	WriteJSON(w, http.StatusOK, toClientDTO(c))
}

// List handles GET /api/admin/platform/clients
func (h *ClientAdminHandler) List(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	skip := int64((page - 1) * pageSize)
	clients, err := h.repo.FindAll(r.Context(), skip, int64(pageSize))
	if err != nil {
		slog.Error("Failed to list clients", "error", err)
		WriteInternalError(w, "Failed to list clients")
		return
	}

	// Convert to DTOs
	dtos := make([]ClientDTO, len(clients))
	for i, c := range clients {
		dtos[i] = toClientDTO(c)
	}

	WriteJSON(w, http.StatusOK, dtos)
}

// Get handles GET /api/admin/platform/clients/{id}
func (h *ClientAdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	c, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == client.ErrNotFound {
			WriteNotFound(w, "Client not found")
			return
		}
		slog.Error("Failed to get client", "error", err, "id", id)
		WriteInternalError(w, "Failed to get client")
		return
	}

	WriteJSON(w, http.StatusOK, toClientDTO(c))
}

// Create handles POST /api/admin/platform/clients (using UseCase)
func (h *ClientAdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateClientRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.CreateClientCommand{
		Name:       req.Name,
		Identifier: req.Identifier,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Update handles PUT /api/admin/platform/clients/{id} (using UseCase)
func (h *ClientAdminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateClientRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.UpdateClientCommand{
		ID:   id,
		Name: req.Name,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Delete handles DELETE /api/admin/platform/clients/{id}
func (h *ClientAdminHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.repo.Delete(r.Context(), id); err != nil {
		if err == client.ErrNotFound {
			WriteNotFound(w, "Client not found")
			return
		}
		slog.Error("Failed to delete client", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete client")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Suspend handles POST /api/admin/platform/clients/{id}/suspend (using UseCase)
func (h *ClientAdminHandler) Suspend(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req SuspendClientRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.SuspendClientCommand{
		ID:     id,
		Reason: req.Reason,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.suspendUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Activate handles POST /api/admin/platform/clients/{id}/activate (using UseCase)
func (h *ClientAdminHandler) Activate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.ActivateClientCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.activateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// AddNote handles POST /api/admin/platform/clients/{id}/notes
func (h *ClientAdminHandler) AddNote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AddNoteRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Text == "" {
		WriteBadRequest(w, "Text is required")
		return
	}

	p := GetPrincipal(r.Context())
	addedBy := ""
	if p != nil {
		addedBy = p.ID
	}

	note := client.ClientNote{
		Text:     req.Text,
		Category: req.Category,
		AddedBy:  addedBy,
	}

	if err := h.repo.AddNote(r.Context(), id, note); err != nil {
		if err == client.ErrNotFound {
			WriteNotFound(w, "Client not found")
			return
		}
		slog.Error("Failed to add note", "error", err, "id", id)
		WriteInternalError(w, "Failed to add note")
		return
	}

	c, _ := h.repo.FindByID(r.Context(), id)
	WriteJSON(w, http.StatusOK, toClientDTO(c))
}

// toClientDTO converts a Client to ClientDTO
func toClientDTO(c *client.Client) ClientDTO {
	dto := ClientDTO{
		ID:         c.ID,
		Name:       c.Name,
		Identifier: c.Identifier,
		Status:     c.Status,
		CreatedAt:  c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:  c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	if c.StatusReason != "" {
		dto.StatusReason = c.StatusReason
	}
	if !c.StatusChangedAt.IsZero() {
		dto.StatusChangedAt = c.StatusChangedAt.Format("2006-01-02T15:04:05Z")
	}

	return dto
}
