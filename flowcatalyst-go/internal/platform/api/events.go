package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/event"
)

// EventHandler handles event endpoints
type EventHandler struct {
	repo event.Repository
}

// NewEventHandler creates a new event handler
func NewEventHandler(repo event.Repository) *EventHandler {
	return &EventHandler{repo: repo}
}

// Routes returns the router for event endpoints
func (h *EventHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.Create)
	r.Post("/batch", h.CreateBatch)
	r.Get("/{id}", h.Get)

	return r
}

// CreateEventRequest represents a request to create an event
type CreateEventRequest struct {
	Type            string               `json:"type"`
	SpecVersion     string               `json:"specVersion,omitempty"`
	Source          string               `json:"source"`
	Subject         string               `json:"subject,omitempty"`
	Time            string               `json:"time,omitempty"`
	Data            string               `json:"data,omitempty"`
	CorrelationID   string               `json:"correlationId,omitempty"`
	CausationID     string               `json:"causationId,omitempty"`
	DeduplicationID string               `json:"deduplicationId,omitempty"`
	MessageGroup    string               `json:"messageGroup,omitempty"`
	ContextData     []event.ContextData  `json:"contextData,omitempty"`
}

// BatchCreateRequest represents a request to create multiple events
type BatchCreateRequest struct {
	Events []CreateEventRequest `json:"events"`
}

// BatchCreateResponse represents the response for batch event creation
type BatchCreateResponse struct {
	Count  int        `json:"count"`
	Events []EventDTO `json:"events"`
}

// EventDTO represents an event for API responses
type EventDTO struct {
	ID              string              `json:"id"`
	SpecVersion     string              `json:"specVersion"`
	Type            string              `json:"type"`
	Source          string              `json:"source"`
	Subject         string              `json:"subject,omitempty"`
	Time            string              `json:"time"`
	Data            string              `json:"data,omitempty"`
	CorrelationID   string              `json:"correlationId,omitempty"`
	CausationID     string              `json:"causationId,omitempty"`
	DeduplicationID string              `json:"deduplicationId,omitempty"`
	MessageGroup    string              `json:"messageGroup,omitempty"`
	ContextData     []event.ContextData `json:"contextData,omitempty"`
	ClientID        string              `json:"clientId,omitempty"`
	CreatedAt       string              `json:"createdAt"`
}

// Create handles POST /api/events
//
//	@Summary		Create a new event
//	@Description	Creates a new domain event. Events are processed asynchronously by subscriptions.
//	@Tags			Events
//	@Accept			json
//	@Produce		json
//	@Param			event	body		CreateEventRequest	true	"Event to create"
//	@Success		201		{object}	EventDTO			"Created event"
//	@Success		200		{object}	EventDTO			"Existing event (idempotent with deduplicationId)"
//	@Failure		400		{object}	ErrorResponse		"Invalid request"
//	@Failure		500		{object}	ErrorResponse		"Internal server error"
//	@Security		BearerAuth
//	@Router			/events [post]
func (h *EventHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if req.Type == "" {
		WriteBadRequest(w, "Type is required")
		return
	}
	if req.Source == "" {
		WriteBadRequest(w, "Source is required")
		return
	}

	// Get client ID from authenticated principal
	p := GetPrincipal(r.Context())
	clientID := ""
	if p != nil {
		clientID = p.ClientID
	}
	// Allow override from header for anchor users
	if p != nil && p.IsAnchor() {
		if headerClientID := r.Header.Get("X-Client-ID"); headerClientID != "" {
			clientID = headerClientID
		}
	}

	e := requestToEvent(&req, clientID)

	if err := h.repo.InsertEvent(r.Context(), e); err != nil {
		if err == event.ErrDuplicateEvent {
			// Idempotent - return existing event if deduplication ID matches
			WriteJSON(w, http.StatusOK, toEventDTO(e))
			return
		}
		slog.Error("Failed to create event", "error", err)
		WriteInternalError(w, "Failed to create event")
		return
	}

	WriteJSON(w, http.StatusCreated, toEventDTO(e))
}

// CreateBatch handles POST /api/events/batch
//
//	@Summary		Create multiple events
//	@Description	Creates multiple events in a single batch (max 100). All events in a batch share the same batch ID.
//	@Tags			Events
//	@Accept			json
//	@Produce		json
//	@Param			events	body		BatchCreateRequest	true	"Events to create"
//	@Success		201		{object}	BatchCreateResponse	"Created events"
//	@Failure		400		{object}	ErrorResponse		"Invalid request"
//	@Failure		500		{object}	ErrorResponse		"Internal server error"
//	@Security		BearerAuth
//	@Router			/events/batch [post]
func (h *EventHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var req BatchCreateRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if len(req.Events) == 0 {
		WriteBadRequest(w, "At least one event is required")
		return
	}
	if len(req.Events) > 100 {
		WriteBadRequest(w, "Maximum 100 events per batch")
		return
	}

	// Get client ID from authenticated principal
	p := GetPrincipal(r.Context())
	clientID := ""
	if p != nil {
		clientID = p.ClientID
	}
	// Allow override from header for anchor users
	if p != nil && p.IsAnchor() {
		if headerClientID := r.Header.Get("X-Client-ID"); headerClientID != "" {
			clientID = headerClientID
		}
	}

	events := make([]*event.Event, len(req.Events))
	for i, er := range req.Events {
		if er.Type == "" {
			WriteBadRequest(w, "Type is required for all events")
			return
		}
		if er.Source == "" {
			WriteBadRequest(w, "Source is required for all events")
			return
		}
		events[i] = requestToEvent(&er, clientID)
	}

	if err := h.repo.InsertEvents(r.Context(), events); err != nil {
		slog.Error("Failed to create events batch", "error", err)
		WriteInternalError(w, "Failed to create events")
		return
	}

	dtos := make([]EventDTO, len(events))
	for i, e := range events {
		dtos[i] = toEventDTO(e)
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"count":  len(dtos),
		"events": dtos,
	})
}

// Get handles GET /api/events/{id}
//
//	@Summary		Get an event by ID
//	@Description	Retrieves a single event by its ID
//	@Tags			Events
//	@Produce		json
//	@Param			id	path		string		true	"Event ID (TSID)"
//	@Success		200	{object}	EventDTO	"Event found"
//	@Failure		404	{object}	ErrorResponse	"Event not found"
//	@Failure		500	{object}	ErrorResponse	"Internal server error"
//	@Security		BearerAuth
//	@Router			/events/{id} [get]
func (h *EventHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	e, err := h.repo.FindEventByID(r.Context(), id)
	if err != nil {
		if err == event.ErrNotFound {
			WriteNotFound(w, "Event not found")
			return
		}
		slog.Error("Failed to get event", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event")
		return
	}

	// Check access for non-anchor users
	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && e.ClientID != p.ClientID {
		WriteNotFound(w, "Event not found")
		return
	}

	WriteJSON(w, http.StatusOK, toEventDTO(e))
}

// requestToEvent converts a create request to an Event
func requestToEvent(req *CreateEventRequest, clientID string) *event.Event {
	e := &event.Event{
		ID:              tsid.Generate(),
		Type:            req.Type,
		SpecVersion:     req.SpecVersion,
		Source:          req.Source,
		Subject:         req.Subject,
		Data:            req.Data,
		CorrelationID:   req.CorrelationID,
		CausationID:     req.CausationID,
		DeduplicationID: req.DeduplicationID,
		MessageGroup:    req.MessageGroup,
		ContextData:     req.ContextData,
		ClientID:        clientID,
	}

	if e.SpecVersion == "" {
		e.SpecVersion = "1.0"
	}

	if req.Time != "" {
		if t, err := time.Parse(time.RFC3339, req.Time); err == nil {
			e.Time = t
		} else {
			e.Time = time.Now()
		}
	} else {
		e.Time = time.Now()
	}

	return e
}

// toEventDTO converts an Event to EventDTO
func toEventDTO(e *event.Event) EventDTO {
	return EventDTO{
		ID:              e.ID,
		SpecVersion:     e.SpecVersion,
		Type:            e.Type,
		Source:          e.Source,
		Subject:         e.Subject,
		Time:            e.Time.Format(time.RFC3339),
		Data:            e.Data,
		CorrelationID:   e.CorrelationID,
		CausationID:     e.CausationID,
		DeduplicationID: e.DeduplicationID,
		MessageGroup:    e.MessageGroup,
		ContextData:     e.ContextData,
		ClientID:        e.ClientID,
		CreatedAt:       e.CreatedAt.Format(time.RFC3339),
	}
}
