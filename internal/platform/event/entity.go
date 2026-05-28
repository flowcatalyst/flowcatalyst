// Package event is the port of fc-platform/src/event. CloudEvents 1.0
// envelope stored in msg_events.
//
// Per docs/conventions.md §3, this is an infrastructure path: rows are
// written by use cases via usecasepgx.Sink (platform sink writes to
// msg_events directly + aud_logs). This subdomain owns the entity
// shape for the read API + the batch-ingest endpoint (consumer apps
// POST events through the SDK outbox processor).
package event

import (
	"encoding/json"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// ContextEntry is a key/value pair on the event's context array.
type ContextEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Event is the CloudEvents-shaped envelope. Maps to msg_events on the
// write side; reads come from the denormalised msg_events_read.
//
// Schema split: msg_events has `time` (the CloudEvents-spec event time)
// + `created_at` (DB insertion). msg_events_read drops `context_data`
// and adds split-out `application`/`subdomain`/`aggregate`.
// `Context` is therefore populated on the write side (from the SDK
// batch ingest) and empty on the read side — callers wanting per-event
// context use the msg_events table directly.
type Event struct {
	ID              string          `json:"id"`
	SpecVersion     string          `json:"specVersion"`
	Type            string          `json:"type"`
	Source          string          `json:"source"`
	Subject         string          `json:"subject"`
	Time            time.Time       `json:"time"`
	Data            json.RawMessage `json:"data,omitempty"`
	Context         []ContextEntry  `json:"context,omitempty"`
	DeduplicationID string          `json:"deduplicationId"`
	ClientID        *string         `json:"clientId,omitempty"`
	MessageGroup    *string         `json:"messageGroup,omitempty"`
	CorrelationID   *string         `json:"correlationId,omitempty"`
	CausationID     *string         `json:"causationId,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`

	// Read-projection fields (msg_events_read). Empty/zero on the write
	// side; populated by the read queries.
	Application *string    `json:"application,omitempty"`
	Subdomain   *string    `json:"subdomain,omitempty"`
	Aggregate   *string    `json:"aggregate,omitempty"`
	ProjectedAt *time.Time `json:"projectedAt,omitempty"`
}

// New constructs an Event with a fresh untyped TSID. Used by the
// batch-ingest endpoint when callers don't supply an ID.
func New(eventType, source, subject string, data json.RawMessage) *Event {
	now := time.Now().UTC()
	return &Event{
		ID:              tsid.GenerateUntyped(),
		SpecVersion:     "1.0",
		Type:            eventType,
		Source:          source,
		Subject:         subject,
		Time:            now,
		Data:            data,
		Context:         []ContextEntry{},
		DeduplicationID: eventType + "-" + tsid.GenerateUntyped(),
		CreatedAt:       now,
	}
}

// PrincipalID extracts the principal ID from the context array, if set.
func (e *Event) PrincipalID() string {
	for _, c := range e.Context {
		if c.Key == "principalId" {
			return c.Value
		}
	}
	return ""
}

// AggregateType extracts aggregateType from the context array, if set.
func (e *Event) AggregateType() string {
	for _, c := range e.Context {
		if c.Key == "aggregateType" {
			return c.Value
		}
	}
	return ""
}
