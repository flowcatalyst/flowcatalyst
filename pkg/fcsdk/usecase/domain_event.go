package usecase

import (
	"strings"
	"time"
)

// DomainEvent is the contract every emitted event must satisfy. Mirrors
// the Rust DomainEvent trait — methods rather than fields so events can
// embed metadata however suits their JSON shape.
type DomainEvent interface {
	EventID() string
	EventType() string   // e.g. "platform:admin:eventtype:created"
	SpecVersion() string // CloudEvents spec version
	Source() string      // e.g. "platform:admin"
	Subject() string     // e.g. "platform.eventtype.evt_01H..."
	Time() time.Time
	PrincipalID() string
	CorrelationID() string
	CausationID() string
	ExecutionID() string
	MessageGroup() string

	// ToDataJSON returns the event-specific data payload as JSON.
	// Implementations typically json.Marshal a struct of their fields.
	ToDataJSON() ([]byte, error)
}

// EventMetadata is the CloudEvents-shaped envelope embedded into every
// domain event. Implementations typically embed this struct and provide
// a custom MarshalJSON that flattens it alongside the event-specific
// data fields, matching the Rust #[serde(flatten)] pattern.
//
// Concrete events still implement the DomainEvent interface explicitly
// (Go's field/method namespace overlap means you can't have both
// `EventID string` and `EventID() string` on the same struct). The
// methods typically delegate to the embedded EventMetadata fields:
//
//	func (e EventFoo) EventID() string       { return e.Metadata.EventID }
//	func (e EventFoo) Time() time.Time       { return e.Metadata.OccurredAt }
//	func (e EventFoo) CorrelationID() string { return e.Metadata.CorrelationID }
//	... etc
type EventMetadata struct {
	EventID       string    `json:"eventId"`
	SpecVersion   string    `json:"specVersion"`
	Source        string    `json:"source"`
	Type          string    `json:"type"`
	Subject       string    `json:"subject"`
	OccurredAt    time.Time `json:"time"`
	CorrelationID string    `json:"correlationId,omitempty"`
	CausationID   string    `json:"causationId,omitempty"`
	PrincipalID   string    `json:"principalId,omitempty"`
	ExecutionID   string    `json:"executionId,omitempty"`
	MessageGroup  string    `json:"messageGroup,omitempty"`
}

// NewEventMetadata builds an EventMetadata from an ExecutionContext
// plus the per-event fields. Convenience constructor — events can also
// construct EventMetadata field-by-field for full control.
func NewEventMetadata(ec ExecutionContext, eventType, source, subject string) EventMetadata {
	return EventMetadata{
		EventID:       newEventID(),
		SpecVersion:   "1.0",
		Source:        source,
		Type:          eventType,
		Subject:       subject,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: ec.CorrelationID,
		CausationID:   ec.CausationID,
		PrincipalID:   ec.PrincipalID,
		ExecutionID:   ec.ExecutionID,
	}
}

// ExtractAggregateType maps "platform.eventtype.123" → "Eventtype".
// Used by EventSink implementations to populate the aggregate_type
// column on outbox / audit rows.
func ExtractAggregateType(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) < 2 {
		return "Unknown"
	}
	s := parts[1]
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ExtractEntityID maps "platform.eventtype.123" → "123".
func ExtractEntityID(subject string) string {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 {
		return ""
	}
	return parts[2]
}

// BuildSubject is the convention for forming a subject from
// domain/aggregate/id. Mirrors the TS SDK helper.
func BuildSubject(domain, aggregate, id string) string {
	return domain + "." + aggregate + "." + id
}

// BuildMessageGroup is the convention for forming a message group.
func BuildMessageGroup(domain, aggregate, id string) string {
	return domain + ":" + aggregate + ":" + id
}

// BuildEventType is the convention for forming an event type.
func BuildEventType(app, domain, aggregate, action string) string {
	return app + ":" + domain + ":" + aggregate + ":" + action
}
