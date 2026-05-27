// dto.go contains the wire-format types for the scheduled_job API.
package api

import (
	"encoding/json"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateScheduledJobRequest is the wire body for POST /api/scheduled-jobs.
type CreateScheduledJobRequest struct {
	Code                string          `json:"code"`
	Name                string          `json:"name"`
	Crons               []string        `json:"crons"`
	Timezone            string          `json:"timezone,omitempty"`
	ClientID            *string         `json:"clientId,omitempty"`
	Description         *string         `json:"description,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          bool            `json:"concurrent"`
	TracksCompletion    bool            `json:"tracksCompletion"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts *int32          `json:"deliveryMaxAttempts,omitempty"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
}

func (r CreateScheduledJobRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:                r.Code,
		Name:                r.Name,
		Crons:               r.Crons,
		Timezone:            r.Timezone,
		ClientID:            r.ClientID,
		Description:         r.Description,
		Payload:             r.Payload,
		Concurrent:          r.Concurrent,
		TracksCompletion:    r.TracksCompletion,
		TimeoutSeconds:      r.TimeoutSeconds,
		DeliveryMaxAttempts: r.DeliveryMaxAttempts,
		TargetURL:           r.TargetURL,
	}
}

// UpdateScheduledJobRequest is the wire body for PUT /api/scheduled-jobs/{id}.
type UpdateScheduledJobRequest struct {
	Name                *string         `json:"name,omitempty"`
	Description         *string         `json:"description,omitempty"`
	Crons               []string        `json:"crons,omitempty"`
	Timezone            *string         `json:"timezone,omitempty"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          *bool           `json:"concurrent,omitempty"`
	TracksCompletion    *bool           `json:"tracksCompletion,omitempty"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts *int32          `json:"deliveryMaxAttempts,omitempty"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
}

func (r UpdateScheduledJobRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:                  id,
		Name:                r.Name,
		Description:         r.Description,
		Crons:               r.Crons,
		Timezone:            r.Timezone,
		Payload:             r.Payload,
		Concurrent:          r.Concurrent,
		TracksCompletion:    r.TracksCompletion,
		TimeoutSeconds:      r.TimeoutSeconds,
		DeliveryMaxAttempts: r.DeliveryMaxAttempts,
		TargetURL:           r.TargetURL,
	}
}

// ScheduledJobResponse mirrors scheduledjob.ScheduledJob.
type ScheduledJobResponse struct {
	ID                  string           `json:"id"`
	ClientID            *string          `json:"clientId,omitempty"`
	Code                string           `json:"code"`
	Name                string           `json:"name"`
	Description         *string          `json:"description,omitempty"`
	Status              string           `json:"status"`
	Crons               []string         `json:"crons"`
	Timezone            string           `json:"timezone"`
	Payload             json.RawMessage  `json:"payload,omitempty"`
	Concurrent          bool             `json:"concurrent"`
	TracksCompletion    bool             `json:"tracksCompletion"`
	TimeoutSeconds      *int32           `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts int32            `json:"deliveryMaxAttempts"`
	TargetURL           *string          `json:"targetUrl,omitempty"`
	LastFiredAt         *httpcompat.Time `json:"lastFiredAt,omitempty"`
	CreatedAt           httpcompat.Time  `json:"createdAt"`
	UpdatedAt           httpcompat.Time  `json:"updatedAt"`
	CreatedBy           *string          `json:"createdBy,omitempty"`
	UpdatedBy           *string          `json:"updatedBy,omitempty"`
	Version             int32            `json:"version"`
}

func fromEntity(j *scheduledjob.ScheduledJob) ScheduledJobResponse {
	crons := j.Crons
	if crons == nil {
		crons = []string{}
	}
	var lastFired *httpcompat.Time
	if j.LastFiredAt != nil {
		v := jsontime.New(*j.LastFiredAt)
		lastFired = &v
	}
	return ScheduledJobResponse{
		ID:                  j.ID,
		ClientID:            j.ClientID,
		Code:                j.Code,
		Name:                j.Name,
		Description:         j.Description,
		Status:              string(j.Status),
		Crons:               crons,
		Timezone:            j.Timezone,
		Payload:             j.Payload,
		Concurrent:          j.Concurrent,
		TracksCompletion:    j.TracksCompletion,
		TimeoutSeconds:      j.TimeoutSeconds,
		DeliveryMaxAttempts: j.DeliveryMaxAttempts,
		TargetURL:           j.TargetURL,
		LastFiredAt:         lastFired,
		CreatedAt:           jsontime.New(j.CreatedAt),
		UpdatedAt:           jsontime.New(j.UpdatedAt),
		CreatedBy:           j.CreatedBy,
		UpdatedBy:           j.UpdatedBy,
		Version:             j.Version,
	}
}

// ScheduledJobListResponse is the wire shape for GET /api/scheduled-jobs.
type ScheduledJobListResponse struct {
	Items []ScheduledJobResponse `json:"items"`
}

// FireNowResponse is the wire shape for POST /api/scheduled-jobs/{id}/fire-now.
type FireNowResponse struct {
	ScheduledJobID string `json:"scheduledJobId"`
	InstanceID     string `json:"instanceId"`
}
