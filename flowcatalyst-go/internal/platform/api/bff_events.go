package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// EventBffHandler handles BFF endpoints for events (read projections)
type EventBffHandler struct {
	eventsRead *mongo.Collection
	clients    *mongo.Collection
}

// NewEventBffHandler creates a new event BFF handler
func NewEventBffHandler(db *mongo.Database) *EventBffHandler {
	return &EventBffHandler{
		eventsRead: db.Collection("events_read"),
		clients:    db.Collection("auth_clients"),
	}
}

// Routes returns the router for event BFF endpoints
func (h *EventBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.Search)
	r.Get("/filter-options", h.FilterOptions)
	r.Get("/{id}", h.Get)

	return r
}

// EventReadDTO represents an event from the read projection
type EventReadDTO struct {
	ID              string            `json:"id"`
	SpecVersion     string            `json:"specVersion"`
	Type            string            `json:"type"`
	Application     string            `json:"application,omitempty"`
	Subdomain       string            `json:"subdomain,omitempty"`
	Aggregate       string            `json:"aggregate,omitempty"`
	Source          string            `json:"source"`
	Subject         string            `json:"subject,omitempty"`
	Time            string            `json:"time"`
	Data            string            `json:"data,omitempty"`
	MessageGroup    string            `json:"messageGroup,omitempty"`
	CorrelationID   string            `json:"correlationId,omitempty"`
	CausationID     string            `json:"causationId,omitempty"`
	DeduplicationID string            `json:"deduplicationId,omitempty"`
	ContextData     []ContextDataDTO  `json:"contextData,omitempty"`
	ClientID        string            `json:"clientId,omitempty"`
	ProjectedAt     string            `json:"projectedAt,omitempty"`
}

// ContextDataDTO represents context data key-value pair
type ContextDataDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// PagedEventResponse represents a paginated event response
type PagedEventResponse struct {
	Items      []EventReadDTO `json:"items"`
	Page       int            `json:"page"`
	Size       int            `json:"size"`
	TotalItems int64          `json:"totalItems"`
	TotalPages int            `json:"totalPages"`
}

// FilterOption represents a filter option with value and label
type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FilterOptionsResponse represents available filter options
type FilterOptionsResponse struct {
	Clients      []FilterOption `json:"clients"`
	Applications []FilterOption `json:"applications"`
	Subdomains   []FilterOption `json:"subdomains"`
	Aggregates   []FilterOption `json:"aggregates"`
	Types        []FilterOption `json:"types"`
}

// Search handles GET /api/bff/events
func (h *EventBffHandler) Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	clientIDs := parseCommaSeparated(r.URL.Query().Get("clientIds"))
	applications := parseCommaSeparated(r.URL.Query().Get("applications"))
	subdomains := parseCommaSeparated(r.URL.Query().Get("subdomains"))
	aggregates := parseCommaSeparated(r.URL.Query().Get("aggregates"))
	types := parseCommaSeparated(r.URL.Query().Get("types"))
	source := r.URL.Query().Get("source")
	subject := r.URL.Query().Get("subject")
	correlationID := r.URL.Query().Get("correlationId")
	messageGroup := r.URL.Query().Get("messageGroup")
	timeAfter := r.URL.Query().Get("timeAfter")
	timeBefore := r.URL.Query().Get("timeBefore")

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 100 {
		size = 20
	}

	// Build filter
	filter := bson.M{}

	if len(clientIDs) > 0 {
		// Handle "null" for platform events
		clientFilter := make([]interface{}, 0, len(clientIDs))
		for _, id := range clientIDs {
			if id == "null" {
				clientFilter = append(clientFilter, nil)
				clientFilter = append(clientFilter, "")
			} else {
				clientFilter = append(clientFilter, id)
			}
		}
		filter["clientId"] = bson.M{"$in": clientFilter}
	}

	if len(applications) > 0 {
		filter["application"] = bson.M{"$in": applications}
	}
	if len(subdomains) > 0 {
		filter["subdomain"] = bson.M{"$in": subdomains}
	}
	if len(aggregates) > 0 {
		filter["aggregate"] = bson.M{"$in": aggregates}
	}
	if len(types) > 0 {
		filter["type"] = bson.M{"$in": types}
	}
	if source != "" {
		filter["source"] = bson.M{"$regex": source, "$options": "i"}
	}
	if subject != "" {
		filter["subject"] = bson.M{"$regex": subject, "$options": "i"}
	}
	if correlationID != "" {
		filter["correlationId"] = correlationID
	}
	if messageGroup != "" {
		filter["messageGroup"] = messageGroup
	}

	// Time range filters
	if timeAfter != "" || timeBefore != "" {
		timeFilter := bson.M{}
		if timeAfter != "" {
			if t, err := time.Parse(time.RFC3339, timeAfter); err == nil {
				timeFilter["$gte"] = t
			}
		}
		if timeBefore != "" {
			if t, err := time.Parse(time.RFC3339, timeBefore); err == nil {
				timeFilter["$lte"] = t
			}
		}
		if len(timeFilter) > 0 {
			filter["time"] = timeFilter
		}
	}

	// Count total
	totalCount, err := h.eventsRead.CountDocuments(ctx, filter)
	if err != nil {
		slog.Error("Failed to count events", "error", err)
		WriteInternalError(w, "Failed to search events")
		return
	}

	// Find events
	opts := options.Find().
		SetSkip(int64(page * size)).
		SetLimit(int64(size)).
		SetSort(bson.D{{Key: "time", Value: -1}})

	cursor, err := h.eventsRead.Find(ctx, filter, opts)
	if err != nil {
		slog.Error("Failed to find events", "error", err)
		WriteInternalError(w, "Failed to search events")
		return
	}
	defer cursor.Close(ctx)

	var events []EventReadDTO
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		events = append(events, docToEventReadDTO(doc))
	}

	if events == nil {
		events = []EventReadDTO{}
	}

	totalPages := int(totalCount) / size
	if int(totalCount)%size > 0 {
		totalPages++
	}

	WriteJSON(w, http.StatusOK, PagedEventResponse{
		Items:      events,
		Page:       page,
		Size:       size,
		TotalItems: totalCount,
		TotalPages: totalPages,
	})
}

// FilterOptions handles GET /api/bff/events/filter-options
func (h *EventBffHandler) FilterOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse current selections for cascading filters
	clientIDs := parseCommaSeparated(r.URL.Query().Get("clientIds"))
	applications := parseCommaSeparated(r.URL.Query().Get("applications"))
	subdomains := parseCommaSeparated(r.URL.Query().Get("subdomains"))
	aggregates := parseCommaSeparated(r.URL.Query().Get("aggregates"))

	// Get clients from the clients collection
	clientOptions := []FilterOption{
		{Value: "null", Label: "Platform (No Client)"},
	}

	clientCursor, err := h.clients.Find(ctx, bson.M{})
	if err == nil {
		defer clientCursor.Close(ctx)
		for clientCursor.Next(ctx) {
			var doc bson.M
			if err := clientCursor.Decode(&doc); err != nil {
				continue
			}
			id := getString(doc, "_id")
			name := getString(doc, "name")
			if name == "" {
				name = getString(doc, "identifier")
			}
			if id != "" {
				clientOptions = append(clientOptions, FilterOption{Value: id, Label: name})
			}
		}
	}

	// Build filter for cascading options
	filter := bson.M{}
	if len(clientIDs) > 0 {
		clientFilter := make([]interface{}, 0)
		for _, id := range clientIDs {
			if id == "null" {
				clientFilter = append(clientFilter, nil, "")
			} else {
				clientFilter = append(clientFilter, id)
			}
		}
		filter["clientId"] = bson.M{"$in": clientFilter}
	}

	// Get distinct applications
	appOptions := getDistinctOptions(ctx, h.eventsRead, "application", filter)

	// Add applications filter for subdomains
	if len(applications) > 0 {
		filter["application"] = bson.M{"$in": applications}
	}
	subdomainOptions := getDistinctOptions(ctx, h.eventsRead, "subdomain", filter)

	// Add subdomains filter for aggregates
	if len(subdomains) > 0 {
		filter["subdomain"] = bson.M{"$in": subdomains}
	}
	aggregateOptions := getDistinctOptions(ctx, h.eventsRead, "aggregate", filter)

	// Add aggregates filter for types
	if len(aggregates) > 0 {
		filter["aggregate"] = bson.M{"$in": aggregates}
	}
	typeOptions := getDistinctOptions(ctx, h.eventsRead, "type", filter)

	WriteJSON(w, http.StatusOK, FilterOptionsResponse{
		Clients:      clientOptions,
		Applications: appOptions,
		Subdomains:   subdomainOptions,
		Aggregates:   aggregateOptions,
		Types:        typeOptions,
	})
}

// Get handles GET /api/bff/events/{id}
func (h *EventBffHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var doc bson.M
	err := h.eventsRead.FindOne(r.Context(), bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			WriteNotFound(w, "Event not found")
			return
		}
		slog.Error("Failed to get event", "error", err, "id", id)
		WriteInternalError(w, "Failed to get event")
		return
	}

	WriteJSON(w, http.StatusOK, docToEventReadDTO(doc))
}

// === Helper functions ===

func parseCommaSeparated(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func getDistinctOptions(ctx context.Context, coll *mongo.Collection, field string, filter bson.M) []FilterOption {
	values, err := coll.Distinct(ctx, field, filter)
	if err != nil {
		return []FilterOption{}
	}

	options := make([]FilterOption, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok && s != "" {
			options = append(options, FilterOption{Value: s, Label: s})
		}
	}
	return options
}

func getString(doc bson.M, key string) string {
	if v, ok := doc[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getTime(doc bson.M, key string) string {
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

func docToEventReadDTO(doc bson.M) EventReadDTO {
	dto := EventReadDTO{
		ID:              getString(doc, "_id"),
		SpecVersion:     getString(doc, "specVersion"),
		Type:            getString(doc, "type"),
		Application:     getString(doc, "application"),
		Subdomain:       getString(doc, "subdomain"),
		Aggregate:       getString(doc, "aggregate"),
		Source:          getString(doc, "source"),
		Subject:         getString(doc, "subject"),
		Time:            getTime(doc, "time"),
		Data:            getString(doc, "data"),
		MessageGroup:    getString(doc, "messageGroup"),
		CorrelationID:   getString(doc, "correlationId"),
		CausationID:     getString(doc, "causationId"),
		DeduplicationID: getString(doc, "deduplicationId"),
		ClientID:        getString(doc, "clientId"),
		ProjectedAt:     getTime(doc, "projectedAt"),
	}

	// Handle context data
	if cd, ok := doc["contextData"].([]interface{}); ok {
		dto.ContextData = make([]ContextDataDTO, 0, len(cd))
		for _, item := range cd {
			if m, ok := item.(bson.M); ok {
				dto.ContextData = append(dto.ContextData, ContextDataDTO{
					Key:   getString(m, "key"),
					Value: getString(m, "value"),
				})
			}
		}
	}

	return dto
}
