// Package processing implements POST /api/dispatch/process — the internal
// callback the message router invokes for each queued dispatch job.
//
// Flow: the scheduler publishes a job to the broker with its mediation_target
// pointed here (not at the subscriber). The router consumes the message and
// POSTs {"messageId": id} to this endpoint, which then:
//
//  1. loads the job and verifies the scheduler-signed bearer token,
//  2. marks it PROCESSING,
//  3. delivers the real webhook to the subscriber's target_url,
//  4. records the attempt in msg_dispatch_job_attempts,
//  5. advances the job status (COMPLETED / retry-scheduled / FAILED),
//  6. returns {"ack": true} so the router removes the queue message.
//
// Retries are driven by the scheduler poller via scheduled_for, NOT by the
// queue: this endpoint always ACKs and reschedules failed jobs to
// NOW()+backoff, so exactly one component re-dispatches a job (no queue-NACK
// racing the poller into a double dispatch). This deliberately diverges from
// the Rust callback, which NACKs and leaves both paths live.
package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// maxResponseBody caps how much of a subscriber response we read into the
// recorded attempt — a hostile or chatty endpoint must not balloon a row.
const maxResponseBody = 64 << 10 // 64 KiB

// defaultTimeout applies when a job carries no explicit timeout_seconds.
const defaultTimeout = 30 * time.Second

// retryBackoff maps a just-finished attempt number (1-based) to the delay
// before the poller may re-dispatch. Index attemptNumber-1, clamped to the
// last element. Exponential-ish, capped at 2m.
var retryBackoff = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

// Verifier checks the HMAC bearer token the router forwards (the scheduler
// signed the job id). Satisfied by *scheduler.DispatchAuthService.
type Verifier interface {
	Verify(jobID, token string) bool
}

// Handler serves the dispatch-processing callback.
type Handler struct {
	repo     *dispatchjob.Repository
	verifier Verifier
	client   *http.Client
}

// New wires the handler. verifier may be nil (dev/no-auth), in which case the
// bearer token is not checked — but that is a misconfiguration in any
// deployment where the scheduler signs tokens, so callers should pass one.
func New(repo *dispatchjob.Repository, verifier Verifier) *Handler {
	return &Handler{
		repo:     repo,
		verifier: verifier,
		// Outer ceiling only; each delivery uses a per-job context timeout.
		// No redirect-following: a 3xx from a webhook target is not a success.
		client: &http.Client{
			Timeout: 2 * time.Minute,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Mount attaches POST /api/dispatch/process to the given (unauthenticated)
// chi router. The handler self-verifies the scheduler HMAC bearer, so it must
// live OUTSIDE the platform JWT middleware.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/dispatch/process", h.serve)
}

type processRequest struct {
	MessageID string `json:"messageId"`
}

// processResponse is the router's contract (see internal/router mediator):
// ack=false with an optional delaySeconds asks the router to retry via the
// queue. This endpoint always acks (poller owns retries), so it only ever
// returns ack=true — the fields exist for shape compatibility.
type processResponse struct {
	Ack     bool   `json:"ack"`
	Message string `json:"message,omitempty"`
}

func writeJSON(w http.ResponseWriter, code int, body processResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req processRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil || strings.TrimSpace(req.MessageID) == "" {
		writeJSON(w, http.StatusBadRequest, processResponse{Ack: true, Message: "invalid messageId"})
		return
	}
	jobID := req.MessageID

	// Verify the scheduler-signed bearer. Absent/invalid → 401, no ack: a
	// forged callback must not be able to trigger deliveries, and the router
	// (which always carries a valid token) will never hit this branch.
	if h.verifier != nil {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "" || !h.verifier.Verify(jobID, token) {
			slog.Warn("dispatch process: bad auth token", "job_id", jobID)
			writeJSON(w, http.StatusUnauthorized, processResponse{Ack: false, Message: "unauthorized"})
			return
		}
	}

	job, err := h.repo.FindByID(ctx, jobID)
	if err != nil {
		// Transient DB error — NACK so the queue redelivers.
		slog.Error("dispatch process: load job failed", "job_id", jobID, "err", err)
		writeJSON(w, http.StatusInternalServerError, processResponse{Ack: false, Message: "load failed"})
		return
	}
	if job == nil {
		// The row is gone; nothing to deliver. Ack to drop the message.
		writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "job not found"})
		return
	}
	if job.Status.IsTerminal() {
		// Already COMPLETED/FAILED/CANCELLED/EXPIRED (e.g. a duplicate
		// redelivery). Ack without re-delivering.
		writeJSON(w, http.StatusOK, processResponse{Ack: true})
		return
	}

	if err := h.repo.MarkInProgress(ctx, jobID); err != nil {
		slog.Warn("dispatch process: mark in-progress failed", "job_id", jobID, "err", err)
	}

	attemptNumber := job.AttemptCount + 1
	attempt := dispatchjob.NewAttempt(attemptNumber)
	res := h.deliver(ctx, job)

	// Record the attempt (best-effort; a recording failure must not change
	// the delivery decision).
	if res.success {
		attempt.CompleteSuccess(res.statusCode, res.body)
	} else {
		attempt.CompleteFailure(res.errMessage, res.errType, res.statusCodePtr())
	}
	if err := h.repo.RecordAttempt(ctx, jobID, attempt); err != nil {
		slog.Warn("dispatch process: record attempt failed", "job_id", jobID, "err", err)
	}

	h.advance(ctx, job, attemptNumber, res, attempt)

	writeJSON(w, http.StatusOK, processResponse{Ack: true})
}

// advance transitions the job row based on the delivery result.
func (h *Handler) advance(ctx context.Context, job *dispatchjob.DispatchJob, attemptNumber int32, res deliveryResult, attempt *dispatchjob.Attempt) {
	jobID := job.ID
	dur := int64(0)
	if attempt.DurationMillis != nil {
		dur = *attempt.DurationMillis
	}

	switch {
	case res.success:
		if err := h.repo.MarkCompleted(ctx, jobID, dur); err != nil {
			slog.Warn("dispatch process: mark completed failed", "job_id", jobID, "err", err)
		}
		slog.Debug("dispatch delivered", "job_id", jobID, "status", res.statusCode, "attempt", attemptNumber)

	case res.deferral:
		// Cooperative back-pressure (ack=false or HTTP 429): retry later
		// WITHOUT consuming the retry budget.
		if err := h.repo.Reschedule(ctx, jobID, time.Now().Add(res.retryAfter)); err != nil {
			slog.Warn("dispatch process: reschedule failed", "job_id", jobID, "err", err)
		}
		slog.Info("dispatch deferred", "job_id", jobID, "retry_after", res.retryAfter, "reason", res.errMessage)

	case int(attemptNumber) >= int(job.MaxRetries):
		// Out of retries → terminal failure.
		errMsg := res.errMessage
		if err := h.repo.MarkFailed(ctx, jobID, &errMsg, dur); err != nil {
			slog.Warn("dispatch process: mark failed failed", "job_id", jobID, "err", err)
		}
		slog.Warn("dispatch failed (retries exhausted)", "job_id", jobID, "attempts", attemptNumber, "max", job.MaxRetries, "err", errMsg)

	default:
		// Retryable failure → schedule a backoff and let the poller pick it up.
		errMsg := res.errMessage
		if err := h.repo.ScheduleRetry(ctx, jobID, time.Now().Add(backoffFor(attemptNumber)), &errMsg); err != nil {
			slog.Warn("dispatch process: schedule retry failed", "job_id", jobID, "err", err)
		}
		slog.Info("dispatch retry scheduled", "job_id", jobID, "attempt", attemptNumber, "backoff", backoffFor(attemptNumber), "err", errMsg)
	}
}

func backoffFor(attemptNumber int32) time.Duration {
	i := int(attemptNumber) - 1
	if i < 0 {
		i = 0
	}
	if i >= len(retryBackoff) {
		i = len(retryBackoff) - 1
	}
	return retryBackoff[i]
}

// deliveryResult is the outcome of one webhook POST.
type deliveryResult struct {
	success    bool
	deferral   bool // cooperative back-pressure (retry, no budget spend)
	retryAfter time.Duration
	statusCode int
	hasStatus  bool
	body       *string
	errMessage string
	errType    dispatchjob.ErrorType
}

func (r deliveryResult) statusCodePtr() *int {
	if !r.hasStatus {
		return nil
	}
	s := r.statusCode
	return &s
}

// deliver POSTs the real event to the subscriber's target_url and classifies
// the response.
func (h *Handler) deliver(ctx context.Context, job *dispatchjob.DispatchJob) deliveryResult {
	timeout := defaultTimeout
	if job.TimeoutSeconds > 0 {
		timeout = time.Duration(job.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	body := buildPayload(job)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.TargetURL, bytes.NewReader(body))
	if err != nil {
		return deliveryResult{errMessage: "build request: " + err.Error(), errType: dispatchjob.ErrorConnection}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Dispatch-Job-Id", job.ID)
	req.Header.Set("X-Event-Type", job.Code)

	resp, err := h.client.Do(req)
	if err != nil {
		msg, et := classifyTransportErr(err)
		return deliveryResult{errMessage: msg, errType: et}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	bodyStr := string(raw)
	status := resp.StatusCode

	switch {
	case status >= 200 && status < 300:
		// Honour a cooperative deferral: {"ack": false} means "accepted but
		// not done — try again later".
		if deferDelay, deferred := parseDeferral(raw); deferred {
			return deliveryResult{
				deferral:   true,
				retryAfter: deferDelay,
				statusCode: status,
				hasStatus:  true,
				body:       &bodyStr,
				errMessage: "subscriber deferred (ack=false)",
			}
		}
		return deliveryResult{success: true, statusCode: status, hasStatus: true, body: &bodyStr}

	case status == http.StatusTooManyRequests: // 429 → back-pressure, not a failure
		return deliveryResult{
			deferral:   true,
			retryAfter: retryAfterOrDefault(resp),
			statusCode: status,
			hasStatus:  true,
			body:       &bodyStr,
			errMessage: "rate limited (429)",
		}

	default: // 3xx / 4xx / 5xx → delivery failure
		return deliveryResult{
			statusCode: status,
			hasStatus:  true,
			body:       &bodyStr,
			errMessage: "HTTP " + resp.Status,
			errType:    dispatchjob.ErrorHTTPError,
		}
	}
}

// buildPayload renders the request body: raw payload in data-only mode,
// otherwise a CloudEvents-style envelope.
func buildPayload(job *dispatchjob.DispatchJob) []byte {
	if job.DataOnly {
		if job.Payload != nil {
			return []byte(*job.Payload)
		}
		return []byte("{}")
	}

	env := map[string]any{
		"id":            job.ID,
		"type":          job.Code,
		"attemptNumber": job.AttemptCount + 1,
	}
	if job.Source != nil {
		env["source"] = *job.Source
	}
	if job.Subject != nil {
		env["subject"] = *job.Subject
	}
	if job.CorrelationID != nil {
		env["correlationId"] = *job.CorrelationID
	}
	if job.MessageGroup != nil {
		env["messageGroup"] = *job.MessageGroup
	}
	if job.ClientID != nil {
		env["clientId"] = *job.ClientID
	}
	if job.Payload != nil {
		// Embed as JSON when it parses; otherwise pass the raw string through
		// so a non-JSON payload isn't silently dropped.
		var parsed json.RawMessage
		if json.Unmarshal([]byte(*job.Payload), &parsed) == nil {
			env["data"] = parsed
		} else {
			env["data"] = *job.Payload
		}
	}
	out, err := json.Marshal(env)
	if err != nil {
		return []byte("{}")
	}
	return out
}

// parseDeferral reports a 2xx body of the form {"ack": false} (optionally
// with delaySeconds) as a cooperative deferral.
func parseDeferral(body []byte) (time.Duration, bool) {
	if len(body) == 0 {
		return 0, false
	}
	var r struct {
		Ack          *bool   `json:"ack"`
		DelaySeconds *uint32 `json:"delaySeconds"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Ack == nil || *r.Ack {
		return 0, false
	}
	d := 30 * time.Second
	if r.DelaySeconds != nil {
		d = time.Duration(*r.DelaySeconds) * time.Second
	}
	return d, true
}

func retryAfterOrDefault(resp *http.Response) time.Duration {
	if v := resp.Header.Get("Retry-After"); v != "" {
		if secs, err := time.ParseDuration(v + "s"); err == nil && secs > 0 {
			return secs
		}
	}
	return 30 * time.Second
}

func classifyTransportErr(err error) (string, dispatchjob.ErrorType) {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "Connection timeout", dispatchjob.ErrorTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Connection timeout", dispatchjob.ErrorTimeout
	}
	return "Connection error: " + err.Error(), dispatchjob.ErrorConnection
}
