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

// DispatchJobBffHandler handles BFF endpoints for dispatch jobs (read projections)
type DispatchJobBffHandler struct {
	dispatchJobsRead *mongo.Collection
	clients          *mongo.Collection
}

// NewDispatchJobBffHandler creates a new dispatch job BFF handler
func NewDispatchJobBffHandler(db *mongo.Database) *DispatchJobBffHandler {
	return &DispatchJobBffHandler{
		dispatchJobsRead: db.Collection("dispatch_jobs_read"),
		clients:          db.Collection("auth_clients"),
	}
}

// Routes returns the router for dispatch job BFF endpoints
func (h *DispatchJobBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.Search)
	r.Get("/filter-options", h.FilterOptions)
	r.Get("/{id}", h.Get)

	return r
}

// DispatchJobReadDTO represents a dispatch job from the read projection
type DispatchJobReadDTO struct {
	ID               string `json:"id"`
	ExternalID       string `json:"externalId,omitempty"`
	Source           string `json:"source,omitempty"`
	Kind             string `json:"kind,omitempty"`
	Code             string `json:"code,omitempty"`
	Subject          string `json:"subject,omitempty"`
	Application      string `json:"application,omitempty"`
	Subdomain        string `json:"subdomain,omitempty"`
	Aggregate        string `json:"aggregate,omitempty"`
	EventID          string `json:"eventId,omitempty"`
	CorrelationID    string `json:"correlationId,omitempty"`
	TargetURL        string `json:"targetUrl,omitempty"`
	Protocol         string `json:"protocol,omitempty"`
	ClientID         string `json:"clientId,omitempty"`
	SubscriptionID   string `json:"subscriptionId,omitempty"`
	ServiceAccountID string `json:"serviceAccountId,omitempty"`
	DispatchPoolID   string `json:"dispatchPoolId,omitempty"`
	MessageGroup     string `json:"messageGroup,omitempty"`
	Mode             string `json:"mode,omitempty"`
	Sequence         *int   `json:"sequence,omitempty"`
	Status           string `json:"status,omitempty"`
	AttemptCount     *int   `json:"attemptCount,omitempty"`
	MaxRetries       *int   `json:"maxRetries,omitempty"`
	LastError        string `json:"lastError,omitempty"`
	TimeoutSeconds   *int   `json:"timeoutSeconds,omitempty"`
	RetryStrategy    string `json:"retryStrategy,omitempty"`
	CreatedAt        string `json:"createdAt,omitempty"`
	UpdatedAt        string `json:"updatedAt,omitempty"`
	ScheduledFor     string `json:"scheduledFor,omitempty"`
	ExpiresAt        string `json:"expiresAt,omitempty"`
	CompletedAt      string `json:"completedAt,omitempty"`
	LastAttemptAt    string `json:"lastAttemptAt,omitempty"`
	DurationMillis   *int64 `json:"durationMillis,omitempty"`
	IdempotencyKey   string `json:"idempotencyKey,omitempty"`
	IsCompleted      *bool  `json:"isCompleted,omitempty"`
	IsTerminal       *bool  `json:"isTerminal,omitempty"`
	ProjectedAt      string `json:"projectedAt,omitempty"`
}

// PagedDispatchJobResponse represents a paginated dispatch job response
type PagedDispatchJobResponse struct {
	Items      []DispatchJobReadDTO `json:"items"`
	Page       int                  `json:"page"`
	Size       int                  `json:"size"`
	TotalItems int64                `json:"totalItems"`
	TotalPages int                  `json:"totalPages"`
}

// DispatchJobFilterOptionsResponse represents available filter options
type DispatchJobFilterOptionsResponse struct {
	Clients      []FilterOption `json:"clients"`
	Applications []FilterOption `json:"applications"`
	Subdomains   []FilterOption `json:"subdomains"`
	Aggregates   []FilterOption `json:"aggregates"`
	Codes        []FilterOption `json:"codes"`
	Statuses     []FilterOption `json:"statuses"`
}

// Search handles GET /api/bff/dispatch-jobs
func (h *DispatchJobBffHandler) Search(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	clientIDs := parseCommaSeparated(r.URL.Query().Get("clientIds"))
	statuses := parseCommaSeparated(r.URL.Query().Get("statuses"))
	applications := parseCommaSeparated(r.URL.Query().Get("applications"))
	subdomains := parseCommaSeparated(r.URL.Query().Get("subdomains"))
	aggregates := parseCommaSeparated(r.URL.Query().Get("aggregates"))
	codes := parseCommaSeparated(r.URL.Query().Get("codes"))
	source := r.URL.Query().Get("source")
	kind := r.URL.Query().Get("kind")
	subscriptionID := r.URL.Query().Get("subscriptionId")
	dispatchPoolID := r.URL.Query().Get("dispatchPoolId")
	messageGroup := r.URL.Query().Get("messageGroup")
	createdAfter := r.URL.Query().Get("createdAfter")
	createdBefore := r.URL.Query().Get("createdBefore")

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
		// Handle "null" for platform jobs
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

	if len(statuses) > 0 {
		filter["status"] = bson.M{"$in": statuses}
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
	if len(codes) > 0 {
		filter["code"] = bson.M{"$in": codes}
	}
	if source != "" {
		filter["source"] = bson.M{"$regex": source, "$options": "i"}
	}
	if kind != "" {
		filter["kind"] = kind
	}
	if subscriptionID != "" {
		filter["subscriptionId"] = subscriptionID
	}
	if dispatchPoolID != "" {
		filter["dispatchPoolId"] = dispatchPoolID
	}
	if messageGroup != "" {
		filter["messageGroup"] = messageGroup
	}

	// Time range filters
	if createdAfter != "" || createdBefore != "" {
		timeFilter := bson.M{}
		if createdAfter != "" {
			if t, err := time.Parse(time.RFC3339, createdAfter); err == nil {
				timeFilter["$gte"] = t
			}
		}
		if createdBefore != "" {
			if t, err := time.Parse(time.RFC3339, createdBefore); err == nil {
				timeFilter["$lte"] = t
			}
		}
		if len(timeFilter) > 0 {
			filter["createdAt"] = timeFilter
		}
	}

	// Count total
	totalCount, err := h.dispatchJobsRead.CountDocuments(ctx, filter)
	if err != nil {
		slog.Error("Failed to count dispatch jobs", "error", err)
		WriteInternalError(w, "Failed to search dispatch jobs")
		return
	}

	// Find dispatch jobs
	opts := options.Find().
		SetSkip(int64(page * size)).
		SetLimit(int64(size)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := h.dispatchJobsRead.Find(ctx, filter, opts)
	if err != nil {
		slog.Error("Failed to find dispatch jobs", "error", err)
		WriteInternalError(w, "Failed to search dispatch jobs")
		return
	}
	defer cursor.Close(ctx)

	var jobs []DispatchJobReadDTO
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		jobs = append(jobs, docToDispatchJobReadDTO(doc))
	}

	if jobs == nil {
		jobs = []DispatchJobReadDTO{}
	}

	totalPages := int(totalCount) / size
	if int(totalCount)%size > 0 {
		totalPages++
	}

	WriteJSON(w, http.StatusOK, PagedDispatchJobResponse{
		Items:      jobs,
		Page:       page,
		Size:       size,
		TotalItems: totalCount,
		TotalPages: totalPages,
	})
}

// FilterOptions handles GET /api/bff/dispatch-jobs/filter-options
func (h *DispatchJobBffHandler) FilterOptions(w http.ResponseWriter, r *http.Request) {
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
	appOptions := getDistinctOptions(ctx, h.dispatchJobsRead, "application", filter)

	// Add applications filter for subdomains
	if len(applications) > 0 {
		filter["application"] = bson.M{"$in": applications}
	}
	subdomainOptions := getDistinctOptions(ctx, h.dispatchJobsRead, "subdomain", filter)

	// Add subdomains filter for aggregates
	if len(subdomains) > 0 {
		filter["subdomain"] = bson.M{"$in": subdomains}
	}
	aggregateOptions := getDistinctOptions(ctx, h.dispatchJobsRead, "aggregate", filter)

	// Add aggregates filter for codes
	if len(aggregates) > 0 {
		filter["aggregate"] = bson.M{"$in": aggregates}
	}
	codeOptions := getDistinctOptions(ctx, h.dispatchJobsRead, "code", filter)

	// Get distinct statuses (not cascaded, always show all available)
	statusOptions := getDistinctOptions(ctx, h.dispatchJobsRead, "status", bson.M{})

	WriteJSON(w, http.StatusOK, DispatchJobFilterOptionsResponse{
		Clients:      clientOptions,
		Applications: appOptions,
		Subdomains:   subdomainOptions,
		Aggregates:   aggregateOptions,
		Codes:        codeOptions,
		Statuses:     statusOptions,
	})
}

// Get handles GET /api/bff/dispatch-jobs/{id}
func (h *DispatchJobBffHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	var doc bson.M
	err := h.dispatchJobsRead.FindOne(r.Context(), bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			WriteNotFound(w, "Dispatch job not found")
			return
		}
		slog.Error("Failed to get dispatch job", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch job")
		return
	}

	WriteJSON(w, http.StatusOK, docToDispatchJobReadDTO(doc))
}

// === Helper functions ===

func docToDispatchJobReadDTO(doc bson.M) DispatchJobReadDTO {
	dto := DispatchJobReadDTO{
		ID:               getString(doc, "_id"),
		ExternalID:       getString(doc, "externalId"),
		Source:           getString(doc, "source"),
		Kind:             getString(doc, "kind"),
		Code:             getString(doc, "code"),
		Subject:          getString(doc, "subject"),
		Application:      getString(doc, "application"),
		Subdomain:        getString(doc, "subdomain"),
		Aggregate:        getString(doc, "aggregate"),
		EventID:          getString(doc, "eventId"),
		CorrelationID:    getString(doc, "correlationId"),
		TargetURL:        getString(doc, "targetUrl"),
		Protocol:         getString(doc, "protocol"),
		ClientID:         getString(doc, "clientId"),
		SubscriptionID:   getString(doc, "subscriptionId"),
		ServiceAccountID: getString(doc, "serviceAccountId"),
		DispatchPoolID:   getString(doc, "dispatchPoolId"),
		MessageGroup:     getString(doc, "messageGroup"),
		Mode:             getString(doc, "mode"),
		Status:           getString(doc, "status"),
		LastError:        getString(doc, "lastError"),
		RetryStrategy:    getString(doc, "retryStrategy"),
		IdempotencyKey:   getString(doc, "idempotencyKey"),
		CreatedAt:        getTime(doc, "createdAt"),
		UpdatedAt:        getTime(doc, "updatedAt"),
		ScheduledFor:     getTime(doc, "scheduledFor"),
		ExpiresAt:        getTime(doc, "expiresAt"),
		CompletedAt:      getTime(doc, "completedAt"),
		LastAttemptAt:    getTime(doc, "lastAttemptAt"),
		ProjectedAt:      getTime(doc, "projectedAt"),
	}

	// Handle optional integer fields
	if v, ok := doc["sequence"]; ok && v != nil {
		if i, ok := v.(int32); ok {
			val := int(i)
			dto.Sequence = &val
		} else if i, ok := v.(int64); ok {
			val := int(i)
			dto.Sequence = &val
		}
	}
	if v, ok := doc["attemptCount"]; ok && v != nil {
		if i, ok := v.(int32); ok {
			val := int(i)
			dto.AttemptCount = &val
		} else if i, ok := v.(int64); ok {
			val := int(i)
			dto.AttemptCount = &val
		}
	}
	if v, ok := doc["maxRetries"]; ok && v != nil {
		if i, ok := v.(int32); ok {
			val := int(i)
			dto.MaxRetries = &val
		} else if i, ok := v.(int64); ok {
			val := int(i)
			dto.MaxRetries = &val
		}
	}
	if v, ok := doc["timeoutSeconds"]; ok && v != nil {
		if i, ok := v.(int32); ok {
			val := int(i)
			dto.TimeoutSeconds = &val
		} else if i, ok := v.(int64); ok {
			val := int(i)
			dto.TimeoutSeconds = &val
		}
	}
	if v, ok := doc["durationMillis"]; ok && v != nil {
		if i, ok := v.(int64); ok {
			dto.DurationMillis = &i
		} else if i, ok := v.(int32); ok {
			val := int64(i)
			dto.DurationMillis = &val
		}
	}

	// Handle optional boolean fields
	if v, ok := doc["isCompleted"]; ok && v != nil {
		if b, ok := v.(bool); ok {
			dto.IsCompleted = &b
		}
	}
	if v, ok := doc["isTerminal"]; ok && v != nil {
		if b, ok := v.(bool); ok {
			dto.IsTerminal = &b
		}
	}

	return dto
}
