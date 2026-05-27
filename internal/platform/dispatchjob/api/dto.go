// dto.go contains the wire-format types for the dispatch_job API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// MetadataDTO mirrors dispatchjob.Metadata.
type MetadataDTO struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// AttemptDTO mirrors dispatchjob.Attempt.
type AttemptDTO struct {
	AttemptNumber  int32            `json:"attemptNumber"`
	AttemptedAt    httpcompat.Time  `json:"attemptedAt"`
	CompletedAt    *httpcompat.Time `json:"completedAt,omitempty"`
	DurationMillis *int64           `json:"durationMillis,omitempty"`
	ResponseCode   *int             `json:"responseCode,omitempty"`
	ResponseBody   *string          `json:"responseBody,omitempty"`
	Success        bool             `json:"success"`
	ErrorMessage   *string          `json:"errorMessage,omitempty"`
	ErrorType      *string          `json:"errorType,omitempty"`
}

func attemptFromEntity(a *dispatchjob.Attempt) AttemptDTO {
	var completed *httpcompat.Time
	if a.CompletedAt != nil {
		v := jsontime.New(*a.CompletedAt)
		completed = &v
	}
	var errType *string
	if a.ErrorType != nil {
		s := string(*a.ErrorType)
		errType = &s
	}
	return AttemptDTO{
		AttemptNumber:  a.AttemptNumber,
		AttemptedAt:    jsontime.New(a.AttemptedAt),
		CompletedAt:    completed,
		DurationMillis: a.DurationMillis,
		ResponseCode:   a.ResponseCode,
		ResponseBody:   a.ResponseBody,
		Success:        a.Success,
		ErrorMessage:   a.ErrorMessage,
		ErrorType:      errType,
	}
}

// DispatchJobResponse mirrors dispatchjob.DispatchJob.
type DispatchJobResponse struct {
	ID                 string              `json:"id"`
	ExternalID         *string             `json:"externalId,omitempty"`
	Kind               string              `json:"kind"`
	Code               string              `json:"code"`
	Source             *string             `json:"source,omitempty"`
	Subject            *string             `json:"subject,omitempty"`
	TargetURL          string              `json:"targetUrl"`
	Protocol           string              `json:"protocol"`
	Payload            *string             `json:"payload,omitempty"`
	PayloadContentType string              `json:"payloadContentType"`
	DataOnly           bool                `json:"dataOnly"`
	EventID            *string             `json:"eventId,omitempty"`
	CorrelationID      *string             `json:"correlationId,omitempty"`
	ClientID           *string             `json:"clientId,omitempty"`
	SubscriptionID     *string             `json:"subscriptionId,omitempty"`
	ServiceAccountID   *string             `json:"serviceAccountId,omitempty"`
	DispatchPoolID     *string             `json:"dispatchPoolId,omitempty"`
	MessageGroup       *string             `json:"messageGroup,omitempty"`
	Mode               common.DispatchMode `json:"mode"`
	Sequence           int32               `json:"sequence"`
	TimeoutSeconds     uint32              `json:"timeoutSeconds"`
	SchemaID           *string             `json:"schemaId,omitempty"`
	MaxRetries         uint32              `json:"maxRetries"`
	RetryStrategy      string              `json:"retryStrategy"`
	Status             string              `json:"status"`
	AttemptCount       int32               `json:"attemptCount"`
	LastError          *string             `json:"lastError,omitempty"`
	Attempts           []AttemptDTO        `json:"attempts,omitempty"`
	Metadata           []MetadataDTO       `json:"metadata,omitempty"`
	IdempotencyKey     *string             `json:"idempotencyKey,omitempty"`
	CreatedAt          httpcompat.Time     `json:"createdAt"`
	UpdatedAt          httpcompat.Time     `json:"updatedAt"`
	ScheduledFor       *httpcompat.Time    `json:"scheduledFor,omitempty"`
	ExpiresAt          *httpcompat.Time    `json:"expiresAt,omitempty"`
	LastAttemptAt      *httpcompat.Time    `json:"lastAttemptAt,omitempty"`
	CompletedAt        *httpcompat.Time    `json:"completedAt,omitempty"`
	DurationMillis     *int64              `json:"durationMillis,omitempty"`
}

func fromEntity(j *dispatchjob.DispatchJob) DispatchJobResponse {
	attempts := make([]AttemptDTO, 0, len(j.Attempts))
	for i := range j.Attempts {
		attempts = append(attempts, attemptFromEntity(&j.Attempts[i]))
	}
	meta := make([]MetadataDTO, 0, len(j.Metadata))
	for _, m := range j.Metadata {
		meta = append(meta, MetadataDTO{Key: m.Key, Value: m.Value})
	}
	var sched, expires, lastAttempt, completed *httpcompat.Time
	if j.ScheduledFor != nil {
		v := jsontime.New(*j.ScheduledFor)
		sched = &v
	}
	if j.ExpiresAt != nil {
		v := jsontime.New(*j.ExpiresAt)
		expires = &v
	}
	if j.LastAttemptAt != nil {
		v := jsontime.New(*j.LastAttemptAt)
		lastAttempt = &v
	}
	if j.CompletedAt != nil {
		v := jsontime.New(*j.CompletedAt)
		completed = &v
	}
	return DispatchJobResponse{
		ID:                 j.ID,
		ExternalID:         j.ExternalID,
		Kind:               string(j.Kind),
		Code:               j.Code,
		Source:             j.Source,
		Subject:            j.Subject,
		TargetURL:          j.TargetURL,
		Protocol:           string(j.Protocol),
		Payload:            j.Payload,
		PayloadContentType: j.PayloadContentType,
		DataOnly:           j.DataOnly,
		EventID:            j.EventID,
		CorrelationID:      j.CorrelationID,
		ClientID:           j.ClientID,
		SubscriptionID:     j.SubscriptionID,
		ServiceAccountID:   j.ServiceAccountID,
		DispatchPoolID:     j.DispatchPoolID,
		MessageGroup:       j.MessageGroup,
		Mode:               j.Mode,
		Sequence:           j.Sequence,
		TimeoutSeconds:     j.TimeoutSeconds,
		SchemaID:           j.SchemaID,
		MaxRetries:         j.MaxRetries,
		RetryStrategy:      string(j.RetryStrategy),
		Status:             string(j.Status),
		AttemptCount:       j.AttemptCount,
		LastError:          j.LastError,
		Attempts:           attempts,
		Metadata:           meta,
		IdempotencyKey:     j.IdempotencyKey,
		CreatedAt:          jsontime.New(j.CreatedAt),
		UpdatedAt:          jsontime.New(j.UpdatedAt),
		ScheduledFor:       sched,
		ExpiresAt:          expires,
		LastAttemptAt:      lastAttempt,
		CompletedAt:        completed,
		DurationMillis:     j.DurationMillis,
	}
}

// DispatchJobListResponse is the wire shape for GET /api/dispatch-jobs.
type DispatchJobListResponse struct {
	Items []DispatchJobResponse `json:"items"`
}

// AttemptListResponse is the wire shape for GET /api/dispatch-jobs/{id}/attempts.
type AttemptListResponse struct {
	Items []AttemptDTO `json:"items"`
}

// DispatchJobFilterOptionsResponse is the wire shape for GET /api/dispatch-jobs/filter-options.
type DispatchJobFilterOptionsResponse struct {
	Statuses        []string `json:"statuses"`
	Codes           []string `json:"codes"`
	ClientIDs       []string `json:"clientIds"`
	DispatchPoolIDs []string `json:"dispatchPoolIds"`
	SubscriptionIDs []string `json:"subscriptionIds"`
	Kinds           []string `json:"kinds"`
}
