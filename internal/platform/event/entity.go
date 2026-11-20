// Package event is the CloudEvents 1.0
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/payloadstore"
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

	// StoredData is the msg_payloads row the event's payload_id names
	// (docs/payload-offload-design.md, section 10), as the read joined it;
	// nil for an event of the legacy shape, whose data is its own data
	// column, and for one with no data. When the payload was held in the
	// database Data holds it as the producer sent it; when it was offloaded
	// Data is nil until a read of the one event resolves it (FindByID,
	// FindRawByID, FindRawByIDScoped). Not on the API.
	StoredData *payloadstore.Stored `json:"-"`
	// DataSize is the data's length in bytes: the payload row's size for an
	// event of the new shape, the length of the data read for a legacy one.
	DataSize int `json:"-"`
	// DataOffloaded: a list left the data out because it is in the payload
	// store (lists never read the store); the detail read returns it.
	DataOffloaded bool `json:"-"`
	// DataUnavailable: a read of one event could not produce its data (the
	// store failed, or the payload is missing). The event is still returned.
	DataUnavailable bool `json:"-"`
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
