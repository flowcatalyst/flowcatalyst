// Package scheduledjob is the port of fc-platform/src/scheduled_job.
// Cron-driven job definitions. The actual cron firing loop (poller +
// dispatcher + stale-recovery) lives in internal/platform/scheduler in
// Wave 3g — this subdomain owns the aggregate + admin CRUD only.
//
// Phase 3e ships: entity, repository, events, and the 7 admin ops
// (create, update, archive, delete, pause, resume, fire_now). `sync` is
// deferred to a follow-up alongside the other sync ops.
package scheduledjob

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the job lifecycle state.
type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusPaused   Status = "PAUSED"
	StatusArchived Status = "ARCHIVED"
)

// ParseStatus is the lenient parser. Unknown → ACTIVE.
func ParseStatus(s string) Status {
	switch s {
	case string(StatusPaused):
		return StatusPaused
	case string(StatusArchived):
		return StatusArchived
	default:
		return StatusActive
	}
}

// ScheduledJob is the aggregate root.
type ScheduledJob struct {
	ID                  string          `json:"id"`
	ClientID            *string         `json:"clientId,omitempty"`
	ApplicationID       *string         `json:"applicationId,omitempty"`
	Code                string          `json:"code"`
	Name                string          `json:"name"`
	Description         *string         `json:"description,omitempty"`
	Status              Status          `json:"status"`
	Crons               []string        `json:"crons"`
	Timezone            string          `json:"timezone"`
	Payload             json.RawMessage `json:"payload,omitempty"`
	Concurrent          bool            `json:"concurrent"`
	TracksCompletion    bool            `json:"tracksCompletion"`
	TimeoutSeconds      *int32          `json:"timeoutSeconds,omitempty"`
	DeliveryMaxAttempts int32           `json:"deliveryMaxAttempts"`
	TargetURL           *string         `json:"targetUrl,omitempty"`
	LastFiredAt         *time.Time      `json:"lastFiredAt,omitempty"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
	CreatedBy           *string         `json:"createdBy,omitempty"`
	UpdatedBy           *string         `json:"updatedBy,omitempty"`
	Version             int32           `json:"version"`
}

// IDStr satisfies usecase.HasID.
func (j ScheduledJob) IDStr() string { return j.ID }

// New constructs an ACTIVE ScheduledJob.
func New(code, name string, crons []string) *ScheduledJob {
	now := time.Now().UTC()
	return &ScheduledJob{
		ID:                  tsid.Generate(tsid.ScheduledJob),
		Code:                code,
		Name:                name,
		Status:              StatusActive,
		Crons:               crons,
		Timezone:            "UTC",
		Concurrent:          false,
		TracksCompletion:    false,
		DeliveryMaxAttempts: 3,
		CreatedAt:           now,
		UpdatedAt:           now,
		Version:             1,
	}
}

// Pause transitions to PAUSED.
func (j *ScheduledJob) Pause() {
	j.Status = StatusPaused
	j.UpdatedAt = time.Now().UTC()
	j.Version++
}

// Resume transitions to ACTIVE.
func (j *ScheduledJob) Resume() {
	j.Status = StatusActive
	j.UpdatedAt = time.Now().UTC()
	j.Version++
}

// Archive transitions to ARCHIVED.
func (j *ScheduledJob) Archive() {
	j.Status = StatusArchived
	j.UpdatedAt = time.Now().UTC()
	j.Version++
}

// MarkFired records the cron-slot timestamp the poller just fired.
// Called by internal/platform/scheduler in Wave 3g.
func (j *ScheduledJob) MarkFired(slot time.Time) {
	j.LastFiredAt = &slot
	j.UpdatedAt = time.Now().UTC()
}
