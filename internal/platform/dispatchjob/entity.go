// Package dispatchjob holds the dispatch-job entity and repository.
//
// Per docs/conventions.md §3, this subdomain is an infrastructure-
// processing path: dispatch jobs are written directly (via ingest +
// router status transitions + stream fan-out) rather than through use
// cases. We expose entity types + a repository with direct read/write
// methods, but no UoW commits and no DomainEvent emissions.
//
// Human-initiated dispatch-job actions (resend, ignore, cancel) DO go
// through use cases.
package dispatchjob

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Kind is EVENT or TASK.
type Kind string

const (
	KindEvent Kind = "EVENT"
	KindTask  Kind = "TASK"
)

// ParseKind — lenient parser. Unknown → EVENT.
func ParseKind(s string) Kind {
	if s == string(KindTask) {
		return KindTask
	}
	return KindEvent
}

// Protocol identifies the delivery transport. Currently HTTP_WEBHOOK only.
type Protocol string

const ProtocolHTTPWebhook Protocol = "HTTP_WEBHOOK"

// RetryStrategy controls backoff between attempts.
type RetryStrategy string

const (
	RetryImmediate          RetryStrategy = "immediate"
	RetryFixed              RetryStrategy = "fixed"
	RetryExponentialBackoff RetryStrategy = "exponential"
)

// ParseRetryStrategy — lenient parser. Unknown → exponential.
func ParseRetryStrategy(s string) RetryStrategy {
	switch s {
	case "immediate", "IMMEDIATE":
		return RetryImmediate
	case "fixed", "FIXED_DELAY":
		return RetryFixed
	default:
		return RetryExponentialBackoff
	}
}

// ErrorType classifies an attempt's failure.
type ErrorType string

const (
	ErrorConnection ErrorType = "CONNECTION"
	ErrorTimeout    ErrorType = "TIMEOUT"
	ErrorHTTPError  ErrorType = "HTTP_ERROR"
	ErrorValidation ErrorType = "VALIDATION"
	ErrorUnknown    ErrorType = "UNKNOWN"
)

// ParseErrorType — lenient parser. Unknown → UNKNOWN.
func ParseErrorType(s string) ErrorType {
	switch s {
	case string(ErrorConnection), string(ErrorTimeout), string(ErrorHTTPError), string(ErrorValidation):
		return ErrorType(s)
	default:
		return ErrorUnknown
	}
}

// Metadata is one key/value tag attached to a DispatchJob. The schema
// stores the slice as JSONB; the
// SDK wire format is an array — `map[string]string` would serialize as
// a JSON object and break drop-in parity.
type Metadata struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Attempt records one delivery attempt against a job. Stored in
// msg_dispatch_job_attempts (separate table, partitioned monthly).
//
// `Success` is exposed on the wire but
// persisted as the schema's `status` column (`SUCCESS` / `FAILURE`).
type Attempt struct {
	AttemptNumber  int32      `json:"attemptNumber"`
	AttemptedAt    time.Time  `json:"attemptedAt"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	DurationMillis *int64     `json:"durationMillis,omitempty"`
	ResponseCode   *int       `json:"responseCode,omitempty"`
	ResponseBody   *string    `json:"responseBody,omitempty"`
	Success        bool       `json:"success"`
	ErrorMessage   *string    `json:"errorMessage,omitempty"`
	ErrorType      *ErrorType `json:"errorType,omitempty"`
}

// NewAttempt constructs a started attempt.
func NewAttempt(n int32) *Attempt {
	return &Attempt{AttemptNumber: n, AttemptedAt: time.Now().UTC()}
}

// CompleteSuccess records a 2xx outcome and bumps duration.
func (a *Attempt) CompleteSuccess(status int, body *string) {
	now := time.Now().UTC()
	a.CompletedAt = &now
	d := now.Sub(a.AttemptedAt).Milliseconds()
	a.DurationMillis = &d
	a.ResponseCode = &status
	a.ResponseBody = body
	a.Success = true
}

// CompleteFailure records an error outcome.
func (a *Attempt) CompleteFailure(msg string, errType ErrorType, status *int) {
	now := time.Now().UTC()
	a.CompletedAt = &now
	d := now.Sub(a.AttemptedAt).Milliseconds()
	a.DurationMillis = &d
	a.ResponseCode = status
	a.ErrorMessage = &msg
	a.ErrorType = &errType
	a.Success = false
}

// DispatchJob is the aggregate. Lives in msg_dispatch_jobs (write side)
// and msg_dispatch_jobs_read (denormalized read side maintained by
// the stream processor). Field set + JSON shape are pinned by the SDK
// drop-in wire contract.
type DispatchJob struct {
	ID                 string                `json:"id"`
	ExternalID         *string               `json:"externalId,omitempty"`
	Kind               Kind                  `json:"kind"`
	Code               string                `json:"code"`
	Source             *string               `json:"source,omitempty"`
	Subject            *string               `json:"subject,omitempty"`
	TargetURL          string                `json:"targetUrl"`
	Protocol           Protocol              `json:"protocol"`
	Payload            *string               `json:"payload,omitempty"`
	PayloadContentType string                `json:"payloadContentType"`
	DataOnly           bool                  `json:"dataOnly"`
	EventID            *string               `json:"eventId,omitempty"`
	CorrelationID      *string               `json:"correlationId,omitempty"`
	ClientID           *string               `json:"clientId,omitempty"`
	SubscriptionID     *string               `json:"subscriptionId,omitempty"`
	ServiceAccountID   *string               `json:"serviceAccountId,omitempty"`
	DispatchPoolID     *string               `json:"dispatchPoolId,omitempty"`
	MessageGroup       *string               `json:"messageGroup,omitempty"`
	Mode               common.DispatchMode   `json:"mode"`
	Sequence           int32                 `json:"sequence"`
	TimeoutSeconds     uint32                `json:"timeoutSeconds"`
	SchemaID           *string               `json:"schemaId,omitempty"`
	MaxRetries         uint32                `json:"maxRetries"`
	RetryStrategy      RetryStrategy         `json:"retryStrategy"`
	Status             common.DispatchStatus `json:"status"`
	AttemptCount       int32                 `json:"attemptCount"`
	LastError          *string               `json:"lastError,omitempty"`
	Attempts           []Attempt             `json:"attempts,omitempty"`
	Metadata           []Metadata            `json:"metadata,omitempty"`
	IdempotencyKey     *string               `json:"idempotencyKey,omitempty"`
	CreatedAt          time.Time             `json:"createdAt"`
	UpdatedAt          time.Time             `json:"updatedAt"`
	ScheduledFor       *time.Time            `json:"scheduledFor,omitempty"`
	ExpiresAt          *time.Time            `json:"expiresAt,omitempty"`
	LastAttemptAt      *time.Time            `json:"lastAttemptAt,omitempty"`
	CompletedAt        *time.Time            `json:"completedAt,omitempty"`
	DurationMillis     *int64                `json:"durationMillis,omitempty"`
}

// PayloadJSON returns the payload parsed as JSON when ContentType is
// application/json, otherwise the literal bytes. Convenience for the
// router and read APIs.
func (j *DispatchJob) PayloadJSON() (json.RawMessage, error) {
	if j.Payload == nil {
		return nil, nil
	}
	return json.RawMessage(*j.Payload), nil
}

// IDStr satisfies usecase.HasID, so DispatchJob can be committed through the
// use-case envelope (operations/) — see the package doc: human-initiated
// actions (cancel, complete, resend) go through use cases; router-driven
// infra writes (Insert, MarkInProgress, ...) stay direct repo calls.
func (j DispatchJob) IDStr() string { return j.ID }

// Cancel flips a FAILED job to CANCELLED — an operator override for a
// stuck/failed job that isn't worth retrying. The FAILED-only guard lives in
// the operation's Execute (operations/shared.go), not here; the entity just
// applies the transition.
func (j *DispatchJob) Cancel() {
	now := time.Now().UTC()
	j.Status = common.DispatchCancelled
	j.CompletedAt = &now
	j.UpdatedAt = now
}

// Complete flips a FAILED job to COMPLETED — an operator override recording
// that the job was actually delivered/handled out of band (verified
// manually, or via a side channel) and should stop reading as a failure.
func (j *DispatchJob) Complete() {
	now := time.Now().UTC()
	j.Status = common.DispatchCompleted
	j.CompletedAt = &now
	j.UpdatedAt = now
}

// ResetToPending puts the job back at PENDING for a fresh delivery cycle:
// clears scheduled_for (immediate eligibility), zeroes attempt_count (a full
// retry budget again), and clears the terminal stamps. reason, when non-nil,
// is recorded in LastError so an operator can see why the job is back at
// PENDING — used by the router-settled hook and the stranded-sibling reaper
// (see the settled package and reaper.go). A nil reason clears LastError
// instead: the operator Resend path carries no reason of its own, mirroring
// the pre-envelope Requeue's behaviour of unconditionally clearing it.
func (j *DispatchJob) ResetToPending(reason *string) {
	j.Status = common.DispatchPending
	j.ScheduledFor = nil
	j.AttemptCount = 0
	j.CompletedAt = nil
	j.DurationMillis = nil
	j.LastError = reason
	j.UpdatedAt = time.Now().UTC()
}
