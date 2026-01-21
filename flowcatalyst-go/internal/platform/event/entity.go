package event

import (
	"time"
)

// Event represents a domain event
// Collection: events
type Event struct {
	ID              string        `bson:"_id" json:"id"`
	SpecVersion     string        `bson:"specVersion" json:"specVersion"`
	Type            string        `bson:"type" json:"type"`         // Event type code
	Source          string        `bson:"source" json:"source"`     // Origin system
	Subject         string        `bson:"subject,omitempty" json:"subject,omitempty"`
	Time            time.Time     `bson:"time" json:"time"`
	Data            string        `bson:"data,omitempty" json:"data,omitempty"` // JSON payload
	CorrelationID   string        `bson:"correlationId,omitempty" json:"correlationId,omitempty"`
	CausationID     string        `bson:"causationId,omitempty" json:"causationId,omitempty"`
	DeduplicationID string        `bson:"deduplicationId,omitempty" json:"deduplicationId,omitempty"`
	MessageGroup    string        `bson:"messageGroup,omitempty" json:"messageGroup,omitempty"`
	ContextData     []ContextData `bson:"contextData,omitempty" json:"contextData,omitempty"`
	ClientID        string        `bson:"clientId,omitempty" json:"clientId,omitempty"` // Tenant isolation
	CreatedAt       time.Time     `bson:"createdAt" json:"createdAt"`
}

// ContextData represents key-value metadata on an event
type ContextData struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// GetContextValue returns the value for a context data key
func (e *Event) GetContextValue(key string) string {
	for _, cd := range e.ContextData {
		if cd.Key == key {
			return cd.Value
		}
	}
	return ""
}

// EventTypeStatus defines the status of an event type
type EventTypeStatus string

const (
	EventTypeStatusCurrent EventTypeStatus = "CURRENT"
	EventTypeStatusArchive EventTypeStatus = "ARCHIVE"
)

// SpecVersionStatus defines the status of a spec version
type SpecVersionStatus string

const (
	SpecVersionStatusFinalising SpecVersionStatus = "FINALISING"
	SpecVersionStatusCurrent    SpecVersionStatus = "CURRENT"
	SpecVersionStatusDeprecated SpecVersionStatus = "DEPRECATED"
)

// SchemaType defines the schema format
type SchemaType string

const (
	SchemaTypeJSONSchema SchemaType = "JSON_SCHEMA"
	SchemaTypeProto      SchemaType = "PROTO"
	SchemaTypeXSD        SchemaType = "XSD"
)

// EventType represents a type of event that can be published
// Collection: event_types
type EventType struct {
	ID           string          `bson:"_id" json:"id"`
	Code         string          `bson:"code" json:"code"` // Unique event type code
	Name         string          `bson:"name" json:"name"`
	Description  string          `bson:"description,omitempty" json:"description,omitempty"`
	SpecVersions []SpecVersion   `bson:"specVersions,omitempty" json:"specVersions,omitempty"`
	Status       EventTypeStatus `bson:"status" json:"status"`
	CreatedAt    time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time       `bson:"updatedAt" json:"updatedAt"`
}

// SpecVersion represents a version of an event type schema
type SpecVersion struct {
	Version    string            `bson:"version" json:"version"`
	MimeType   string            `bson:"mimeType" json:"mimeType"`
	Schema     string            `bson:"schema,omitempty" json:"schema,omitempty"`
	SchemaType SchemaType        `bson:"schemaType,omitempty" json:"schemaType,omitempty"`
	Status     SpecVersionStatus `bson:"status" json:"status"`
}

// GetCurrentVersion returns the current spec version
func (et *EventType) GetCurrentVersion() *SpecVersion {
	for i := range et.SpecVersions {
		if et.SpecVersions[i].Status == SpecVersionStatusCurrent {
			return &et.SpecVersions[i]
		}
	}
	return nil
}

// IsActive returns true if the event type is current (not archived)
func (et *EventType) IsActive() bool {
	return et.Status == EventTypeStatusCurrent
}

// Schema represents a standalone schema definition
// Collection: schemas
type Schema struct {
	ID          string     `bson:"_id" json:"id"`
	Name        string     `bson:"name" json:"name"`
	Description string     `bson:"description,omitempty" json:"description,omitempty"`
	MimeType    string     `bson:"mimeType" json:"mimeType"`
	SchemaType  SchemaType `bson:"schemaType" json:"schemaType"`
	Content     string     `bson:"content" json:"content"`
	EventTypeID string     `bson:"eventTypeId,omitempty" json:"eventTypeId,omitempty"`
	Version     string     `bson:"version,omitempty" json:"version,omitempty"`
	CreatedAt   time.Time  `bson:"createdAt" json:"createdAt"`
	UpdatedAt   time.Time  `bson:"updatedAt" json:"updatedAt"`
}
