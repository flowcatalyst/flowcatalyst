package dispatchjob

import (
	"time"
)

// DispatchStatus defines the status of a dispatch job
type DispatchStatus string

const (
	DispatchStatusPending    DispatchStatus = "PENDING"
	DispatchStatusQueued     DispatchStatus = "QUEUED"
	DispatchStatusInProgress DispatchStatus = "IN_PROGRESS"
	DispatchStatusCompleted  DispatchStatus = "COMPLETED"
	DispatchStatusError      DispatchStatus = "ERROR"
	DispatchStatusCancelled  DispatchStatus = "CANCELLED"
)

// DispatchKind defines the kind of dispatch job
type DispatchKind string

const (
	DispatchKindEvent DispatchKind = "EVENT" // Triggered by an event
	DispatchKindTask  DispatchKind = "TASK"  // Standalone task
)

// DispatchProtocol defines the delivery protocol
type DispatchProtocol string

const (
	DispatchProtocolHTTPWebhook DispatchProtocol = "HTTP_WEBHOOK"
)

// DispatchAttemptStatus defines the status of a delivery attempt
type DispatchAttemptStatus string

const (
	DispatchAttemptStatusSuccess         DispatchAttemptStatus = "SUCCESS"
	DispatchAttemptStatusClientError     DispatchAttemptStatus = "CLIENT_ERROR"     // 4xx
	DispatchAttemptStatusServerError     DispatchAttemptStatus = "SERVER_ERROR"     // 5xx
	DispatchAttemptStatusTimeout         DispatchAttemptStatus = "TIMEOUT"
	DispatchAttemptStatusConnectionError DispatchAttemptStatus = "CONNECTION_ERROR"
)

// ErrorType categorizes errors for retry decisions
type ErrorType string

const (
	ErrorTypeTransient ErrorType = "TRANSIENT" // Retry
	ErrorTypePermanent ErrorType = "PERMANENT" // Don't retry
	ErrorTypeUnknown   ErrorType = "UNKNOWN"
)

// DispatchMode from subscription package (re-defined to avoid circular import)
type DispatchMode string

const (
	DispatchModeImmediate    DispatchMode = "IMMEDIATE"
	DispatchModeNextOnError  DispatchMode = "NEXT_ON_ERROR"
	DispatchModeBlockOnError DispatchMode = "BLOCK_ON_ERROR"
)

// DispatchJob represents a job to dispatch a message
// Collection: dispatch_jobs
type DispatchJob struct {
	ID                 string               `bson:"_id" json:"id"`
	ExternalID         string               `bson:"externalId,omitempty" json:"externalId,omitempty"`
	Source             string               `bson:"source" json:"source"`
	Kind               DispatchKind         `bson:"kind" json:"kind"`
	Code               string               `bson:"code" json:"code"`
	Subject            string               `bson:"subject,omitempty" json:"subject,omitempty"`
	EventID            string               `bson:"eventId,omitempty" json:"eventId,omitempty"`
	CorrelationID      string               `bson:"correlationId,omitempty" json:"correlationId,omitempty"`
	Metadata           []DispatchJobMetadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	TargetURL          string               `bson:"targetUrl" json:"targetUrl"`
	Protocol           DispatchProtocol     `bson:"protocol" json:"protocol"`
	Headers            map[string]string    `bson:"headers,omitempty" json:"headers,omitempty"`
	Payload            string               `bson:"payload" json:"payload"`
	PayloadContentType string               `bson:"payloadContentType" json:"payloadContentType"`
	DataOnly           bool                 `bson:"dataOnly" json:"dataOnly"`
	ServiceAccountID   string               `bson:"serviceAccountId,omitempty" json:"serviceAccountId,omitempty"`
	ClientID           string               `bson:"clientId,omitempty" json:"clientId,omitempty"`
	SubscriptionID     string               `bson:"subscriptionId,omitempty" json:"subscriptionId,omitempty"`
	Mode               DispatchMode         `bson:"mode,omitempty" json:"mode,omitempty"`
	DispatchPoolID     string               `bson:"dispatchPoolId,omitempty" json:"dispatchPoolId,omitempty"`
	MessageGroup       string               `bson:"messageGroup,omitempty" json:"messageGroup,omitempty"`
	Sequence           int                  `bson:"sequence,omitempty" json:"sequence,omitempty"`
	TimeoutSeconds     int                  `bson:"timeoutSeconds,omitempty" json:"timeoutSeconds,omitempty"`
	SchemaID           string               `bson:"schemaId,omitempty" json:"schemaId,omitempty"`
	Status             DispatchStatus       `bson:"status" json:"status"`
	MaxRetries         int                  `bson:"maxRetries" json:"maxRetries"`
	RetryStrategy      string               `bson:"retryStrategy,omitempty" json:"retryStrategy,omitempty"`
	ScheduledFor       time.Time            `bson:"scheduledFor,omitempty" json:"scheduledFor,omitempty"`
	ExpiresAt          time.Time            `bson:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	AttemptCount       int                  `bson:"attemptCount" json:"attemptCount"`
	LastAttemptAt      time.Time            `bson:"lastAttemptAt,omitempty" json:"lastAttemptAt,omitempty"`
	CompletedAt        time.Time            `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
	DurationMillis     int64                `bson:"durationMillis,omitempty" json:"durationMillis,omitempty"`
	LastError          string               `bson:"lastError,omitempty" json:"lastError,omitempty"`
	IdempotencyKey     string               `bson:"idempotencyKey,omitempty" json:"idempotencyKey,omitempty"`
	Attempts           []DispatchAttempt    `bson:"attempts,omitempty" json:"attempts,omitempty"`
	CreatedAt          time.Time            `bson:"createdAt" json:"createdAt"`
	UpdatedAt          time.Time            `bson:"updatedAt" json:"updatedAt"`
}

// DispatchJobMetadata represents metadata on a dispatch job
type DispatchJobMetadata struct {
	ID    string `bson:"id" json:"id"`
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// DispatchAttempt represents a single delivery attempt
type DispatchAttempt struct {
	ID              string                `bson:"id" json:"id"`
	AttemptNumber   int                   `bson:"attemptNumber" json:"attemptNumber"`
	AttemptedAt     time.Time             `bson:"attemptedAt" json:"attemptedAt"`
	CompletedAt     time.Time             `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
	DurationMillis  int64                 `bson:"durationMillis,omitempty" json:"durationMillis,omitempty"`
	Status          DispatchAttemptStatus `bson:"status" json:"status"`
	ResponseCode    int                   `bson:"responseCode,omitempty" json:"responseCode,omitempty"`
	ResponseBody    string                `bson:"responseBody,omitempty" json:"responseBody,omitempty"`
	ErrorMessage    string                `bson:"errorMessage,omitempty" json:"errorMessage,omitempty"`
	ErrorStackTrace string                `bson:"errorStackTrace,omitempty" json:"errorStackTrace,omitempty"`
	ErrorType       ErrorType             `bson:"errorType,omitempty" json:"errorType,omitempty"`
	CreatedAt       time.Time             `bson:"createdAt" json:"createdAt"`
}

// IsPending returns true if the job is pending
func (j *DispatchJob) IsPending() bool {
	return j.Status == DispatchStatusPending
}

// IsQueued returns true if the job is queued
func (j *DispatchJob) IsQueued() bool {
	return j.Status == DispatchStatusQueued
}

// IsInProgress returns true if the job is in progress
func (j *DispatchJob) IsInProgress() bool {
	return j.Status == DispatchStatusInProgress
}

// IsCompleted returns true if the job is completed
func (j *DispatchJob) IsCompleted() bool {
	return j.Status == DispatchStatusCompleted
}

// IsError returns true if the job is in error state
func (j *DispatchJob) IsError() bool {
	return j.Status == DispatchStatusError
}

// IsTerminal returns true if the job is in a terminal state
func (j *DispatchJob) IsTerminal() bool {
	return j.Status == DispatchStatusCompleted ||
		j.Status == DispatchStatusError ||
		j.Status == DispatchStatusCancelled
}

// CanRetry returns true if the job can be retried
func (j *DispatchJob) CanRetry() bool {
	return j.AttemptCount < j.MaxRetries && !j.IsTerminal()
}

// IsExpired returns true if the job has expired
func (j *DispatchJob) IsExpired() bool {
	if j.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(j.ExpiresAt)
}

// GetMetadataValue returns the value for a metadata key
func (j *DispatchJob) GetMetadataValue(key string) string {
	for _, m := range j.Metadata {
		if m.Key == key {
			return m.Value
		}
	}
	return ""
}

// GetLastAttempt returns the most recent attempt
func (j *DispatchJob) GetLastAttempt() *DispatchAttempt {
	if len(j.Attempts) == 0 {
		return nil
	}
	return &j.Attempts[len(j.Attempts)-1]
}
