package eventtype

import (
	"time"
)

// EventTypeStatus represents the status of an event type
type EventTypeStatus string

const (
	EventTypeStatusCurrent  EventTypeStatus = "CURRENT"
	EventTypeStatusArchived EventTypeStatus = "ARCHIVED"
)

// SchemaType represents the type of schema
type SchemaType string

const (
	SchemaTypeJSONSchema SchemaType = "JSON_SCHEMA"
	SchemaTypeProto      SchemaType = "PROTO"
	SchemaTypeXSD        SchemaType = "XSD"
)

// SpecVersionStatus represents the status of a spec version
type SpecVersionStatus string

const (
	SpecVersionStatusFinalising  SpecVersionStatus = "FINALISING"
	SpecVersionStatusCurrent     SpecVersionStatus = "CURRENT"
	SpecVersionStatusDeprecated  SpecVersionStatus = "DEPRECATED"
)

// SpecVersion represents a versioned schema for an event type
type SpecVersion struct {
	Version    string            `bson:"version" json:"version"`       // e.g., "1.0", "1.1", "2.0"
	MimeType   string            `bson:"mimeType" json:"mimeType"`     // e.g., "application/json"
	Schema     string            `bson:"schema" json:"schema"`         // Schema content (JSON Schema, etc.)
	SchemaType SchemaType        `bson:"schemaType" json:"schemaType"` // JSON_SCHEMA, PROTO, XSD
	Status     SpecVersionStatus `bson:"status" json:"status"`         // FINALISING, CURRENT, DEPRECATED
	CreatedAt  time.Time         `bson:"createdAt" json:"createdAt"`
	UpdatedAt  time.Time         `bson:"updatedAt" json:"updatedAt"`
}

// IsCurrent returns true if this spec version is current
func (s *SpecVersion) IsCurrent() bool {
	return s.Status == SpecVersionStatusCurrent
}

// IsDeprecated returns true if this spec version is deprecated
func (s *SpecVersion) IsDeprecated() bool {
	return s.Status == SpecVersionStatusDeprecated
}

// IsFinalising returns true if this spec version is still being finalized
func (s *SpecVersion) IsFinalising() bool {
	return s.Status == SpecVersionStatusFinalising
}

// EventType represents an event type with versioned schemas
// Collection: event_types
type EventType struct {
	ID           string          `bson:"_id" json:"id"`
	Code         string          `bson:"code" json:"code"`
	Name         string          `bson:"name" json:"name"`
	Description  string          `bson:"description,omitempty" json:"description,omitempty"`
	Category     string          `bson:"category,omitempty" json:"category,omitempty"`
	SpecVersions []SpecVersion   `bson:"specVersions,omitempty" json:"specVersions,omitempty"`
	Status       EventTypeStatus `bson:"status" json:"status"`
	CreatedAt    time.Time       `bson:"createdAt" json:"createdAt"`
	UpdatedAt    time.Time       `bson:"updatedAt" json:"updatedAt"`
}

// IsCurrent returns true if the event type is current (not archived)
func (e *EventType) IsCurrent() bool {
	return e.Status == EventTypeStatusCurrent
}

// IsArchived returns true if the event type is archived
func (e *EventType) IsArchived() bool {
	return e.Status == EventTypeStatusArchived
}

// FindSpecVersion finds a spec version by version string
func (e *EventType) FindSpecVersion(version string) *SpecVersion {
	for i := range e.SpecVersions {
		if e.SpecVersions[i].Version == version {
			return &e.SpecVersions[i]
		}
	}
	return nil
}

// HasVersion checks if the event type has a specific version
func (e *EventType) HasVersion(version string) bool {
	return e.FindSpecVersion(version) != nil
}

// GetCurrentVersion returns the current spec version if one exists
func (e *EventType) GetCurrentVersion() *SpecVersion {
	for i := range e.SpecVersions {
		if e.SpecVersions[i].IsCurrent() {
			return &e.SpecVersions[i]
		}
	}
	return nil
}

// AllVersionsDeprecated returns true if all spec versions are deprecated
func (e *EventType) AllVersionsDeprecated() bool {
	if len(e.SpecVersions) == 0 {
		return false
	}
	for _, v := range e.SpecVersions {
		if !v.IsDeprecated() {
			return false
		}
	}
	return true
}

// AddSpecVersion adds a new spec version to the event type
func (e *EventType) AddSpecVersion(sv SpecVersion) *EventType {
	e.SpecVersions = append(e.SpecVersions, sv)
	return e
}

// WithStatus sets the event type status and returns the event type for chaining
func (e *EventType) WithStatus(status EventTypeStatus) *EventType {
	e.Status = status
	return e
}
