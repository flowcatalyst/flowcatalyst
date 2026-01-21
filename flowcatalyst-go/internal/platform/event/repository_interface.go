package event

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
)

// Repository defines the interface for event data access.
// All implementations must be wrapped with instrumentation.
type Repository interface {
	// Event operations
	FindEventByID(ctx context.Context, id string) (*Event, error)
	FindEventsByType(ctx context.Context, eventType string, skip, limit int64) ([]*Event, error)
	FindEventsByClient(ctx context.Context, clientID string, skip, limit int64) ([]*Event, error)
	InsertEvent(ctx context.Context, event *Event) error
	InsertEvents(ctx context.Context, events []*Event) error
	CountEvents(ctx context.Context, filter bson.M) (int64, error)

	// Event Type operations
	FindEventTypeByID(ctx context.Context, id string) (*EventType, error)
	FindEventTypeByCode(ctx context.Context, code string) (*EventType, error)
	FindAllEventTypes(ctx context.Context) ([]*EventType, error)
	FindActiveEventTypes(ctx context.Context) ([]*EventType, error)
	InsertEventType(ctx context.Context, eventType *EventType) error
	UpdateEventType(ctx context.Context, eventType *EventType) error
	DeleteEventType(ctx context.Context, id string) error
	ArchiveEventType(ctx context.Context, id string) error

	// Schema operations
	FindSchemaByID(ctx context.Context, id string) (*Schema, error)
	FindSchemasByEventType(ctx context.Context, eventTypeID string) ([]*Schema, error)
	InsertSchema(ctx context.Context, schema *Schema) error
	UpdateSchema(ctx context.Context, schema *Schema) error
	DeleteSchema(ctx context.Context, id string) error
}
