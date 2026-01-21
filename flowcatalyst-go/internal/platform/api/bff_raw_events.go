package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RawEventBffHandler handles debug BFF endpoints for querying raw events
// from the transactional events collection.
// This is for admin/debug purposes only.
// @Description Debug endpoint for raw event store queries
type RawEventBffHandler struct {
	events *mongo.Collection
}

// NewRawEventBffHandler creates a new raw event BFF handler
func NewRawEventBffHandler(db *mongo.Database) *RawEventBffHandler {
	return &RawEventBffHandler{
		events: db.Collection("events"),
	}
}

// Routes returns the router for raw event BFF endpoints
func (h *RawEventBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Get("/{id}", h.Get)

	return r
}

// === DTOs ===

// RawEventResponse represents a raw event for API responses
type RawEventResponse struct {
	ID              string                 `json:"id"`
	SpecVersion     string                 `json:"specVersion"`
	Type            string                 `json:"type"`
	Source          string                 `json:"source"`
	Subject         string                 `json:"subject,omitempty"`
	Time            string                 `json:"time"`
	Data            string                 `json:"data,omitempty"`
	MessageGroup    string                 `json:"messageGroup,omitempty"`
	CorrelationID   string                 `json:"correlationId,omitempty"`
	CausationID     string                 `json:"causationId,omitempty"`
	DeduplicationID string                 `json:"deduplicationId,omitempty"`
	ContextData     []ContextDataResponse  `json:"contextData,omitempty"`
	ClientID        string                 `json:"clientId,omitempty"`
	CreatedAt       string                 `json:"createdAt,omitempty"`
}

// ContextDataResponse represents context data key-value pair
type ContextDataResponse struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PagedRawEventResponse represents a paginated raw event response
type PagedRawEventResponse struct {
	Items      []RawEventResponse `json:"items"`
	Page       int                `json:"page"`
	Size       int                `json:"size"`
	TotalItems int64              `json:"totalItems"`
	TotalPages int                `json:"totalPages"`
}

// List handles GET /api/bff/debug/events
// @Summary List raw events
// @Description List raw events from the transactional collection (debug/admin only)
// @Tags Raw Events (Debug)
// @Accept json
// @Produce json
// @Param page query int false "Page number (0-based)" default(0)
// @Param size query int false "Page size (max 100)" default(20)
// @Success 200 {object} PagedRawEventResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /bff/debug/events [get]
func (h *RawEventBffHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO: Add permission check here
	// authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW_RAW)

	// Parse pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 || size > 100 {
		size = 20
	}

	// Count total
	totalCount, err := h.events.CountDocuments(ctx, bson.M{})
	if err != nil {
		slog.Error("Failed to count raw events", "error", err)
		WriteInternalError(w, "Failed to list raw events")
		return
	}

	// Query raw events
	opts := options.Find().
		SetSkip(int64(page * size)).
		SetLimit(int64(size)).
		SetSort(bson.D{{Key: "time", Value: -1}})

	cursor, err := h.events.Find(ctx, bson.M{}, opts)
	if err != nil {
		slog.Error("Failed to find raw events", "error", err)
		WriteInternalError(w, "Failed to list raw events")
		return
	}
	defer cursor.Close(ctx)

	var events []RawEventResponse
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		events = append(events, docToRawEventResponse(doc))
	}

	if events == nil {
		events = []RawEventResponse{}
	}

	totalPages := int(totalCount) / size
	if int(totalCount)%size > 0 {
		totalPages++
	}

	WriteJSON(w, http.StatusOK, PagedRawEventResponse{
		Items:      events,
		Page:       page,
		Size:       size,
		TotalItems: totalCount,
		TotalPages: totalPages,
	})
}

// Get handles GET /api/bff/debug/events/{id}
// @Summary Get raw event by ID
// @Description Get a single raw event by its ID (debug/admin only)
// @Tags Raw Events (Debug)
// @Accept json
// @Produce json
// @Param id path string true "Event ID"
// @Success 200 {object} RawEventResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /bff/debug/events/{id} [get]
func (h *RawEventBffHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// TODO: Add permission check here
	// authorizationService.requirePermission(principalId, PlatformMessagingPermissions.EVENT_VIEW_RAW)

	var doc bson.M
	err := h.events.FindOne(r.Context(), bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			WriteNotFound(w, "Event not found: "+id)
			return
		}
		slog.Error("Failed to get raw event", "error", err, "id", id)
		WriteInternalError(w, "Failed to get raw event")
		return
	}

	WriteJSON(w, http.StatusOK, docToRawEventResponse(doc))
}

// === Helper functions ===

func docToRawEventResponse(doc bson.M) RawEventResponse {
	response := RawEventResponse{
		ID:              getStringValue(doc, "_id"),
		SpecVersion:     getStringValue(doc, "specVersion"),
		Type:            getStringValue(doc, "type"),
		Source:          getStringValue(doc, "source"),
		Subject:         getStringValue(doc, "subject"),
		Time:            getTimeValue(doc, "time"),
		Data:            getStringValue(doc, "data"),
		MessageGroup:    getStringValue(doc, "messageGroup"),
		CorrelationID:   getStringValue(doc, "correlationId"),
		CausationID:     getStringValue(doc, "causationId"),
		DeduplicationID: getStringValue(doc, "deduplicationId"),
		ClientID:        getStringValue(doc, "clientId"),
		CreatedAt:       getTimeValue(doc, "createdAt"),
	}

	// Handle context data
	if cd, ok := doc["contextData"].([]interface{}); ok {
		response.ContextData = make([]ContextDataResponse, 0, len(cd))
		for _, item := range cd {
			if m, ok := item.(bson.M); ok {
				response.ContextData = append(response.ContextData, ContextDataResponse{
					Key:   getStringValue(m, "key"),
					Value: getStringValue(m, "value"),
				})
			}
		}
	}

	return response
}

func getStringValue(doc bson.M, key string) string {
	if v, ok := doc[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getTimeValue(doc bson.M, key string) string {
	if v, ok := doc[key]; ok {
		switch t := v.(type) {
		case time.Time:
			return t.Format(time.RFC3339)
		case string:
			return t
		}
	}
	return ""
}
