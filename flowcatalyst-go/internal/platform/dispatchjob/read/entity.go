package read

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
	DispatchKindEvent DispatchKind = "EVENT"
	DispatchKindTask  DispatchKind = "TASK"
)

// DispatchJobRead is a denormalized read projection of DispatchJob
// Collection: dispatch_jobs_read
type DispatchJobRead struct {
	ID                 string         `bson:"_id" json:"id"`
	ExternalID         string         `bson:"externalId,omitempty" json:"externalId,omitempty"`
	Source             string         `bson:"source" json:"source"`
	Kind               DispatchKind   `bson:"kind" json:"kind"`
	Code               string         `bson:"code" json:"code"`
	Subject            string         `bson:"subject,omitempty" json:"subject,omitempty"`
	EventID            string         `bson:"eventId,omitempty" json:"eventId,omitempty"`
	CorrelationID      string         `bson:"correlationId,omitempty" json:"correlationId,omitempty"`
	TargetURL          string         `bson:"targetUrl" json:"targetUrl"`
	PayloadContentType string         `bson:"payloadContentType" json:"payloadContentType"`
	ClientID           string         `bson:"clientId,omitempty" json:"clientId,omitempty"`
	SubscriptionID     string         `bson:"subscriptionId,omitempty" json:"subscriptionId,omitempty"`
	DispatchPoolID     string         `bson:"dispatchPoolId,omitempty" json:"dispatchPoolId,omitempty"`
	MessageGroup       string         `bson:"messageGroup,omitempty" json:"messageGroup,omitempty"`
	Sequence           int            `bson:"sequence,omitempty" json:"sequence,omitempty"`
	Status             DispatchStatus `bson:"status" json:"status"`
	MaxRetries         int            `bson:"maxRetries" json:"maxRetries"`
	ScheduledFor       time.Time      `bson:"scheduledFor,omitempty" json:"scheduledFor,omitempty"`
	ExpiresAt          time.Time      `bson:"expiresAt,omitempty" json:"expiresAt,omitempty"`
	AttemptCount       int            `bson:"attemptCount" json:"attemptCount"`
	LastAttemptAt      time.Time      `bson:"lastAttemptAt,omitempty" json:"lastAttemptAt,omitempty"`
	CompletedAt        time.Time      `bson:"completedAt,omitempty" json:"completedAt,omitempty"`
	DurationMillis     int64          `bson:"durationMillis,omitempty" json:"durationMillis,omitempty"`
	LastError          string         `bson:"lastError,omitempty" json:"lastError,omitempty"`
	CreatedAt          time.Time      `bson:"createdAt" json:"createdAt"`
	UpdatedAt          time.Time      `bson:"updatedAt" json:"updatedAt"`

	// Computed flags for efficient filtering
	IsCompleted bool `bson:"isCompleted" json:"isCompleted"`
	IsTerminal  bool `bson:"isTerminal" json:"isTerminal"`
	IsExpired   bool `bson:"isExpired" json:"isExpired"`
	CanRetry    bool `bson:"canRetry" json:"canRetry"`

	// Denormalized application info
	ApplicationID   string `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	ApplicationCode string `bson:"applicationCode,omitempty" json:"applicationCode,omitempty"`
	ApplicationName string `bson:"applicationName,omitempty" json:"applicationName,omitempty"`

	// Denormalized subscription info
	SubscriptionCode string `bson:"subscriptionCode,omitempty" json:"subscriptionCode,omitempty"`
	SubscriptionName string `bson:"subscriptionName,omitempty" json:"subscriptionName,omitempty"`

	// Denormalized dispatch pool info
	DispatchPoolCode string `bson:"dispatchPoolCode,omitempty" json:"dispatchPoolCode,omitempty"`
	DispatchPoolName string `bson:"dispatchPoolName,omitempty" json:"dispatchPoolName,omitempty"`

	// Denormalized aggregate info
	SubdomainCode string `bson:"subdomainCode,omitempty" json:"subdomainCode,omitempty"`
	AggregateType string `bson:"aggregateType,omitempty" json:"aggregateType,omitempty"`
	AggregateID   string `bson:"aggregateId,omitempty" json:"aggregateId,omitempty"`

	// Event type info
	EventTypeID   string `bson:"eventTypeId,omitempty" json:"eventTypeId,omitempty"`
	EventTypeCode string `bson:"eventTypeCode,omitempty" json:"eventTypeCode,omitempty"`
	EventTypeName string `bson:"eventTypeName,omitempty" json:"eventTypeName,omitempty"`
}

// IsPending returns true if the job is pending
func (j *DispatchJobRead) IsPending() bool {
	return j.Status == DispatchStatusPending
}

// IsQueued returns true if the job is queued
func (j *DispatchJobRead) IsQueued() bool {
	return j.Status == DispatchStatusQueued
}

// IsInProgress returns true if the job is in progress
func (j *DispatchJobRead) IsInProgress() bool {
	return j.Status == DispatchStatusInProgress
}
