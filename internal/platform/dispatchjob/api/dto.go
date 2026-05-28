// dto.go contains the wire-format types for the dispatch_job API.
package api

import (
	"strings"
	"time"

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

// DispatchJobRead is the slim read-projection wire shape for the list
// endpoints (GET /api/dispatch-jobs, /bff/dispatch-jobs). Matches the
// SPA's `DispatchJobRead` interface (frontend/src/api/dispatch-jobs.ts):
// the list grid binds id/code/source/status/mode/targetUrl/createdAt, and
// the interface also carries the projection facets. Mirrors Rust's
// DispatchJobReadResponse (dispatch_job/api.rs:104).
type DispatchJobRead struct {
	ID               string           `json:"id"`
	EventID          *string          `json:"eventId,omitempty"`
	SubscriptionID   *string          `json:"subscriptionId,omitempty"`
	ClientID         *string          `json:"clientId,omitempty"`
	ClientIdentifier *string          `json:"clientIdentifier,omitempty"`
	Application      *string          `json:"application,omitempty"`
	Subdomain        *string          `json:"subdomain,omitempty"`
	Aggregate        *string          `json:"aggregate,omitempty"`
	Code             string           `json:"code"`
	Source           *string          `json:"source,omitempty"`
	Subject          *string          `json:"subject,omitempty"`
	Status           string           `json:"status"`
	Kind             string           `json:"kind"`
	TargetURL        string           `json:"targetUrl"`
	Mode             string           `json:"mode"`
	DispatchMode     *string          `json:"dispatchMode,omitempty"`
	Priority         *int32           `json:"priority,omitempty"`
	CorrelationID    *string          `json:"correlationId,omitempty"`
	ScheduledFor     *httpcompat.Time `json:"scheduledFor,omitempty"`
	CreatedAt        httpcompat.Time  `json:"createdAt"`
	UpdatedAt        httpcompat.Time  `json:"updatedAt"`
	CompletedAt      *httpcompat.Time `json:"completedAt,omitempty"`
	LastAttemptAt    *httpcompat.Time `json:"lastAttemptAt,omitempty"`
	AttemptCount     int32            `json:"attemptCount"`
}

func readFromEntity(j *dispatchjob.DispatchJob) DispatchJobRead {
	tp := func(t *time.Time) *httpcompat.Time {
		if t == nil {
			return nil
		}
		v := jsontime.New(*t)
		return &v
	}
	mode := string(j.Mode)
	app, sub, agg := splitCode(j.Code)
	return DispatchJobRead{
		ID:             j.ID,
		EventID:        j.EventID,
		SubscriptionID: j.SubscriptionID,
		ClientID:       j.ClientID,
		Application:    app,
		Subdomain:      sub,
		Aggregate:      agg,
		Code:           j.Code,
		Source:         j.Source,
		Subject:        j.Subject,
		Status:         string(j.Status),
		Kind:           string(j.Kind),
		TargetURL:      j.TargetURL,
		Mode:           mode,
		DispatchMode:   &mode,
		CorrelationID:  j.CorrelationID,
		ScheduledFor:   tp(j.ScheduledFor),
		CreatedAt:      jsontime.New(j.CreatedAt),
		UpdatedAt:      jsontime.New(j.UpdatedAt),
		CompletedAt:    tp(j.CompletedAt),
		LastAttemptAt:  tp(j.LastAttemptAt),
		AttemptCount:   j.AttemptCount,
	}
}

// splitCode derives application/subdomain/aggregate from the colon-
// delimited dispatch code (e.g. "orders:fulfillment:shipment:shipped"),
// mirroring the Rust EventRead accessors. The write-side msg_dispatch_jobs
// table has no separate facet columns, so we project them from the code.
func splitCode(code string) (app, sub, agg *string) {
	parts := strings.Split(code, ":")
	if len(parts) > 0 && parts[0] != "" {
		app = &parts[0]
	}
	if len(parts) > 1 && parts[1] != "" {
		sub = &parts[1]
	}
	if len(parts) > 2 && parts[2] != "" {
		agg = &parts[2]
	}
	return app, sub, agg
}

// RawDispatchJobResponse is the debug raw-job wire shape for GET
// /bff/debug/dispatch-jobs. The SPA's RawDispatchJobListPage binds a bare
// array with fields kind/code/status/payloadLength/idempotencyKey/etc.
// Mirrors Rust's shared/debug_api.rs RawDispatchJobResponse, including the
// derived payloadLength + attemptHistoryCount.
type RawDispatchJobResponse struct {
	ID                  string           `json:"id"`
	ExternalID          *string          `json:"externalId,omitempty"`
	Source              *string          `json:"source,omitempty"`
	Kind                string           `json:"kind"`
	Code                string           `json:"code"`
	Subject             *string          `json:"subject,omitempty"`
	EventID             *string          `json:"eventId,omitempty"`
	CorrelationID       *string          `json:"correlationId,omitempty"`
	TargetURL           string           `json:"targetUrl"`
	Protocol            string           `json:"protocol"`
	ClientID            *string          `json:"clientId,omitempty"`
	SubscriptionID      *string          `json:"subscriptionId,omitempty"`
	ServiceAccountID    *string          `json:"serviceAccountId,omitempty"`
	DispatchPoolID      *string          `json:"dispatchPoolId,omitempty"`
	MessageGroup        *string          `json:"messageGroup,omitempty"`
	Mode                string           `json:"mode"`
	Sequence            int32            `json:"sequence"`
	Status              string           `json:"status"`
	AttemptCount        int32            `json:"attemptCount"`
	MaxRetries          uint32           `json:"maxRetries"`
	LastError           *string          `json:"lastError,omitempty"`
	TimeoutSeconds      uint32           `json:"timeoutSeconds"`
	RetryStrategy       string           `json:"retryStrategy"`
	IdempotencyKey      *string          `json:"idempotencyKey,omitempty"`
	CreatedAt           httpcompat.Time  `json:"createdAt"`
	UpdatedAt           httpcompat.Time  `json:"updatedAt"`
	ScheduledFor        *httpcompat.Time `json:"scheduledFor,omitempty"`
	CompletedAt         *httpcompat.Time `json:"completedAt,omitempty"`
	PayloadContentType  string           `json:"payloadContentType"`
	PayloadLength       int              `json:"payloadLength"`
	AttemptHistoryCount int              `json:"attemptHistoryCount"`
}

func rawFromEntity(j *dispatchjob.DispatchJob) RawDispatchJobResponse {
	tp := func(t *time.Time) *httpcompat.Time {
		if t == nil {
			return nil
		}
		v := jsontime.New(*t)
		return &v
	}
	payloadLen := 0
	if j.Payload != nil {
		payloadLen = len(*j.Payload)
	}
	return RawDispatchJobResponse{
		ID:                  j.ID,
		ExternalID:          j.ExternalID,
		Source:              j.Source,
		Kind:                string(j.Kind),
		Code:                j.Code,
		Subject:             j.Subject,
		EventID:             j.EventID,
		CorrelationID:       j.CorrelationID,
		TargetURL:           j.TargetURL,
		Protocol:            string(j.Protocol),
		ClientID:            j.ClientID,
		SubscriptionID:      j.SubscriptionID,
		ServiceAccountID:    j.ServiceAccountID,
		DispatchPoolID:      j.DispatchPoolID,
		MessageGroup:        j.MessageGroup,
		Mode:                string(j.Mode),
		Sequence:            j.Sequence,
		Status:              string(j.Status),
		AttemptCount:        j.AttemptCount,
		MaxRetries:          j.MaxRetries,
		LastError:           j.LastError,
		TimeoutSeconds:      j.TimeoutSeconds,
		RetryStrategy:       string(j.RetryStrategy),
		IdempotencyKey:      j.IdempotencyKey,
		CreatedAt:           jsontime.New(j.CreatedAt),
		UpdatedAt:           jsontime.New(j.UpdatedAt),
		ScheduledFor:        tp(j.ScheduledFor),
		CompletedAt:         tp(j.CompletedAt),
		PayloadContentType:  j.PayloadContentType,
		PayloadLength:       payloadLen,
		AttemptHistoryCount: len(j.Attempts),
	}
}

// DispatchJobListResponse is the wire shape for GET /api/dispatch-jobs.
type DispatchJobListResponse struct {
	Items []DispatchJobResponse `json:"items"`
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
