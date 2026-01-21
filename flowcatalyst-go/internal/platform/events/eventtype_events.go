package events

import (
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/eventtype"
)

// EventTypeCreated is emitted when a new event type is created
type EventTypeCreated struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

func (e *EventTypeCreated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Category    string `json:"category,omitempty"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
		Name:        e.Name,
		Description: e.Description,
		Category:    e.Category,
	})
}

func NewEventTypeCreated(ctx *common.ExecutionContext, et *eventtype.EventType) *EventTypeCreated {
	return &EventTypeCreated{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeCreated, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
		Name:            et.Name,
		Description:     et.Description,
		Category:        et.Category,
	}
}

// EventTypeUpdated is emitted when an event type is updated
type EventTypeUpdated struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

func (e *EventTypeUpdated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		Category    string `json:"category,omitempty"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
		Name:        e.Name,
		Description: e.Description,
		Category:    e.Category,
	})
}

func NewEventTypeUpdated(ctx *common.ExecutionContext, et *eventtype.EventType) *EventTypeUpdated {
	return &EventTypeUpdated{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeUpdated, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
		Name:            et.Name,
		Description:     et.Description,
		Category:        et.Category,
	}
}

// EventTypeArchived is emitted when an event type is archived
type EventTypeArchived struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
}

func (e *EventTypeArchived) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
	})
}

func NewEventTypeArchived(ctx *common.ExecutionContext, et *eventtype.EventType) *EventTypeArchived {
	return &EventTypeArchived{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeArchived, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
	}
}

// EventTypeSchemaAdded is emitted when a new schema version is added to an event type
type EventTypeSchemaAdded struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
	Version     string `json:"version"`
	MimeType    string `json:"mimeType"`
	SchemaType  string `json:"schemaType"`
}

func (e *EventTypeSchemaAdded) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
		Version     string `json:"version"`
		MimeType    string `json:"mimeType"`
		SchemaType  string `json:"schemaType"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
		Version:     e.Version,
		MimeType:    e.MimeType,
		SchemaType:  e.SchemaType,
	})
}

func NewEventTypeSchemaAdded(ctx *common.ExecutionContext, et *eventtype.EventType, sv *eventtype.SpecVersion) *EventTypeSchemaAdded {
	return &EventTypeSchemaAdded{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeSchemaAdded, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
		Version:         sv.Version,
		MimeType:        sv.MimeType,
		SchemaType:      string(sv.SchemaType),
	}
}

// EventTypeSchemaFinalised is emitted when a schema version is finalised (made current)
type EventTypeSchemaFinalised struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
	Version     string `json:"version"`
}

func (e *EventTypeSchemaFinalised) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
		Version     string `json:"version"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
		Version:     e.Version,
	})
}

func NewEventTypeSchemaFinalised(ctx *common.ExecutionContext, et *eventtype.EventType, version string) *EventTypeSchemaFinalised {
	return &EventTypeSchemaFinalised{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeSchemaFinalised, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
		Version:         version,
	}
}

// EventTypeSchemaDeprecated is emitted when a schema version is deprecated
type EventTypeSchemaDeprecated struct {
	common.BaseDomainEvent
	EventTypeID string `json:"eventTypeId"`
	Code        string `json:"code"`
	Version     string `json:"version"`
}

func (e *EventTypeSchemaDeprecated) ToDataJSON() string {
	return common.MarshalDataJSON(struct {
		EventTypeID string `json:"eventTypeId"`
		Code        string `json:"code"`
		Version     string `json:"version"`
	}{
		EventTypeID: e.EventTypeID,
		Code:        e.Code,
		Version:     e.Version,
	})
}

func NewEventTypeSchemaDeprecated(ctx *common.ExecutionContext, et *eventtype.EventType, version string) *EventTypeSchemaDeprecated {
	return &EventTypeSchemaDeprecated{
		BaseDomainEvent: newBase(ctx, EventTypeEventTypeSchemaDeprecated, "platform", "eventtype", et.ID),
		EventTypeID:     et.ID,
		Code:            et.Code,
		Version:         version,
	}
}
