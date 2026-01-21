package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"log/slog"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/dispatchjob"
	"go.flowcatalyst.tech/internal/router/model"
)

// DispatchProcessingHandler handles the internal dispatch processing endpoint
// that is called by the message router.
//
// This matches Java's DispatchProcessingResource
type DispatchProcessingHandler struct {
	repo        dispatchjob.Repository
	authService *dispatchjob.DispatchAuthService
	httpClient  *http.Client
}

// NewDispatchProcessingHandler creates a new dispatch processing handler
func NewDispatchProcessingHandler(
	repo dispatchjob.Repository,
	authService *dispatchjob.DispatchAuthService,
) *DispatchProcessingHandler {
	return &DispatchProcessingHandler{
		repo:        repo,
		authService: authService,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Routes returns the router for dispatch processing endpoint
func (h *DispatchProcessingHandler) Routes() chi.Router {
	r := chi.NewRouter()
	r.Post("/", h.Process)
	return r
}

// Process handles POST /api/dispatch/process
// This is the internal endpoint called by the message router.
//
//	@Summary		Process a dispatch job (internal endpoint called by message router)
//	@Description	Internal endpoint that executes webhook dispatch and records attempts.
//	@Description	Requires HMAC-SHA256 authentication via Bearer token.
//	@Tags			Dispatch Processing
//	@Accept			json
//	@Produce		json
//	@Param			request	body		model.ProcessRequest	true	"Processing request with messageId"
//	@Success		200		{object}	model.ProcessResponse	"Job processed (check ack field for success/failure)"
//	@Failure		401		{object}	model.ProcessResponse	"Invalid or missing authentication token"
//	@Failure		500		{object}	model.ProcessResponse	"Internal error during processing"
//	@Router			/dispatch/process [post]
func (h *DispatchProcessingHandler) Process(w http.ResponseWriter, r *http.Request) {
	// Parse request
	var req model.ProcessRequest
	if err := DecodeJSON(r, &req); err != nil {
		slog.Warn("Failed to parse dispatch process request", "error", err)
		WriteJSON(w, http.StatusBadRequest, model.NewNackResponse("Invalid request body"))
		return
	}

	slog.Info("Received dispatch job processing request", "messageId", req.MessageID)

	// Extract and validate auth token
	token := extractBearerTokenFromHeader(r.Header.Get("Authorization"))
	if token == "" {
		slog.Warn("Dispatch process request missing Authorization header", "messageId", req.MessageID)
		WriteJSON(w, http.StatusUnauthorized, model.NewNackResponse("Missing Authorization header"))
		return
	}

	if err := h.authService.ValidateAuthToken(req.MessageID, token); err != nil {
		slog.Warn("Dispatch process auth failed", "messageId", req.MessageID)
		WriteJSON(w, http.StatusUnauthorized, model.NewNackResponse("Invalid auth token"))
		return
	}

	// Process the dispatch job
	result, err := h.processDispatchJob(r.Context(), req.MessageID)
	if err != nil {
		slog.Error("Error processing dispatch job", "error", err, "messageId", req.MessageID)
		WriteJSON(w, http.StatusInternalServerError, model.NewNackResponse(err.Error()))
		return
	}

	WriteJSON(w, http.StatusOK, result)
}

// processDispatchJob processes a single dispatch job
// Returns a ProcessResponse indicating whether to ACK or NACK the message
func (h *DispatchProcessingHandler) processDispatchJob(ctx context.Context, dispatchJobID string) (*model.ProcessResponse, error) {
	// Fetch the dispatch job
	job, err := h.repo.FindByID(ctx, dispatchJobID)
	if err != nil {
		if err == dispatchjob.ErrNotFound {
			// Job not found - ACK it since we can't process (similar to Laravel behavior)
			slog.Warn("Dispatch job not found", "jobId", dispatchJobID)
			return model.NewAckResponse("Cannot find record."), nil
		}
		return nil, err
	}

	// Check if already completed or cancelled
	if job.IsTerminal() {
		slog.Info("Job already in terminal state", "jobId", dispatchJobID, "status", string(job.Status))
		return model.NewAckResponse("Job already completed"), nil
	}

	// Check if expired
	if job.IsExpired() {
		slog.Info("Job has expired", "jobId", dispatchJobID)
		// Update status to cancelled
		h.repo.UpdateStatus(ctx, dispatchJobID, dispatchjob.DispatchStatusCancelled)
		return model.NewAckResponse("Job expired"), nil
	}

	// Check if scheduled for later (notBefore check)
	if !job.ScheduledFor.IsZero() && time.Now().Before(job.ScheduledFor) {
		// Not ready yet - NACK with delay
		delaySeconds := int(time.Until(job.ScheduledFor).Seconds())
		if delaySeconds > model.MaxDelaySeconds {
			delaySeconds = model.MaxDelaySeconds
		}
		if delaySeconds < 1 {
			delaySeconds = 1
		}
		slog.Info("Job not ready yet (notBefore)", "jobId", dispatchJobID, "delaySeconds", delaySeconds)
		return model.NewNackWithDelayResponse("notBefore time not reached", delaySeconds), nil
	}

	// Update status to IN_PROGRESS
	h.repo.UpdateStatus(ctx, dispatchJobID, dispatchjob.DispatchStatusInProgress)

	// Execute the webhook delivery
	attempt := h.executeWebhook(ctx, job)

	// Record the attempt
	job.Attempts = append(job.Attempts, *attempt)
	job.AttemptCount++
	job.LastAttemptAt = attempt.AttemptedAt

	// Determine outcome
	if attempt.Status == dispatchjob.DispatchAttemptStatusSuccess {
		// Success - mark as completed
		job.Status = dispatchjob.DispatchStatusCompleted
		job.CompletedAt = time.Now()
		job.DurationMillis = time.Since(job.CreatedAt).Milliseconds()
		h.repo.Update(ctx, job)
		return model.NewAckResponse("Success"), nil
	}

	// Failure - check if can retry
	job.LastError = attempt.ErrorMessage

	if job.AttemptCount >= job.MaxRetries {
		// Max retries reached - mark as error (terminal state)
		job.Status = dispatchjob.DispatchStatusError
		h.repo.Update(ctx, job)
		slog.Warn("Max retries reached, marking as ERROR", "jobId", dispatchJobID, "attempts", job.AttemptCount)
		return model.NewAckResponse("Max retries exceeded"), nil
	}

	// Can retry - calculate backoff delay
	delaySeconds := h.calculateBackoffDelay(job.AttemptCount)
	job.Status = dispatchjob.DispatchStatusPending // Reset to pending for retry
	h.repo.Update(ctx, job)

	slog.Info("Attempt failed, will retry", "jobId", dispatchJobID, "attempt", job.AttemptCount, "maxRetries", job.MaxRetries, "delaySeconds", delaySeconds)

	return model.NewNackWithDelayResponse(attempt.ErrorMessage, delaySeconds), nil
}

// executeWebhook executes the webhook delivery and returns the attempt record
func (h *DispatchProcessingHandler) executeWebhook(ctx context.Context, job *dispatchjob.DispatchJob) *dispatchjob.DispatchAttempt {
	startTime := time.Now()
	attempt := &dispatchjob.DispatchAttempt{
		ID:            tsid.Generate(),
		AttemptNumber: job.AttemptCount + 1,
		AttemptedAt:   startTime,
		CreatedAt:     startTime,
	}

	// Create timeout context
	timeout := time.Duration(job.TimeoutSeconds) * time.Second
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Create request
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, job.TargetURL, strings.NewReader(job.Payload))
	if err != nil {
		attempt.Status = dispatchjob.DispatchAttemptStatusClientError
		attempt.ErrorMessage = "Failed to create request: " + err.Error()
		attempt.ErrorType = dispatchjob.ErrorTypePermanent
		return h.finalizeAttempt(attempt, startTime)
	}

	// Set headers
	req.Header.Set("Content-Type", job.PayloadContentType)
	if job.PayloadContentType == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range job.Headers {
		req.Header.Set(k, v)
	}

	// Execute request
	resp, err := h.httpClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			attempt.Status = dispatchjob.DispatchAttemptStatusTimeout
			attempt.ErrorMessage = "Request timeout"
			attempt.ErrorType = dispatchjob.ErrorTypeTransient
		} else if strings.Contains(err.Error(), "connection refused") ||
			strings.Contains(err.Error(), "no such host") {
			attempt.Status = dispatchjob.DispatchAttemptStatusConnectionError
			attempt.ErrorMessage = err.Error()
			attempt.ErrorType = dispatchjob.ErrorTypeTransient
		} else {
			attempt.Status = dispatchjob.DispatchAttemptStatusServerError
			attempt.ErrorMessage = err.Error()
			attempt.ErrorType = dispatchjob.ErrorTypeTransient
		}
		return h.finalizeAttempt(attempt, startTime)
	}
	defer resp.Body.Close()

	attempt.ResponseCode = resp.StatusCode

	// Read response body (limit to 64KB)
	body := make([]byte, 64*1024)
	n, _ := resp.Body.Read(body)
	attempt.ResponseBody = string(body[:n])

	// Determine status based on response code
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		attempt.Status = dispatchjob.DispatchAttemptStatusSuccess
	} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		attempt.Status = dispatchjob.DispatchAttemptStatusClientError
		attempt.ErrorMessage = "HTTP " + http.StatusText(resp.StatusCode)
		attempt.ErrorType = dispatchjob.ErrorTypePermanent
	} else {
		attempt.Status = dispatchjob.DispatchAttemptStatusServerError
		attempt.ErrorMessage = "HTTP " + http.StatusText(resp.StatusCode)
		attempt.ErrorType = dispatchjob.ErrorTypeTransient
	}

	return h.finalizeAttempt(attempt, startTime)
}

// finalizeAttempt completes the attempt record with timing
func (h *DispatchProcessingHandler) finalizeAttempt(attempt *dispatchjob.DispatchAttempt, startTime time.Time) *dispatchjob.DispatchAttempt {
	attempt.CompletedAt = time.Now()
	attempt.DurationMillis = time.Since(startTime).Milliseconds()
	return attempt
}

// calculateBackoffDelay calculates exponential backoff delay
func (h *DispatchProcessingHandler) calculateBackoffDelay(attemptCount int) int {
	// Exponential backoff: 2^attempt * 5 seconds
	// Attempt 1: 10s, Attempt 2: 20s, Attempt 3: 40s, etc.
	delay := (1 << attemptCount) * 5
	if delay > model.MaxDelaySeconds {
		delay = model.MaxDelaySeconds
	}
	return delay
}

// extractBearerTokenFromHeader extracts the token from an Authorization header value
func extractBearerTokenFromHeader(authHeader string) string {
	if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
		return strings.TrimSpace(authHeader[7:])
	}
	return ""
}
