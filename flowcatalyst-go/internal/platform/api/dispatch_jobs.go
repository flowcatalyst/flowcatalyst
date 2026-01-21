package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/dispatchjob"
)

// DispatchJobHandler handles dispatch job endpoints
type DispatchJobHandler struct {
	repo dispatchjob.Repository
}

// NewDispatchJobHandler creates a new dispatch job handler
func NewDispatchJobHandler(repo dispatchjob.Repository) *DispatchJobHandler {
	return &DispatchJobHandler{repo: repo}
}

// Routes returns the router for dispatch job endpoints
func (h *DispatchJobHandler) Routes() chi.Router {
	r := chi.NewRouter()

	r.Post("/", h.Create)
	r.Post("/batch", h.CreateBatch)
	r.Get("/", h.Search)
	r.Get("/{id}", h.Get)
	r.Get("/{id}/attempts", h.GetAttempts)

	return r
}

// CreateDispatchJobRequest represents a request to create a dispatch job
type CreateDispatchJobRequest struct {
	Source             string            `json:"source"`
	Kind               string            `json:"kind,omitempty"`
	Code               string            `json:"code"`
	Subject            string            `json:"subject,omitempty"`
	EventID            string            `json:"eventId,omitempty"`
	CorrelationID      string            `json:"correlationId,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	TargetURL          string            `json:"targetUrl"`
	Protocol           string            `json:"protocol,omitempty"`
	Headers            map[string]string `json:"headers,omitempty"`
	Payload            string            `json:"payload"`
	PayloadContentType string            `json:"payloadContentType,omitempty"`
	DataOnly           bool              `json:"dataOnly,omitempty"`
	ServiceAccountID   string            `json:"serviceAccountId"`
	ClientID           string            `json:"clientId,omitempty"`
	SubscriptionID     string            `json:"subscriptionId,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	DispatchPoolID     string            `json:"dispatchPoolId,omitempty"`
	MessageGroup       string            `json:"messageGroup,omitempty"`
	Sequence           int               `json:"sequence,omitempty"`
	TimeoutSeconds     int               `json:"timeoutSeconds,omitempty"`
	SchemaID           string            `json:"schemaId,omitempty"`
	MaxRetries         int               `json:"maxRetries,omitempty"`
	RetryStrategy      string            `json:"retryStrategy,omitempty"`
	ScheduledFor       string            `json:"scheduledFor,omitempty"`
	ExpiresAt          string            `json:"expiresAt,omitempty"`
	IdempotencyKey     string            `json:"idempotencyKey,omitempty"`
	ExternalID         string            `json:"externalId,omitempty"`
	QueueURL           string            `json:"queueUrl,omitempty"`
}

// DispatchJobDTO represents a dispatch job for API responses
type DispatchJobDTO struct {
	ID                 string            `json:"id"`
	ExternalID         string            `json:"externalId,omitempty"`
	Source             string            `json:"source"`
	Kind               string            `json:"kind"`
	Code               string            `json:"code"`
	Subject            string            `json:"subject,omitempty"`
	EventID            string            `json:"eventId,omitempty"`
	CorrelationID      string            `json:"correlationId,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
	TargetURL          string            `json:"targetUrl"`
	Protocol           string            `json:"protocol"`
	Headers            map[string]string `json:"headers,omitempty"`
	PayloadContentType string            `json:"payloadContentType,omitempty"`
	DataOnly           bool              `json:"dataOnly"`
	ServiceAccountID   string            `json:"serviceAccountId,omitempty"`
	ClientID           string            `json:"clientId,omitempty"`
	SubscriptionID     string            `json:"subscriptionId,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	DispatchPoolID     string            `json:"dispatchPoolId,omitempty"`
	MessageGroup       string            `json:"messageGroup,omitempty"`
	Sequence           int               `json:"sequence"`
	TimeoutSeconds     int               `json:"timeoutSeconds"`
	SchemaID           string            `json:"schemaId,omitempty"`
	Status             string            `json:"status"`
	MaxRetries         int               `json:"maxRetries"`
	RetryStrategy      string            `json:"retryStrategy,omitempty"`
	ScheduledFor       string            `json:"scheduledFor,omitempty"`
	ExpiresAt          string            `json:"expiresAt,omitempty"`
	AttemptCount       int               `json:"attemptCount"`
	LastAttemptAt      string            `json:"lastAttemptAt,omitempty"`
	CompletedAt        string            `json:"completedAt,omitempty"`
	DurationMillis     int64             `json:"durationMillis,omitempty"`
	LastError          string            `json:"lastError,omitempty"`
	IdempotencyKey     string            `json:"idempotencyKey,omitempty"`
	CreatedAt          string            `json:"createdAt"`
	UpdatedAt          string            `json:"updatedAt"`
}

// DispatchAttemptDTO represents a dispatch attempt for API responses
type DispatchAttemptDTO struct {
	ID              string `json:"id"`
	AttemptNumber   int    `json:"attemptNumber"`
	AttemptedAt     string `json:"attemptedAt"`
	CompletedAt     string `json:"completedAt,omitempty"`
	DurationMillis  int64  `json:"durationMillis,omitempty"`
	Status          string `json:"status"`
	ResponseCode    int    `json:"responseCode,omitempty"`
	ResponseBody    string `json:"responseBody,omitempty"`
	ErrorMessage    string `json:"errorMessage,omitempty"`
	ErrorStackTrace string `json:"errorStackTrace,omitempty"`
	ErrorType       string `json:"errorType,omitempty"`
	CreatedAt       string `json:"createdAt"`
}

// PagedDispatchJobDTOResponse represents a paginated dispatch job response
type PagedDispatchJobDTOResponse struct {
	Items      []DispatchJobDTO `json:"items"`
	Page       int              `json:"page"`
	Size       int              `json:"size"`
	TotalItems int64            `json:"totalItems"`
	TotalPages int              `json:"totalPages"`
}

// BatchDispatchJobResponse represents the response for batch dispatch job creation
type BatchDispatchJobResponse struct {
	Jobs  []DispatchJobDTO `json:"jobs"`
	Count int              `json:"count"`
}

// Create handles POST /api/dispatch/jobs
//
//	@Summary		Create a dispatch job
//	@Description	Creates a new webhook dispatch job for HTTP delivery
//	@Tags			Dispatch Jobs
//	@Accept			json
//	@Produce		json
//	@Param			job	body		CreateDispatchJobRequest	true	"Dispatch job to create"
//	@Success		201	{object}	DispatchJobDTO				"Created dispatch job"
//	@Success		200	{object}	DispatchJobDTO				"Existing job (idempotent with idempotencyKey)"
//	@Failure		400	{object}	ErrorResponse				"Invalid request"
//	@Failure		500	{object}	ErrorResponse				"Internal server error"
//	@Security		BearerAuth
//	@Router			/dispatch/jobs [post]
func (h *DispatchJobHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateDispatchJobRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Source == "" {
		WriteBadRequest(w, "source is required")
		return
	}
	if req.Code == "" {
		WriteBadRequest(w, "code is required")
		return
	}
	if req.TargetURL == "" {
		WriteBadRequest(w, "targetUrl is required")
		return
	}
	if req.Payload == "" {
		WriteBadRequest(w, "payload is required")
		return
	}
	if req.ServiceAccountID == "" {
		WriteBadRequest(w, "serviceAccountId is required")
		return
	}

	// Check for idempotency
	if req.IdempotencyKey != "" {
		existing, err := h.repo.FindByIdempotencyKey(r.Context(), req.IdempotencyKey)
		if err == nil && existing != nil {
			WriteJSON(w, http.StatusOK, toDispatchJobDTO(existing))
			return
		}
	}

	// Get client ID from authenticated principal if not provided
	p := GetPrincipal(r.Context())
	clientID := req.ClientID
	if clientID == "" && p != nil {
		clientID = p.ClientID
	}
	// Allow override from header for anchor users
	if p != nil && p.IsAnchor() {
		if headerClientID := r.Header.Get("X-Client-ID"); headerClientID != "" {
			clientID = headerClientID
		}
	}

	job := requestToDispatchJob(&req, clientID)

	if err := h.repo.Insert(r.Context(), job); err != nil {
		if err == dispatchjob.ErrDuplicateJob {
			WriteJSON(w, http.StatusOK, toDispatchJobDTO(job))
			return
		}
		slog.Error("Failed to create dispatch job", "error", err)
		WriteInternalError(w, "Failed to create dispatch job")
		return
	}

	WriteJSON(w, http.StatusCreated, toDispatchJobDTO(job))
}

// CreateBatch handles POST /api/dispatch/jobs/batch
//
//	@Summary		Create multiple dispatch jobs
//	@Description	Creates multiple dispatch jobs in a batch (max 100)
//	@Tags			Dispatch Jobs
//	@Accept			json
//	@Produce		json
//	@Param			jobs	body		[]CreateDispatchJobRequest	true	"Dispatch jobs to create"
//	@Success		201		{object}	BatchDispatchJobResponse	"Created dispatch jobs"
//	@Failure		400		{object}	ErrorResponse				"Invalid request"
//	@Failure		500		{object}	ErrorResponse				"Internal server error"
//	@Security		BearerAuth
//	@Router			/dispatch/jobs/batch [post]
func (h *DispatchJobHandler) CreateBatch(w http.ResponseWriter, r *http.Request) {
	var requests []CreateDispatchJobRequest
	if err := DecodeJSON(r, &requests); err != nil {
		WriteBadRequest(w, "Invalid request body")
		return
	}

	if len(requests) == 0 {
		WriteBadRequest(w, "At least one dispatch job is required")
		return
	}
	if len(requests) > 100 {
		WriteBadRequest(w, "Maximum 100 dispatch jobs per batch")
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

	jobs := make([]*dispatchjob.DispatchJob, len(requests))
	for i, req := range requests {
		if req.Source == "" {
			WriteBadRequest(w, "source is required for all dispatch jobs")
			return
		}
		if req.Code == "" {
			WriteBadRequest(w, "code is required for all dispatch jobs")
			return
		}
		if req.TargetURL == "" {
			WriteBadRequest(w, "targetUrl is required for all dispatch jobs")
			return
		}
		if req.Payload == "" {
			WriteBadRequest(w, "payload is required for all dispatch jobs")
			return
		}
		if req.ServiceAccountID == "" {
			WriteBadRequest(w, "serviceAccountId is required for all dispatch jobs")
			return
		}

		jobClientID := req.ClientID
		if jobClientID == "" {
			jobClientID = clientID
		}
		jobs[i] = requestToDispatchJob(&req, jobClientID)
	}

	if err := h.repo.InsertMany(r.Context(), jobs); err != nil {
		slog.Error("Failed to create dispatch jobs batch", "error", err)
		WriteInternalError(w, "Failed to create dispatch jobs")
		return
	}

	dtos := make([]DispatchJobDTO, len(jobs))
	for i, job := range jobs {
		dtos[i] = toDispatchJobDTO(job)
	}

	WriteJSON(w, http.StatusCreated, map[string]interface{}{
		"jobs":  dtos,
		"count": len(dtos),
	})
}

// Search handles GET /api/dispatch/jobs
func (h *DispatchJobHandler) Search(w http.ResponseWriter, r *http.Request) {
	// Note: This is a simple implementation. The full search with filters
	// is in the BFF handler which queries the read projection.
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 0 {
		page = 0
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size <= 0 || size > 100 {
		size = 20
	}

	// For the write collection, just return pending jobs for now
	// The full search should use the BFF endpoint
	jobs, err := h.repo.FindPending(r.Context(), int64(size))
	if err != nil {
		slog.Error("Failed to search dispatch jobs", "error", err)
		WriteInternalError(w, "Failed to search dispatch jobs")
		return
	}

	dtos := make([]DispatchJobDTO, len(jobs))
	for i, job := range jobs {
		dtos[i] = toDispatchJobDTO(job)
	}

	// Simplified response - full pagination in BFF
	WriteJSON(w, http.StatusOK, PagedDispatchJobDTOResponse{
		Items:      dtos,
		Page:       page,
		Size:       size,
		TotalItems: int64(len(dtos)),
		TotalPages: 1,
	})
}

// Get handles GET /api/dispatch/jobs/{id}
//
//	@Summary		Get a dispatch job by ID
//	@Description	Retrieves a single dispatch job by its ID
//	@Tags			Dispatch Jobs
//	@Produce		json
//	@Param			id	path		string			true	"Dispatch job ID (TSID)"
//	@Success		200	{object}	DispatchJobDTO	"Dispatch job found"
//	@Failure		404	{object}	ErrorResponse	"Dispatch job not found"
//	@Failure		500	{object}	ErrorResponse	"Internal server error"
//	@Security		BearerAuth
//	@Router			/dispatch/jobs/{id} [get]
func (h *DispatchJobHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == dispatchjob.ErrNotFound {
			WriteNotFound(w, "Dispatch job not found")
			return
		}
		slog.Error("Failed to get dispatch job", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch job")
		return
	}

	// Check access for non-anchor users
	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && job.ClientID != p.ClientID {
		WriteNotFound(w, "Dispatch job not found")
		return
	}

	WriteJSON(w, http.StatusOK, toDispatchJobDTO(job))
}

// GetAttempts handles GET /api/dispatch/jobs/{id}/attempts
//
//	@Summary		Get dispatch attempts for a job
//	@Description	Retrieves all delivery attempts for a dispatch job
//	@Tags			Dispatch Jobs
//	@Produce		json
//	@Param			id	path		string					true	"Dispatch job ID (TSID)"
//	@Success		200	{array}		DispatchAttemptDTO		"Dispatch attempts"
//	@Failure		404	{object}	ErrorResponse			"Dispatch job not found"
//	@Failure		500	{object}	ErrorResponse			"Internal server error"
//	@Security		BearerAuth
//	@Router			/dispatch/jobs/{id}/attempts [get]
func (h *DispatchJobHandler) GetAttempts(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	job, err := h.repo.FindByID(r.Context(), id)
	if err != nil {
		if err == dispatchjob.ErrNotFound {
			WriteNotFound(w, "Dispatch job not found")
			return
		}
		slog.Error("Failed to get dispatch job", "error", err, "id", id)
		WriteInternalError(w, "Failed to get dispatch job")
		return
	}

	// Check access for non-anchor users
	p := GetPrincipal(r.Context())
	if p != nil && !p.IsAnchor() && job.ClientID != p.ClientID {
		WriteNotFound(w, "Dispatch job not found")
		return
	}

	attempts := make([]DispatchAttemptDTO, len(job.Attempts))
	for i, a := range job.Attempts {
		attempts[i] = DispatchAttemptDTO{
			ID:              a.ID,
			AttemptNumber:   a.AttemptNumber,
			AttemptedAt:     a.AttemptedAt.Format(time.RFC3339),
			CompletedAt:     formatTimeIfNotZero(a.CompletedAt),
			DurationMillis:  a.DurationMillis,
			Status:          string(a.Status),
			ResponseCode:    a.ResponseCode,
			ResponseBody:    a.ResponseBody,
			ErrorMessage:    a.ErrorMessage,
			ErrorStackTrace: a.ErrorStackTrace,
			ErrorType:       string(a.ErrorType),
			CreatedAt:       a.CreatedAt.Format(time.RFC3339),
		}
	}

	WriteJSON(w, http.StatusOK, attempts)
}

// requestToDispatchJob converts a create request to a DispatchJob
func requestToDispatchJob(req *CreateDispatchJobRequest, clientID string) *dispatchjob.DispatchJob {
	job := &dispatchjob.DispatchJob{
		ID:                 tsid.Generate(),
		ExternalID:         req.ExternalID,
		Source:             req.Source,
		Code:               req.Code,
		Subject:            req.Subject,
		EventID:            req.EventID,
		CorrelationID:      req.CorrelationID,
		TargetURL:          req.TargetURL,
		Headers:            req.Headers,
		Payload:            req.Payload,
		PayloadContentType: req.PayloadContentType,
		DataOnly:           req.DataOnly,
		ServiceAccountID:   req.ServiceAccountID,
		ClientID:           clientID,
		SubscriptionID:     req.SubscriptionID,
		DispatchPoolID:     req.DispatchPoolID,
		MessageGroup:       req.MessageGroup,
		Sequence:           req.Sequence,
		TimeoutSeconds:     req.TimeoutSeconds,
		SchemaID:           req.SchemaID,
		MaxRetries:         req.MaxRetries,
		RetryStrategy:      req.RetryStrategy,
		IdempotencyKey:     req.IdempotencyKey,
		Status:             dispatchjob.DispatchStatusPending,
	}

	// Set kind
	if req.Kind != "" {
		job.Kind = dispatchjob.DispatchKind(req.Kind)
	} else {
		job.Kind = dispatchjob.DispatchKindEvent
	}

	// Set protocol
	if req.Protocol != "" {
		job.Protocol = dispatchjob.DispatchProtocol(req.Protocol)
	} else {
		job.Protocol = dispatchjob.DispatchProtocolHTTPWebhook
	}

	// Set mode
	if req.Mode != "" {
		job.Mode = dispatchjob.DispatchMode(req.Mode)
	}

	// Convert metadata
	if req.Metadata != nil {
		job.Metadata = make([]dispatchjob.DispatchJobMetadata, 0, len(req.Metadata))
		for k, v := range req.Metadata {
			job.Metadata = append(job.Metadata, dispatchjob.DispatchJobMetadata{
				ID:    tsid.Generate(),
				Key:   k,
				Value: v,
			})
		}
	}

	// Parse scheduled time
	if req.ScheduledFor != "" {
		if t, err := time.Parse(time.RFC3339, req.ScheduledFor); err == nil {
			job.ScheduledFor = t
		}
	}

	// Parse expiry time
	if req.ExpiresAt != "" {
		if t, err := time.Parse(time.RFC3339, req.ExpiresAt); err == nil {
			job.ExpiresAt = t
		}
	}

	// Set defaults
	if job.MaxRetries == 0 {
		job.MaxRetries = 3
	}
	if job.TimeoutSeconds == 0 {
		job.TimeoutSeconds = 30
	}
	if job.PayloadContentType == "" {
		job.PayloadContentType = "application/json"
	}

	return job
}

// toDispatchJobDTO converts a DispatchJob to DispatchJobDTO
func toDispatchJobDTO(job *dispatchjob.DispatchJob) DispatchJobDTO {
	dto := DispatchJobDTO{
		ID:                 job.ID,
		ExternalID:         job.ExternalID,
		Source:             job.Source,
		Kind:               string(job.Kind),
		Code:               job.Code,
		Subject:            job.Subject,
		EventID:            job.EventID,
		CorrelationID:      job.CorrelationID,
		TargetURL:          job.TargetURL,
		Protocol:           string(job.Protocol),
		Headers:            job.Headers,
		PayloadContentType: job.PayloadContentType,
		DataOnly:           job.DataOnly,
		ServiceAccountID:   job.ServiceAccountID,
		ClientID:           job.ClientID,
		SubscriptionID:     job.SubscriptionID,
		Mode:               string(job.Mode),
		DispatchPoolID:     job.DispatchPoolID,
		MessageGroup:       job.MessageGroup,
		Sequence:           job.Sequence,
		TimeoutSeconds:     job.TimeoutSeconds,
		SchemaID:           job.SchemaID,
		Status:             string(job.Status),
		MaxRetries:         job.MaxRetries,
		RetryStrategy:      job.RetryStrategy,
		AttemptCount:       job.AttemptCount,
		DurationMillis:     job.DurationMillis,
		LastError:          job.LastError,
		IdempotencyKey:     job.IdempotencyKey,
		CreatedAt:          job.CreatedAt.Format(time.RFC3339),
		UpdatedAt:          job.UpdatedAt.Format(time.RFC3339),
	}

	// Convert metadata to map
	if job.Metadata != nil {
		dto.Metadata = make(map[string]string)
		for _, m := range job.Metadata {
			dto.Metadata[m.Key] = m.Value
		}
	}

	// Format optional time fields
	if !job.ScheduledFor.IsZero() {
		dto.ScheduledFor = job.ScheduledFor.Format(time.RFC3339)
	}
	if !job.ExpiresAt.IsZero() {
		dto.ExpiresAt = job.ExpiresAt.Format(time.RFC3339)
	}
	if !job.LastAttemptAt.IsZero() {
		dto.LastAttemptAt = job.LastAttemptAt.Format(time.RFC3339)
	}
	if !job.CompletedAt.IsZero() {
		dto.CompletedAt = job.CompletedAt.Format(time.RFC3339)
	}

	return dto
}

// formatTimeIfNotZero formats a time or returns empty string if zero
func formatTimeIfNotZero(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}
