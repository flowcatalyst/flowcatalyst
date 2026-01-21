package api

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/eventtype"
	"go.flowcatalyst.tech/internal/platform/eventtype/operations"
)

// EventTypeHandler handles event type endpoints using UseCases
// @Description Event type management API with schema versioning support
type EventTypeHandler struct {
	repo eventtype.Repository

	// UseCases
	createUseCase           *operations.CreateEventTypeUseCase
	updateUseCase           *operations.UpdateEventTypeUseCase
	archiveUseCase          *operations.ArchiveEventTypeUseCase
	addSchemaUseCase        *operations.AddSchemaUseCase
	finaliseSchemaUseCase   *operations.FinaliseSchemaUseCase
	deprecateSchemaUseCase  *operations.DeprecateSchemaUseCase
}

// NewEventTypeHandler creates a new event type handler with UseCases
func NewEventTypeHandler(
	repo eventtype.Repository,
	uow common.UnitOfWork,
) *EventTypeHandler {
	return &EventTypeHandler{
		repo:                   repo,
		createUseCase:          operations.NewCreateEventTypeUseCase(repo, uow),
		updateUseCase:          operations.NewUpdateEventTypeUseCase(repo, uow),
		archiveUseCase:         operations.NewArchiveEventTypeUseCase(repo, uow),
		addSchemaUseCase:       operations.NewAddSchemaUseCase(repo, uow),
		finaliseSchemaUseCase:  operations.NewFinaliseSchemaUseCase(repo, uow),
		deprecateSchemaUseCase: operations.NewDeprecateSchemaUseCase(repo, uow),
	}
}

// Routes returns the router for event type endpoints
func (h *EventTypeHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/{id}", h.Get)
	r.Put("/{id}", h.Update)
	r.Delete("/{id}", h.Delete)
	r.Post("/{id}/archive", h.Archive)

	// Schema management
	r.Get("/{id}/schemas", h.ListSchemas)
	r.Post("/{id}/schemas", h.AddSchema)
	r.Get("/{id}/schemas/{version}", h.GetSchema)
	r.Post("/{id}/schemas/{version}/finalise", h.FinaliseSchema)
	r.Post("/{id}/schemas/{version}/deprecate", h.DeprecateSchema)
	r.Delete("/{id}/schemas/{version}", h.DeleteSchema)

	return r
}

// List handles GET /event-types
// @Summary List all event types
// @Description Returns a list of all event types with optional filtering
// @Tags Event Types
// @Accept json
// @Produce json
// @Param status query string false "Filter by status (CURRENT, ARCHIVED)"
// @Success 200 {array} eventtype.EventType
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types [get]
func (h *EventTypeHandler) List(w http.ResponseWriter, r *http.Request) {
	types, err := h.repo.FindAll(r.Context())
	if err != nil {
		slog.Error("Failed to list event types", "error", err)
		WriteInternalError(w, "Failed to list event types")
		return
	}
	WriteJSON(w, http.StatusOK, types)
}

// Get handles GET /event-types/{id}
// @Summary Get event type by ID
// @Description Returns a single event type by its ID
// @Tags Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 200 {object} eventtype.EventType
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id} [get]
func (h *EventTypeHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event type")
		return
	}
	if et == nil {
		WriteNotFound(w, "Event type not found")
		return
	}
	WriteJSON(w, http.StatusOK, et)
}

// Create handles POST /event-types (using UseCase)
// @Summary Create a new event type
// @Description Creates a new event type in the system
// @Tags Event Types
// @Accept json
// @Produce json
// @Param request body operations.CreateEventTypeCommand true "Event type details"
// @Success 201 {object} eventtype.EventType
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Event type with code already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types [post]
func (h *EventTypeHandler) Create(w http.ResponseWriter, r *http.Request) {
	var cmd operations.CreateEventTypeCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// Update handles PUT /event-types/{id} (using UseCase)
// @Summary Update an event type
// @Description Updates an existing event type
// @Tags Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param request body operations.UpdateEventTypeCommand true "Updated event type details"
// @Success 200 {object} eventtype.EventType
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id} [put]
func (h *EventTypeHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cmd operations.UpdateEventTypeCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}
	cmd.ID = id

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// Delete handles DELETE /event-types/{id}
// @Summary Delete an event type
// @Description Permanently deletes an event type (prefer archive for soft delete)
// @Tags Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Event type is in use"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id} [delete]
func (h *EventTypeHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.repo.Delete(r.Context(), id); err != nil {
		slog.Error("Failed to delete event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete event type")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Archive handles POST /event-types/{id}/archive (using UseCase)
// @Summary Archive an event type
// @Description Archives an event type (soft delete, reversible)
// @Tags Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 200 {object} eventtype.EventType
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Already archived"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/archive [post]
func (h *EventTypeHandler) Archive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.ArchiveEventTypeCommand{ID: id}
	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.archiveUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// ListSchemas handles GET /event-types/{id}/schemas
// @Summary List schema versions for an event type
// @Description Returns all schema versions for an event type
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 200 {array} eventtype.SpecVersion
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas [get]
func (h *EventTypeHandler) ListSchemas(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event type")
		return
	}
	if et == nil {
		WriteNotFound(w, "Event type not found")
		return
	}

	schemas := et.SpecVersions
	if schemas == nil {
		schemas = []eventtype.SpecVersion{}
	}
	WriteJSON(w, http.StatusOK, schemas)
}

// GetSchema handles GET /event-types/{id}/schemas/{version}
// @Summary Get a specific schema version
// @Description Returns a single schema version by version number
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version (e.g., 1.0.0)"
// @Success 200 {object} eventtype.SpecVersion
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas/{version} [get]
func (h *EventTypeHandler) GetSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event type")
		return
	}
	if et == nil {
		WriteNotFound(w, "Event type not found")
		return
	}

	sv := et.FindSpecVersion(version)
	if sv == nil {
		WriteNotFound(w, "Schema version not found")
		return
	}

	WriteJSON(w, http.StatusOK, sv)
}

// AddSchema handles POST /event-types/{id}/schemas (using UseCase)
// @Summary Add a new schema version
// @Description Adds a new schema version to an event type (starts in FINALISING status)
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param request body operations.AddSchemaCommand true "Schema details"
// @Success 201 {object} eventtype.SpecVersion
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Schema version already exists"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas [post]
func (h *EventTypeHandler) AddSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var cmd operations.AddSchemaCommand
	if err := DecodeJSON(r, &cmd); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}
	cmd.EventTypeID = id

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.addSchemaUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusCreated)
}

// FinaliseSchema handles POST /event-types/{id}/schemas/{version}/finalise (using UseCase)
// @Summary Finalise a schema version
// @Description Finalises a schema version, making it immutable and CURRENT
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version (e.g., 1.0.0)"
// @Success 200 {object} eventtype.SpecVersion
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Schema already finalised"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas/{version}/finalise [post]
func (h *EventTypeHandler) FinaliseSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	cmd := operations.FinaliseSchemaCommand{
		EventTypeID: id,
		Version:     version,
	}
	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.finaliseSchemaUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// DeprecateSchema handles POST /event-types/{id}/schemas/{version}/deprecate (using UseCase)
// @Summary Deprecate a schema version
// @Description Deprecates a schema version, marking it as no longer recommended
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version (e.g., 1.0.0)"
// @Success 200 {object} eventtype.SpecVersion
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Schema not in CURRENT status"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas/{version}/deprecate [post]
func (h *EventTypeHandler) DeprecateSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	cmd := operations.DeprecateSchemaCommand{
		EventTypeID: id,
		Version:     version,
	}
	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.deprecateSchemaUseCase.Execute(r.Context(), cmd, execCtx)

	WriteUseCaseResult(w, result, http.StatusOK)
}

// DeleteSchema handles DELETE /event-types/{id}/schemas/{version}
// @Summary Delete a schema version
// @Description Deletes a draft schema version (only FINALISING schemas can be deleted)
// @Tags Event Types - Schemas
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version (e.g., 1.0.0)"
// @Success 204 "No Content"
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse "Cannot delete finalised schema"
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /api/v1/event-types/{id}/schemas/{version} [delete]
func (h *EventTypeHandler) DeleteSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		slog.Error("Failed to get event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event type")
		return
	}
	if et == nil {
		WriteNotFound(w, "Event type not found")
		return
	}

	sv := et.FindSpecVersion(version)
	if sv == nil {
		WriteNotFound(w, "Schema version not found")
		return
	}

	// Only allow deletion of finalising (draft) schemas
	if sv.Status != eventtype.SpecVersionStatusFinalising {
		WriteConflict(w, "Only draft (finalising) schemas can be deleted")
		return
	}

	// Remove the version from the array
	newVersions := make([]eventtype.SpecVersion, 0, len(et.SpecVersions)-1)
	for _, v := range et.SpecVersions {
		if v.Version != version {
			newVersions = append(newVersions, v)
		}
	}
	et.SpecVersions = newVersions
	et.UpdatedAt = time.Now()

	if err := h.repo.Update(r.Context(), et); err != nil {
		slog.Error("Failed to update event type", "error", err, "id", id)
		WriteInternalError(w, "Failed to delete schema version")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getPrincipalID extracts principal ID from request context
func getPrincipalID(r *http.Request) string {
	p := GetPrincipal(r.Context())
	if p != nil {
		return p.ID
	}
	return ""
}
