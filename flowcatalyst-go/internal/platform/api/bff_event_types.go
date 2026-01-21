package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/eventtype"
	"go.flowcatalyst.tech/internal/platform/eventtype/operations"
)

// EventTypeBffHandler handles BFF endpoints for event types
// @Description Web-optimized event type endpoints with string IDs for JavaScript clients
type EventTypeBffHandler struct {
	repo       eventtype.Repository
	unitOfWork common.UnitOfWork

	// UseCases
	createUseCase    *operations.CreateEventTypeUseCase
	updateUseCase    *operations.UpdateEventTypeUseCase
	archiveUseCase   *operations.ArchiveEventTypeUseCase
	addSchemaUseCase *operations.AddSchemaUseCase
	finaliseUseCase  *operations.FinaliseSchemaUseCase
	deprecateUseCase *operations.DeprecateSchemaUseCase
}

// NewEventTypeBffHandler creates a new event type BFF handler
func NewEventTypeBffHandler(repo eventtype.Repository, uow common.UnitOfWork) *EventTypeBffHandler {
	return &EventTypeBffHandler{
		repo:             repo,
		unitOfWork:       uow,
		createUseCase:    operations.NewCreateEventTypeUseCase(repo, uow),
		updateUseCase:    operations.NewUpdateEventTypeUseCase(repo, uow),
		archiveUseCase:   operations.NewArchiveEventTypeUseCase(repo, uow),
		addSchemaUseCase: operations.NewAddSchemaUseCase(repo, uow),
		finaliseUseCase:  operations.NewFinaliseSchemaUseCase(repo, uow),
		deprecateUseCase: operations.NewDeprecateSchemaUseCase(repo, uow),
	}
}

// Routes returns the router for event type BFF endpoints
func (h *EventTypeBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Post("/", h.Create)
	r.Get("/filters/applications", h.GetApplications)
	r.Get("/filters/subdomains", h.GetSubdomains)
	r.Get("/filters/aggregates", h.GetAggregates)
	r.Get("/{id}", h.Get)
	r.Patch("/{id}", h.Update)
	r.Post("/{id}/archive", h.Archive)

	// Schema management
	r.Post("/{id}/schemas", h.AddSchema)
	r.Post("/{id}/schemas/{version}/finalise", h.FinaliseSchema)
	r.Post("/{id}/schemas/{version}/deprecate", h.DeprecateSchema)

	return r
}

// === BFF DTOs - IDs as Strings for JavaScript precision ===

// BffEventTypeResponse represents an event type in BFF API responses
// @Description Event type with string IDs for JavaScript client precision
type BffEventTypeResponse struct {
	ID           string                   `json:"id"`
	Code         string                   `json:"code"`
	Name         string                   `json:"name"`
	Description  string                   `json:"description,omitempty"`
	Status       string                   `json:"status"`
	Application  string                   `json:"application,omitempty"`
	Subdomain    string                   `json:"subdomain,omitempty"`
	Aggregate    string                   `json:"aggregate,omitempty"`
	Event        string                   `json:"event,omitempty"`
	SpecVersions []BffSpecVersionResponse `json:"specVersions,omitempty"`
	CreatedAt    string                   `json:"createdAt,omitempty"`
	UpdatedAt    string                   `json:"updatedAt,omitempty"`
}

// BffSpecVersionResponse represents a schema version in BFF responses
type BffSpecVersionResponse struct {
	Version    string `json:"version"`
	MimeType   string `json:"mimeType,omitempty"`
	SchemaType string `json:"schemaType,omitempty"`
	Status     string `json:"status,omitempty"`
}

// BffEventTypeListResponse wraps a list of event types
type BffEventTypeListResponse struct {
	Items []BffEventTypeResponse `json:"items"`
	Total int                    `json:"total"`
}

// BffFilterOptionsResponse represents available filter options
type BffFilterOptionsResponse struct {
	Options []string `json:"options"`
}

// CreateEventTypeRequest represents a request to create an event type
type CreateEventTypeRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// UpdateEventTypeRequest represents a request to update an event type
type UpdateEventTypeRequest struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

// AddSchemaRequest represents a request to add a schema version
type AddSchemaRequest struct {
	Version    string `json:"version"`
	MimeType   string `json:"mimeType"`
	Schema     string `json:"schema"`
	SchemaType string `json:"schemaType"`
}

// toBffEventTypeResponse converts an EventType to BFF response format
func toBffEventTypeResponse(et *eventtype.EventType) BffEventTypeResponse {
	// Parse code to extract application, subdomain, aggregate, event
	parts := strings.Split(et.Code, ":")
	var app, subdomain, aggregate, event string
	if len(parts) > 0 {
		app = parts[0]
	}
	if len(parts) > 1 {
		subdomain = parts[1]
	}
	if len(parts) > 2 {
		aggregate = parts[2]
	}
	if len(parts) > 3 {
		event = parts[3]
	}

	// Convert spec versions
	specVersions := make([]BffSpecVersionResponse, 0, len(et.SpecVersions))
	for _, sv := range et.SpecVersions {
		specVersions = append(specVersions, BffSpecVersionResponse{
			Version:    sv.Version,
			MimeType:   sv.MimeType,
			SchemaType: string(sv.SchemaType),
			Status:     string(sv.Status),
		})
	}

	return BffEventTypeResponse{
		ID:           et.ID,
		Code:         et.Code,
		Name:         et.Name,
		Description:  et.Description,
		Status:       string(et.Status),
		Application:  app,
		Subdomain:    subdomain,
		Aggregate:    aggregate,
		Event:        event,
		SpecVersions: specVersions,
		CreatedAt:    formatTime(et.CreatedAt),
		UpdatedAt:    formatTime(et.UpdatedAt),
	}
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

// List handles GET /bff/event-types
// @Summary List all event types (BFF)
// @Description Returns all event types with optional filtering. Supports multi-value filtering with comma-separated values.
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param status query string false "Filter by status (ACTIVE, ARCHIVED)"
// @Param application query []string false "Filter by application names (comma-separated)"
// @Param subdomain query []string false "Filter by subdomains (comma-separated)"
// @Param aggregate query []string false "Filter by aggregates (comma-separated)"
// @Success 200 {object} BffEventTypeListResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types [get]
func (h *EventTypeBffHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	status := r.URL.Query().Get("status")
	applications := parseCommaSeparated(r.URL.Query().Get("application"))
	subdomains := parseCommaSeparated(r.URL.Query().Get("subdomain"))
	aggregates := parseCommaSeparated(r.URL.Query().Get("aggregate"))

	// Get all event types
	eventTypes, err := h.repo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to list event types", "error", err)
		WriteInternalError(w, "Failed to list event types")
		return
	}

	// Apply filters
	var filtered []*eventtype.EventType
	for _, et := range eventTypes {
		// Status filter
		if status != "" && string(et.Status) != status {
			continue
		}

		// Parse code for filtering
		parts := strings.Split(et.Code, ":")
		app := ""
		subdomain := ""
		aggregate := ""
		if len(parts) > 0 {
			app = parts[0]
		}
		if len(parts) > 1 {
			subdomain = parts[1]
		}
		if len(parts) > 2 {
			aggregate = parts[2]
		}

		// Application filter
		if len(applications) > 0 && !containsString(applications, app) {
			continue
		}

		// Subdomain filter
		if len(subdomains) > 0 && !containsString(subdomains, subdomain) {
			continue
		}

		// Aggregate filter
		if len(aggregates) > 0 && !containsString(aggregates, aggregate) {
			continue
		}

		filtered = append(filtered, et)
	}

	// Convert to BFF response
	responses := make([]BffEventTypeResponse, len(filtered))
	for i, et := range filtered {
		responses[i] = toBffEventTypeResponse(et)
	}

	WriteJSON(w, http.StatusOK, BffEventTypeListResponse{
		Items: responses,
		Total: len(responses),
	})
}

// Get handles GET /bff/event-types/{id}
// @Summary Get event type by ID (BFF)
// @Description Returns a single event type by its ID
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 200 {object} BffEventTypeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id} [get]
func (h *EventTypeBffHandler) Get(w http.ResponseWriter, r *http.Request) {
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

	WriteJSON(w, http.StatusOK, toBffEventTypeResponse(et))
}

// GetApplications handles GET /bff/event-types/filters/applications
// @Summary Get distinct application names
// @Description Returns distinct application names for filtering
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Success 200 {object} BffFilterOptionsResponse
// @Router /bff/event-types/filters/applications [get]
func (h *EventTypeBffHandler) GetApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventTypes, err := h.repo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to get applications", "error", err)
		WriteInternalError(w, "Failed to get applications")
		return
	}

	// Extract distinct applications
	appSet := make(map[string]struct{})
	for _, et := range eventTypes {
		parts := strings.Split(et.Code, ":")
		if len(parts) > 0 && parts[0] != "" {
			appSet[parts[0]] = struct{}{}
		}
	}

	apps := make([]string, 0, len(appSet))
	for app := range appSet {
		apps = append(apps, app)
	}

	WriteJSON(w, http.StatusOK, BffFilterOptionsResponse{Options: apps})
}

// GetSubdomains handles GET /bff/event-types/filters/subdomains
// @Summary Get distinct subdomains
// @Description Returns distinct subdomains for filtering, optionally filtered by application
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param application query []string false "Filter by application names"
// @Success 200 {object} BffFilterOptionsResponse
// @Router /bff/event-types/filters/subdomains [get]
func (h *EventTypeBffHandler) GetSubdomains(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	applications := parseCommaSeparated(r.URL.Query().Get("application"))

	eventTypes, err := h.repo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to get subdomains", "error", err)
		WriteInternalError(w, "Failed to get subdomains")
		return
	}

	// Extract distinct subdomains
	subdomainSet := make(map[string]struct{})
	for _, et := range eventTypes {
		parts := strings.Split(et.Code, ":")
		if len(parts) < 2 {
			continue
		}

		// Filter by application if provided
		if len(applications) > 0 && !containsString(applications, parts[0]) {
			continue
		}

		if parts[1] != "" {
			subdomainSet[parts[1]] = struct{}{}
		}
	}

	subdomains := make([]string, 0, len(subdomainSet))
	for subdomain := range subdomainSet {
		subdomains = append(subdomains, subdomain)
	}

	WriteJSON(w, http.StatusOK, BffFilterOptionsResponse{Options: subdomains})
}

// GetAggregates handles GET /bff/event-types/filters/aggregates
// @Summary Get distinct aggregates
// @Description Returns distinct aggregates for filtering, optionally filtered by application and subdomain
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param application query []string false "Filter by application names"
// @Param subdomain query []string false "Filter by subdomains"
// @Success 200 {object} BffFilterOptionsResponse
// @Router /bff/event-types/filters/aggregates [get]
func (h *EventTypeBffHandler) GetAggregates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	applications := parseCommaSeparated(r.URL.Query().Get("application"))
	subdomains := parseCommaSeparated(r.URL.Query().Get("subdomain"))

	eventTypes, err := h.repo.FindAll(ctx)
	if err != nil {
		slog.Error("Failed to get aggregates", "error", err)
		WriteInternalError(w, "Failed to get aggregates")
		return
	}

	// Extract distinct aggregates
	aggregateSet := make(map[string]struct{})
	for _, et := range eventTypes {
		parts := strings.Split(et.Code, ":")
		if len(parts) < 3 {
			continue
		}

		// Filter by application if provided
		if len(applications) > 0 && !containsString(applications, parts[0]) {
			continue
		}

		// Filter by subdomain if provided
		if len(subdomains) > 0 && !containsString(subdomains, parts[1]) {
			continue
		}

		if parts[2] != "" {
			aggregateSet[parts[2]] = struct{}{}
		}
	}

	aggregates := make([]string, 0, len(aggregateSet))
	for aggregate := range aggregateSet {
		aggregates = append(aggregates, aggregate)
	}

	WriteJSON(w, http.StatusOK, BffFilterOptionsResponse{Options: aggregates})
}

// Create handles POST /bff/event-types
// @Summary Create a new event type (BFF)
// @Description Creates a new event type
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param request body CreateEventTypeRequest true "Event type details"
// @Success 201 {object} BffEventTypeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types [post]
func (h *EventTypeBffHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateEventTypeRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.CreateEventTypeCommand{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.createUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Type assert to get EventTypeID
	createdEvent, ok := result.Value().(*events.EventTypeCreated)
	if !ok {
		WriteInternalError(w, "Failed to process created event type")
		return
	}

	// Fetch the created event type
	et, err := h.repo.FindByID(r.Context(), createdEvent.EventTypeID)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch created event type")
		return
	}

	WriteJSON(w, http.StatusCreated, toBffEventTypeResponse(et))
}

// Update handles PATCH /bff/event-types/{id}
// @Summary Update an event type (BFF)
// @Description Updates an existing event type
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param request body UpdateEventTypeRequest true "Updated event type details"
// @Success 200 {object} BffEventTypeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id} [patch]
func (h *EventTypeBffHandler) Update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req UpdateEventTypeRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.UpdateEventTypeCommand{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.updateUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the updated event type
	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch updated event type")
		return
	}

	WriteJSON(w, http.StatusOK, toBffEventTypeResponse(et))
}

// Archive handles POST /bff/event-types/{id}/archive
// @Summary Archive an event type (BFF)
// @Description Archives an event type
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Success 200 {object} BffEventTypeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id}/archive [post]
func (h *EventTypeBffHandler) Archive(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	cmd := operations.ArchiveEventTypeCommand{ID: id}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.archiveUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the archived event type
	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch archived event type")
		return
	}

	WriteJSON(w, http.StatusOK, toBffEventTypeResponse(et))
}

// AddSchema handles POST /bff/event-types/{id}/schemas
// @Summary Add a schema version (BFF)
// @Description Adds a new schema version to an event type
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param request body AddSchemaRequest true "Schema details"
// @Success 201 {object} BffEventTypeResponse
// @Failure 400 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id}/schemas [post]
func (h *EventTypeBffHandler) AddSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var req AddSchemaRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	cmd := operations.AddSchemaCommand{
		EventTypeID: id,
		Version:     req.Version,
		MimeType:    req.MimeType,
		Schema:      req.Schema,
		SchemaType:  req.SchemaType,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.addSchemaUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the updated event type
	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch updated event type")
		return
	}

	WriteJSON(w, http.StatusCreated, toBffEventTypeResponse(et))
}

// FinaliseSchema handles POST /bff/event-types/{id}/schemas/{version}/finalise
// @Summary Finalise a schema version (BFF)
// @Description Finalises a schema version, making it immutable
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version"
// @Success 200 {object} BffEventTypeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id}/schemas/{version}/finalise [post]
func (h *EventTypeBffHandler) FinaliseSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	cmd := operations.FinaliseSchemaCommand{
		EventTypeID: id,
		Version:     version,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.finaliseUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the updated event type
	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch updated event type")
		return
	}

	WriteJSON(w, http.StatusOK, toBffEventTypeResponse(et))
}

// DeprecateSchema handles POST /bff/event-types/{id}/schemas/{version}/deprecate
// @Summary Deprecate a schema version (BFF)
// @Description Deprecates a schema version
// @Tags BFF - Event Types
// @Accept json
// @Produce json
// @Param id path string true "Event Type ID"
// @Param version path string true "Schema version"
// @Success 200 {object} BffEventTypeResponse
// @Failure 404 {object} ErrorResponse
// @Failure 409 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Router /bff/event-types/{id}/schemas/{version}/deprecate [post]
func (h *EventTypeBffHandler) DeprecateSchema(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	version := chi.URLParam(r, "version")

	cmd := operations.DeprecateSchemaCommand{
		EventTypeID: id,
		Version:     version,
	}

	execCtx := common.ExecutionContextFromRequest(r, getPrincipalID(r))
	result := h.deprecateUseCase.Execute(r.Context(), cmd, execCtx)

	if result.IsFailure() {
		WriteUseCaseError(w, result.Error())
		return
	}

	// Fetch the updated event type
	et, err := h.repo.FindByID(r.Context(), id)
	if err != nil || et == nil {
		WriteInternalError(w, "Failed to fetch updated event type")
		return
	}

	WriteJSON(w, http.StatusOK, toBffEventTypeResponse(et))
}

// === Helper functions ===

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
