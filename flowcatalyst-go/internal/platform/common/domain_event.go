package common

import (
	"encoding/json"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// DomainEvent is the interface that all domain events must implement.
// It follows the CloudEvents specification for interoperability.
//
// Domain events represent something that happened in the domain.
// They are immutable facts that are persisted for event sourcing
// and audit trail purposes.
type DomainEvent interface {
	// EventID returns the unique identifier for this event.
	// Format: TSID Crockford Base32 string.
	EventID() string

	// EventType returns the type code for this event.
	// Format: {app}:{domain}:{aggregate}:{action}
	// Example: "platform:control-plane:eventtype:created"
	EventType() string

	// SpecVersion returns the schema version of this event type.
	// Example: "1.0"
	SpecVersion() string

	// Source returns the system that generated this event.
	// Example: "platform:control-plane"
	Source() string

	// Subject returns the qualified aggregate identifier.
	// Format: {domain}.{aggregate}.{id}
	// Example: "platform.eventtype.0HZXEQ5Y8JY5Z"
	Subject() string

	// Time returns when the event occurred.
	Time() time.Time

	// CorrelationID returns the distributed tracing identifier.
	CorrelationID() string

	// CausationID returns the ID of the event that caused this event.
	// May be empty if this is a root event.
	CausationID() string

	// ExecutionID returns the unique ID for this use case execution.
	// Different from EventID - ExecutionID groups all events from one execution.
	ExecutionID() string

	// PrincipalID returns the ID of who initiated the action.
	PrincipalID() string

	// MessageGroup returns the group key for FIFO ordering.
	// Messages with the same group are processed in order.
	// Example: "platform:eventtype:0HZXEQ5Y8JY5Z"
	MessageGroup() string

	// ToDataJSON serializes the event-specific payload to JSON.
	ToDataJSON() string
}

// BaseDomainEvent provides a base implementation of DomainEvent
// that can be embedded in concrete event types.
//
// Example usage:
//
//	type EventTypeCreated struct {
//	    common.BaseDomainEvent
//	    EventTypeID string `json:"eventTypeId"`
//	    Code        string `json:"code"`
//	    Name        string `json:"name"`
//	}
type BaseDomainEvent struct {
	ID          string    `json:"eventId" bson:"_id"`
	Type        string    `json:"eventType" bson:"type"`
	Version     string    `json:"specVersion" bson:"specVersion"`
	Src         string    `json:"source" bson:"source"`
	Subj        string    `json:"subject" bson:"subject"`
	Timestamp   time.Time `json:"time" bson:"time"`
	Correlation string    `json:"correlationId" bson:"correlationId"`
	Causation   string    `json:"causationId,omitempty" bson:"causationId,omitempty"`
	Execution   string    `json:"executionId" bson:"executionId"`
	Principal   string    `json:"principalId" bson:"principalId"`
	MsgGroup    string    `json:"messageGroup" bson:"messageGroup"`
}

// NewBaseDomainEvent creates a new BaseDomainEvent with fields populated
// from the execution context.
func NewBaseDomainEvent(
	ctx *ExecutionContext,
	eventType string,
	subject string,
	messageGroup string,
) BaseDomainEvent {
	return BaseDomainEvent{
		ID:          tsid.Generate(),
		Type:        eventType,
		Version:     "1.0",
		Src:         "platform:control-plane",
		Subj:        subject,
		Timestamp:   time.Now(),
		Correlation: ctx.CorrelationID,
		Causation:   ctx.CausationID,
		Execution:   ctx.ExecutionID,
		Principal:   ctx.PrincipalID,
		MsgGroup:    messageGroup,
	}
}

// NewBaseDomainEventWithVersion creates a BaseDomainEvent with a specific spec version.
func NewBaseDomainEventWithVersion(
	ctx *ExecutionContext,
	eventType string,
	specVersion string,
	subject string,
	messageGroup string,
) BaseDomainEvent {
	base := NewBaseDomainEvent(ctx, eventType, subject, messageGroup)
	base.Version = specVersion
	return base
}

// NewBaseDomainEventWithSource creates a BaseDomainEvent with a custom source.
func NewBaseDomainEventWithSource(
	ctx *ExecutionContext,
	eventType string,
	source string,
	subject string,
	messageGroup string,
) BaseDomainEvent {
	base := NewBaseDomainEvent(ctx, eventType, subject, messageGroup)
	base.Src = source
	return base
}

// DomainEvent interface implementation for BaseDomainEvent

func (e BaseDomainEvent) EventID() string       { return e.ID }
func (e BaseDomainEvent) EventType() string     { return e.Type }
func (e BaseDomainEvent) SpecVersion() string   { return e.Version }
func (e BaseDomainEvent) Source() string        { return e.Src }
func (e BaseDomainEvent) Subject() string       { return e.Subj }
func (e BaseDomainEvent) Time() time.Time       { return e.Timestamp }
func (e BaseDomainEvent) CorrelationID() string { return e.Correlation }
func (e BaseDomainEvent) CausationID() string   { return e.Causation }
func (e BaseDomainEvent) ExecutionID() string   { return e.Execution }
func (e BaseDomainEvent) PrincipalID() string   { return e.Principal }
func (e BaseDomainEvent) MessageGroup() string  { return e.MsgGroup }

// ToDataJSON returns an empty object for the base event.
// Concrete event types should override this to include their payload.
func (e BaseDomainEvent) ToDataJSON() string {
	return "{}"
}

// PersistedEvent represents a domain event as stored in MongoDB.
// It includes the full event structure plus searchable context data.
type PersistedEvent struct {
	ID              string        `bson:"_id" json:"id"`
	SpecVersion     string        `bson:"specVersion" json:"specVersion"`
	Type            string        `bson:"type" json:"type"`
	Source          string        `bson:"source" json:"source"`
	Subject         string        `bson:"subject" json:"subject"`
	Time            time.Time     `bson:"time" json:"time"`
	Data            string        `bson:"data" json:"data"`
	CorrelationID   string        `bson:"correlationId" json:"correlationId"`
	CausationID     string        `bson:"causationId,omitempty" json:"causationId,omitempty"`
	ExecutionID     string        `bson:"executionId" json:"executionId"`
	DeduplicationID string        `bson:"deduplicationId" json:"deduplicationId"`
	MessageGroup    string        `bson:"messageGroup" json:"messageGroup"`
	ContextData     []ContextData `bson:"contextData" json:"contextData"`
	ClientID        string        `bson:"clientId,omitempty" json:"clientId,omitempty"`
}

// ContextData represents searchable key-value metadata on an event.
type ContextData struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// ToPersistedEvent converts a DomainEvent to a PersistedEvent for storage.
func ToPersistedEvent(event DomainEvent, clientID string) *PersistedEvent {
	contextData := []ContextData{
		{Key: "principalId", Value: event.PrincipalID()},
		{Key: "aggregateType", Value: extractAggregateType(event.Subject())},
	}

	return &PersistedEvent{
		ID:              event.EventID(),
		SpecVersion:     event.SpecVersion(),
		Type:            event.EventType(),
		Source:          event.Source(),
		Subject:         event.Subject(),
		Time:            event.Time(),
		Data:            event.ToDataJSON(),
		CorrelationID:   event.CorrelationID(),
		CausationID:     event.CausationID(),
		ExecutionID:     event.ExecutionID(),
		DeduplicationID: event.EventType() + "-" + event.EventID(),
		MessageGroup:    event.MessageGroup(),
		ContextData:     contextData,
		ClientID:        clientID,
	}
}

// extractAggregateType extracts the aggregate type from a subject string.
// Subject format: {domain}.{aggregate}.{id}
// Example: "platform.eventtype.0HZXEQ5Y8JY5Z" -> "eventtype"
func extractAggregateType(subject string) string {
	// Simple extraction - find the second segment
	start := 0
	dotCount := 0
	for i, c := range subject {
		if c == '.' {
			dotCount++
			if dotCount == 1 {
				start = i + 1
			} else if dotCount == 2 {
				return subject[start:i]
			}
		}
	}
	if dotCount == 1 {
		return subject[start:]
	}
	return subject
}

// MarshalDataJSON is a helper to serialize event payload to JSON.
func MarshalDataJSON(data any) string {
	bytes, err := json.Marshal(data)
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
