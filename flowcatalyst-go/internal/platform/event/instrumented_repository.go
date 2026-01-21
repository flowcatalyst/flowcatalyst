package event

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"

	"go.flowcatalyst.tech/internal/common/repository"
)

const (
	collectionEvents     = "events"
	collectionEventTypes = "event_types"
	collectionSchemas    = "schemas"
)

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

// === Event operations ===

func (r *instrumentedRepository) FindEventByID(ctx context.Context, id string) (*Event, error) {
	return repository.Instrument(ctx, collectionEvents, "FindEventByID", func() (*Event, error) {
		return r.inner.FindEventByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindEventsByType(ctx context.Context, eventType string, skip, limit int64) ([]*Event, error) {
	return repository.Instrument(ctx, collectionEvents, "FindEventsByType", func() ([]*Event, error) {
		return r.inner.FindEventsByType(ctx, eventType, skip, limit)
	})
}

func (r *instrumentedRepository) FindEventsByClient(ctx context.Context, clientID string, skip, limit int64) ([]*Event, error) {
	return repository.Instrument(ctx, collectionEvents, "FindEventsByClient", func() ([]*Event, error) {
		return r.inner.FindEventsByClient(ctx, clientID, skip, limit)
	})
}

func (r *instrumentedRepository) InsertEvent(ctx context.Context, event *Event) error {
	return repository.InstrumentVoid(ctx, collectionEvents, "InsertEvent", func() error {
		return r.inner.InsertEvent(ctx, event)
	})
}

func (r *instrumentedRepository) InsertEvents(ctx context.Context, events []*Event) error {
	return repository.InstrumentVoid(ctx, collectionEvents, "InsertEvents", func() error {
		return r.inner.InsertEvents(ctx, events)
	})
}

func (r *instrumentedRepository) CountEvents(ctx context.Context, filter bson.M) (int64, error) {
	return repository.Instrument(ctx, collectionEvents, "CountEvents", func() (int64, error) {
		return r.inner.CountEvents(ctx, filter)
	})
}

// === Event Type operations ===

func (r *instrumentedRepository) FindEventTypeByID(ctx context.Context, id string) (*EventType, error) {
	return repository.Instrument(ctx, collectionEventTypes, "FindEventTypeByID", func() (*EventType, error) {
		return r.inner.FindEventTypeByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindEventTypeByCode(ctx context.Context, code string) (*EventType, error) {
	return repository.Instrument(ctx, collectionEventTypes, "FindEventTypeByCode", func() (*EventType, error) {
		return r.inner.FindEventTypeByCode(ctx, code)
	})
}

func (r *instrumentedRepository) FindAllEventTypes(ctx context.Context) ([]*EventType, error) {
	return repository.Instrument(ctx, collectionEventTypes, "FindAllEventTypes", func() ([]*EventType, error) {
		return r.inner.FindAllEventTypes(ctx)
	})
}

func (r *instrumentedRepository) FindActiveEventTypes(ctx context.Context) ([]*EventType, error) {
	return repository.Instrument(ctx, collectionEventTypes, "FindActiveEventTypes", func() ([]*EventType, error) {
		return r.inner.FindActiveEventTypes(ctx)
	})
}

func (r *instrumentedRepository) InsertEventType(ctx context.Context, eventType *EventType) error {
	return repository.InstrumentVoid(ctx, collectionEventTypes, "InsertEventType", func() error {
		return r.inner.InsertEventType(ctx, eventType)
	})
}

func (r *instrumentedRepository) UpdateEventType(ctx context.Context, eventType *EventType) error {
	return repository.InstrumentVoid(ctx, collectionEventTypes, "UpdateEventType", func() error {
		return r.inner.UpdateEventType(ctx, eventType)
	})
}

func (r *instrumentedRepository) DeleteEventType(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionEventTypes, "DeleteEventType", func() error {
		return r.inner.DeleteEventType(ctx, id)
	})
}

func (r *instrumentedRepository) ArchiveEventType(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionEventTypes, "ArchiveEventType", func() error {
		return r.inner.ArchiveEventType(ctx, id)
	})
}

// === Schema operations ===

func (r *instrumentedRepository) FindSchemaByID(ctx context.Context, id string) (*Schema, error) {
	return repository.Instrument(ctx, collectionSchemas, "FindSchemaByID", func() (*Schema, error) {
		return r.inner.FindSchemaByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindSchemasByEventType(ctx context.Context, eventTypeID string) ([]*Schema, error) {
	return repository.Instrument(ctx, collectionSchemas, "FindSchemasByEventType", func() ([]*Schema, error) {
		return r.inner.FindSchemasByEventType(ctx, eventTypeID)
	})
}

func (r *instrumentedRepository) InsertSchema(ctx context.Context, schema *Schema) error {
	return repository.InstrumentVoid(ctx, collectionSchemas, "InsertSchema", func() error {
		return r.inner.InsertSchema(ctx, schema)
	})
}

func (r *instrumentedRepository) UpdateSchema(ctx context.Context, schema *Schema) error {
	return repository.InstrumentVoid(ctx, collectionSchemas, "UpdateSchema", func() error {
		return r.inner.UpdateSchema(ctx, schema)
	})
}

func (r *instrumentedRepository) DeleteSchema(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionSchemas, "DeleteSchema", func() error {
		return r.inner.DeleteSchema(ctx, id)
	})
}
