package event

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicateCode     = errors.New("duplicate code")
	ErrDuplicateEvent    = errors.New("duplicate event")
)

// mongoRepository provides MongoDB access to event data
type mongoRepository struct {
	events     *mongo.Collection
	eventTypes *mongo.Collection
	schemas    *mongo.Collection
}

// NewRepository creates a new event repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		events:     db.Collection("events"),
		eventTypes: db.Collection("event_types"),
		schemas:    db.Collection("schemas"),
	})
}

// === Event operations ===

// FindEventByID finds an event by ID
func (r *mongoRepository) FindEventByID(ctx context.Context, id string) (*Event, error) {
	var event Event
	err := r.events.FindOne(ctx, bson.M{"_id": id}).Decode(&event)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &event, nil
}

// FindEventsByType finds events by type with pagination
func (r *mongoRepository) FindEventsByType(ctx context.Context, eventType string, skip, limit int64) ([]*Event, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.events.Find(ctx, bson.M{"type": eventType}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindEventsByClient finds events for a client with pagination
func (r *mongoRepository) FindEventsByClient(ctx context.Context, clientID string, skip, limit int64) ([]*Event, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.events.Find(ctx, bson.M{"clientId": clientID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*Event
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// InsertEvent creates a new event
func (r *mongoRepository) InsertEvent(ctx context.Context, event *Event) error {
	if event.ID == "" {
		event.ID = tsid.Generate()
	}
	event.CreatedAt = time.Now()

	_, err := r.events.InsertOne(ctx, event)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateEvent
	}
	return err
}

// InsertEvents inserts multiple events
func (r *mongoRepository) InsertEvents(ctx context.Context, events []*Event) error {
	now := time.Now()
	docs := make([]interface{}, len(events))
	for i, e := range events {
		if e.ID == "" {
			e.ID = tsid.Generate()
		}
		e.CreatedAt = now
		docs[i] = e
	}

	_, err := r.events.InsertMany(ctx, docs)
	return err
}

// CountEvents counts events matching a filter
func (r *mongoRepository) CountEvents(ctx context.Context, filter bson.M) (int64, error) {
	return r.events.CountDocuments(ctx, filter)
}

// === Event Type operations ===

// FindEventTypeByID finds an event type by ID
func (r *mongoRepository) FindEventTypeByID(ctx context.Context, id string) (*EventType, error) {
	var eventType EventType
	err := r.eventTypes.FindOne(ctx, bson.M{"_id": id}).Decode(&eventType)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &eventType, nil
}

// FindEventTypeByCode finds an event type by code
func (r *mongoRepository) FindEventTypeByCode(ctx context.Context, code string) (*EventType, error) {
	var eventType EventType
	err := r.eventTypes.FindOne(ctx, bson.M{"code": code}).Decode(&eventType)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &eventType, nil
}

// FindAllEventTypes returns all event types
func (r *mongoRepository) FindAllEventTypes(ctx context.Context) ([]*EventType, error) {
	cursor, err := r.eventTypes.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var eventTypes []*EventType
	if err := cursor.All(ctx, &eventTypes); err != nil {
		return nil, err
	}
	return eventTypes, nil
}

// FindActiveEventTypes returns all active event types
func (r *mongoRepository) FindActiveEventTypes(ctx context.Context) ([]*EventType, error) {
	cursor, err := r.eventTypes.Find(ctx, bson.M{"status": EventTypeStatusCurrent})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var eventTypes []*EventType
	if err := cursor.All(ctx, &eventTypes); err != nil {
		return nil, err
	}
	return eventTypes, nil
}

// InsertEventType creates a new event type
func (r *mongoRepository) InsertEventType(ctx context.Context, eventType *EventType) error {
	if eventType.ID == "" {
		eventType.ID = tsid.Generate()
	}
	now := time.Now()
	eventType.CreatedAt = now
	eventType.UpdatedAt = now

	_, err := r.eventTypes.InsertOne(ctx, eventType)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateCode
	}
	return err
}

// UpdateEventType updates an existing event type
func (r *mongoRepository) UpdateEventType(ctx context.Context, eventType *EventType) error {
	eventType.UpdatedAt = time.Now()

	result, err := r.eventTypes.ReplaceOne(ctx, bson.M{"_id": eventType.ID}, eventType)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteEventType removes an event type
func (r *mongoRepository) DeleteEventType(ctx context.Context, id string) error {
	result, err := r.eventTypes.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// ArchiveEventType sets an event type to archived status
func (r *mongoRepository) ArchiveEventType(ctx context.Context, id string) error {
	result, err := r.eventTypes.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":    EventTypeStatusArchive,
			"updatedAt": time.Now(),
		}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// === Schema operations ===

// FindSchemaByID finds a schema by ID
func (r *mongoRepository) FindSchemaByID(ctx context.Context, id string) (*Schema, error) {
	var schema Schema
	err := r.schemas.FindOne(ctx, bson.M{"_id": id}).Decode(&schema)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &schema, nil
}

// FindSchemasByEventType finds schemas for an event type
func (r *mongoRepository) FindSchemasByEventType(ctx context.Context, eventTypeID string) ([]*Schema, error) {
	cursor, err := r.schemas.Find(ctx, bson.M{"eventTypeId": eventTypeID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var schemas []*Schema
	if err := cursor.All(ctx, &schemas); err != nil {
		return nil, err
	}
	return schemas, nil
}

// InsertSchema creates a new schema
func (r *mongoRepository) InsertSchema(ctx context.Context, schema *Schema) error {
	if schema.ID == "" {
		schema.ID = tsid.Generate()
	}
	now := time.Now()
	schema.CreatedAt = now
	schema.UpdatedAt = now

	_, err := r.schemas.InsertOne(ctx, schema)
	return err
}

// UpdateSchema updates an existing schema
func (r *mongoRepository) UpdateSchema(ctx context.Context, schema *Schema) error {
	schema.UpdatedAt = time.Now()

	result, err := r.schemas.ReplaceOne(ctx, bson.M{"_id": schema.ID}, schema)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSchema removes a schema
func (r *mongoRepository) DeleteSchema(ctx context.Context, id string) error {
	result, err := r.schemas.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
