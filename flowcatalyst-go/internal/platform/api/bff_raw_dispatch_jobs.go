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

// RawDispatchJobBffHandler handles debug BFF endpoints for querying raw dispatch jobs
// from the transactional dispatch_jobs collection.
// This is for admin/debug purposes only.
// @Description Debug endpoint for raw dispatch job queries
type RawDispatchJobBffHandler struct {
	dispatchJobs *mongo.Collection
}

// NewRawDispatchJobBffHandler creates a new raw dispatch job BFF handler
func NewRawDispatchJobBffHandler(db *mongo.Database) *RawDispatchJobBffHandler {
	return &RawDispatchJobBffHandler{
		dispatchJobs: db.Collection("dispatch_jobs"),
	}
}

// Routes returns the router for raw dispatch job BFF endpoints
func (h *RawDispatchJobBffHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Get("/", h.List)
	r.Get("/{id}", h.Get)

	return r
}

// === DTOs ===

// RawDispatchJobResponse represents a raw dispatch job for API responses
type RawDispatchJobResponse struct {
	ID                 string `json:"id"`
	ExternalID         string `json:"externalId,omitempty"`
	Source             string `json:"source"`
	Kind               string `json:"kind,omitempty"`
	Code               string `json:"code"`
	Subject            string `json:"subject,omitempty"`
	EventID            string `json:"eventId,omitempty"`
	CorrelationID      string `json:"correlationId,omitempty"`
	TargetURL          string `json:"targetUrl"`
	Protocol           string `json:"protocol,omitempty"`
	ClientID           string `json:"clientId,omitempty"`
	SubscriptionID     string `json:"subscriptionId,omitempty"`
	ServiceAccountID   string `json:"serviceAccountId,omitempty"`
	DispatchPoolID     string `json:"dispatchPoolId,omitempty"`
	MessageGroup       string `json:"messageGroup,omitempty"`
	Mode               string `json:"mode,omitempty"`
	Sequence           int    `json:"sequence"`
	Status             string `json:"status"`
	AttemptCount       int    `json:"attemptCount"`
	MaxRetries         int    `json:"maxRetries"`
	LastError          string `json:"lastError,omitempty"`
	TimeoutSeconds     int    `json:"timeoutSeconds"`
	RetryStrategy      string `json:"retryStrategy,omitempty"`
	IdempotencyKey     string `json:"idempotencyKey,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
	ScheduledFor       string `json:"scheduledFor,omitempty"`
	CompletedAt        string `json:"completedAt,omitempty"`
	PayloadContentType string `json:"payloadContentType,omitempty"`
	PayloadLength      int    `json:"payloadLength"`
	AttemptHistoryCount int   `json:"attemptHistoryCount"`
}

// PagedRawDispatchJobResponse represents a paginated raw dispatch job response
type PagedRawDispatchJobResponse struct {
	Items      []RawDispatchJobResponse `json:"items"`
	Page       int                      `json:"page"`
	Size       int                      `json:"size"`
	TotalItems int64                    `json:"totalItems"`
	TotalPages int                      `json:"totalPages"`
}

// List handles GET /api/bff/debug/dispatch-jobs
// @Summary List raw dispatch jobs
// @Description List raw dispatch jobs from the transactional collection (debug/admin only)
// @Tags Raw Dispatch Jobs (Debug)
// @Accept json
// @Produce json
// @Param page query int false "Page number (0-based)" default(0)
// @Param size query int false "Page size (max 100)" default(20)
// @Success 200 {object} PagedRawDispatchJobResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /bff/debug/dispatch-jobs [get]
func (h *RawDispatchJobBffHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// TODO: Add permission check here
	// authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW)

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
	totalCount, err := h.dispatchJobs.CountDocuments(ctx, bson.M{})
	if err != nil {
		slog.Error("Failed to count raw dispatch jobs", "error", err)
		WriteInternalError(w, "Failed to list raw dispatch jobs")
		return
	}

	// Query raw dispatch jobs
	opts := options.Find().
		SetSkip(int64(page * size)).
		SetLimit(int64(size)).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := h.dispatchJobs.Find(ctx, bson.M{}, opts)
	if err != nil {
		slog.Error("Failed to find raw dispatch jobs", "error", err)
		WriteInternalError(w, "Failed to list raw dispatch jobs")
		return
	}
	defer cursor.Close(ctx)

	var jobs []RawDispatchJobResponse
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		jobs = append(jobs, docToRawDispatchJobResponse(doc))
	}

	if jobs == nil {
		jobs = []RawDispatchJobResponse{}
	}

	totalPages := int(totalCount) / size
	if int(totalCount)%size > 0 {
		totalPages++
	}

	WriteJSON(w, http.StatusOK, PagedRawDispatchJobResponse{
		Items:      jobs,
		Page:       page,
		Size:       size,
		TotalItems: totalCount,
		TotalPages: totalPages,
	})
}

// Get handles GET /api/bff/debug/dispatch-jobs/{id}
// @Summary Get raw dispatch job by ID
// @Description Get a single raw dispatch job by its ID (debug/admin only)
// @Tags Raw Dispatch Jobs (Debug)
// @Accept json
// @Produce json
// @Param id path string true "Dispatch Job ID"
// @Success 200 {object} RawDispatchJobResponse
// @Failure 404 {object} ErrorResponse
// @Failure 500 {object} ErrorResponse
// @Security BearerAuth
// @Router /bff/debug/dispatch-jobs/{id} [get]
func (h *RawDispatchJobBffHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// TODO: Add permission check here
	// authorizationService.requirePermission(principalId, PlatformMessagingPermissions.DISPATCH_JOB_VIEW_RAW)

	var doc bson.M
	err := h.dispatchJobs.FindOne(r.Context(), bson.M{"_id": id}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			WriteNotFound(w, "Dispatch job not found: "+id)
			return
		}
		slog.Error("Failed to get raw dispatch job", "error", err, "id", id)
		WriteInternalError(w, "Failed to get raw dispatch job")
		return
	}

	WriteJSON(w, http.StatusOK, docToRawDispatchJobResponse(doc))
}

// === Helper functions ===

func docToRawDispatchJobResponse(doc bson.M) RawDispatchJobResponse {
	// Get payload length without including the full payload
	payloadLength := 0
	if payload, ok := doc["payload"].(string); ok {
		payloadLength = len(payload)
	}

	// Get attempts count
	attemptHistoryCount := 0
	if attempts, ok := doc["attempts"].([]interface{}); ok {
		attemptHistoryCount = len(attempts)
	}

	return RawDispatchJobResponse{
		ID:                  getStringValue(doc, "_id"),
		ExternalID:          getStringValue(doc, "externalId"),
		Source:              getStringValue(doc, "source"),
		Kind:                getStringValue(doc, "kind"),
		Code:                getStringValue(doc, "code"),
		Subject:             getStringValue(doc, "subject"),
		EventID:             getStringValue(doc, "eventId"),
		CorrelationID:       getStringValue(doc, "correlationId"),
		TargetURL:           getStringValue(doc, "targetUrl"),
		Protocol:            getStringValue(doc, "protocol"),
		ClientID:            getStringValue(doc, "clientId"),
		SubscriptionID:      getStringValue(doc, "subscriptionId"),
		ServiceAccountID:    getStringValue(doc, "serviceAccountId"),
		DispatchPoolID:      getStringValue(doc, "dispatchPoolId"),
		MessageGroup:        getStringValue(doc, "messageGroup"),
		Mode:                getStringValue(doc, "mode"),
		Sequence:            getIntValue(doc, "sequence"),
		Status:              getStringValue(doc, "status"),
		AttemptCount:        getIntValue(doc, "attemptCount"),
		MaxRetries:          getIntValue(doc, "maxRetries"),
		LastError:           getStringValue(doc, "lastError"),
		TimeoutSeconds:      getIntValue(doc, "timeoutSeconds"),
		RetryStrategy:       getStringValue(doc, "retryStrategy"),
		IdempotencyKey:      getStringValue(doc, "idempotencyKey"),
		CreatedAt:           getTimeValue(doc, "createdAt"),
		UpdatedAt:           getTimeValue(doc, "updatedAt"),
		ScheduledFor:        getTimeValue(doc, "scheduledFor"),
		CompletedAt:         getTimeValue(doc, "completedAt"),
		PayloadContentType:  getStringValue(doc, "payloadContentType"),
		PayloadLength:       payloadLength,
		AttemptHistoryCount: attemptHistoryCount,
	}
}

func getIntValue(doc bson.M, key string) int {
	if v, ok := doc[key]; ok {
		switch i := v.(type) {
		case int:
			return i
		case int32:
			return int(i)
		case int64:
			return int(i)
		case float64:
			return int(i)
		}
	}
	return 0
}

func getTimeValueFromDoc(doc bson.M, key string) string {
	if v, ok := doc[key]; ok {
		switch t := v.(type) {
		case time.Time:
			if !t.IsZero() {
				return t.Format(time.RFC3339)
			}
		case string:
			return t
		}
	}
	return ""
}
