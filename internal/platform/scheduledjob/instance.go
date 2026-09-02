package scheduledjob

import (
	"encoding/json"
	"time"
)

// InstanceStatus is the lifecycle state of a single firing.
//
// Terminal states: COMPLETED, FAILED, DELIVERY_FAILED. When the parent
// job has TracksCompletion=false, DELIVERED is also terminal (the
// consumer never acks completion). Non-terminal states feed the
// "currently running" badge via HasActiveInstance.
type InstanceStatus string

const (
	InstanceStatusQueued         InstanceStatus = "QUEUED"
	InstanceStatusInFlight       InstanceStatus = "IN_FLIGHT"
	InstanceStatusDelivered      InstanceStatus = "DELIVERED"
	InstanceStatusCompleted      InstanceStatus = "COMPLETED"
	InstanceStatusFailed         InstanceStatus = "FAILED"
	InstanceStatusDeliveryFailed InstanceStatus = "DELIVERY_FAILED"
)

// ParseInstanceStatus parses a stored/wire instance-status value. Returns
// ok=false for anything other than the six known constants — callers MUST
// reject on ok=false rather than coerce an unrecognised value to QUEUED
// (X-06: a loud read error, never a silent default — a corrupted terminal
// status silently reappearing as QUEUED could resurrect a firing that
// already completed or failed). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseInstanceStatus(s string) (InstanceStatus, bool) {
	switch InstanceStatus(s) {
	case InstanceStatusQueued, InstanceStatusInFlight, InstanceStatusDelivered,
		InstanceStatusCompleted, InstanceStatusFailed, InstanceStatusDeliveryFailed:
		return InstanceStatus(s), true
	default:
		return "", false
	}
}

// TriggerKind tags the cause of a firing.
type TriggerKind string

const (
	TriggerCron     TriggerKind = "CRON"
	TriggerManual   TriggerKind = "MANUAL"
	TriggerBackfill TriggerKind = "BACKFILL"
)

// ParseTriggerKind parses a stored/wire trigger-kind value. Returns
// ok=false for anything other than CRON, MANUAL, or BACKFILL — callers
// MUST reject on ok=false rather than coerce an unrecognised value to CRON
// (X-06: a loud read error, never a silent default). Follows the (T, bool)
// shape of common.ParseOutboxItemType.
func ParseTriggerKind(s string) (TriggerKind, bool) {
	switch TriggerKind(s) {
	case TriggerCron, TriggerManual, TriggerBackfill:
		return TriggerKind(s), true
	default:
		return "", false
	}
}

// ScheduledJobInstance is a single firing of a ScheduledJob. Written by
// the cron poller (internal/platform/scheduler) and read by the BFF +
// API.
type ScheduledJobInstance struct {
	ID               string          `json:"id"`
	ScheduledJobID   string          `json:"scheduledJobId"`
	ClientID         *string         `json:"clientId,omitempty"`
	JobCode          string          `json:"jobCode"`
	TriggerKind      TriggerKind     `json:"triggerKind"`
	ScheduledFor     *time.Time      `json:"scheduledFor,omitempty"`
	FiredAt          time.Time       `json:"firedAt"`
	DeliveredAt      *time.Time      `json:"deliveredAt,omitempty"`
	CompletedAt      *time.Time      `json:"completedAt,omitempty"`
	Status           InstanceStatus  `json:"status"`
	DeliveryAttempts int32           `json:"deliveryAttempts"`
	DeliveryError    *string         `json:"deliveryError,omitempty"`
	CompletionStatus *string         `json:"completionStatus,omitempty"`
	CompletionResult json.RawMessage `json:"completionResult,omitempty"`
	CorrelationID    *string         `json:"correlationId,omitempty"`
	CreatedAt        time.Time       `json:"createdAt"`
}

// ScheduledJobInstanceLog is one log entry attached to an instance. The
// scheduler writes these as the firing progresses (QUEUED → IN_FLIGHT,
// dispatch error retries, completion).
type ScheduledJobInstanceLog struct {
	ID             string          `json:"id"`
	InstanceID     string          `json:"instanceId"`
	ScheduledJobID *string         `json:"scheduledJobId,omitempty"`
	ClientID       *string         `json:"clientId,omitempty"`
	Level          string          `json:"level"`
	Message        string          `json:"message"`
	Metadata       json.RawMessage `json:"metadata,omitempty"`
	CreatedAt      time.Time       `json:"createdAt"`
}

// InstanceListFilters drives the paginated instance list query —
// AND semantics across non-nil fields,
// timestamp window is half-open [From, To).
type InstanceListFilters struct {
	ScheduledJobID *string
	ClientID       *string
	Status         *InstanceStatus
	TriggerKind    *TriggerKind
	From           *time.Time
	To             *time.Time
	Limit          *int64
	Offset         *int64
}
