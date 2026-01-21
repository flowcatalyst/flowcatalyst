package read

import (
	"time"
)

// EventRead is a denormalized read projection of Event for efficient querying
// Collection: events_read
type EventRead struct {
	ID              string        `bson:"_id" json:"id"`
	SpecVersion     string        `bson:"specVersion" json:"specVersion"`
	Type            string        `bson:"type" json:"type"`
	Source          string        `bson:"source" json:"source"`
	Subject         string        `bson:"subject,omitempty" json:"subject,omitempty"`
	Time            time.Time     `bson:"time" json:"time"`
	Data            string        `bson:"data,omitempty" json:"data,omitempty"`
	CorrelationID   string        `bson:"correlationId,omitempty" json:"correlationId,omitempty"`
	CausationID     string        `bson:"causationId,omitempty" json:"causationId,omitempty"`
	DeduplicationID string        `bson:"deduplicationId,omitempty" json:"deduplicationId,omitempty"`
	MessageGroup    string        `bson:"messageGroup,omitempty" json:"messageGroup,omitempty"`
	ContextData     []ContextData `bson:"contextData,omitempty" json:"contextData,omitempty"`
	ClientID        string        `bson:"clientId,omitempty" json:"clientId,omitempty"`
	CreatedAt       time.Time     `bson:"createdAt" json:"createdAt"`

	// Denormalized application info
	ApplicationID   string `bson:"applicationId,omitempty" json:"applicationId,omitempty"`
	ApplicationCode string `bson:"applicationCode,omitempty" json:"applicationCode,omitempty"`
	ApplicationName string `bson:"applicationName,omitempty" json:"applicationName,omitempty"`

	// Denormalized aggregate info (extracted from source/subject)
	SubdomainCode string `bson:"subdomainCode,omitempty" json:"subdomainCode,omitempty"`
	AggregateType string `bson:"aggregateType,omitempty" json:"aggregateType,omitempty"`
	AggregateID   string `bson:"aggregateId,omitempty" json:"aggregateId,omitempty"`

	// Event type info
	EventTypeID   string `bson:"eventTypeId,omitempty" json:"eventTypeId,omitempty"`
	EventTypeCode string `bson:"eventTypeCode,omitempty" json:"eventTypeCode,omitempty"`
	EventTypeName string `bson:"eventTypeName,omitempty" json:"eventTypeName,omitempty"`
}

// ContextData represents key-value metadata on an event
type ContextData struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// GetContextValue returns the value for a context data key
func (e *EventRead) GetContextValue(key string) string {
	for _, cd := range e.ContextData {
		if cd.Key == key {
			return cd.Value
		}
	}
	return ""
}
