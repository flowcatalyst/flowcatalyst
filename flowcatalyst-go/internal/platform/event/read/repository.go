package read

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var ErrNotFound = errors.New("not found")

// Repository provides read access to denormalized events
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates a new EventRead repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		collection: db.Collection("events_read"),
	}
}

// FindByID finds an event by ID
func (r *Repository) FindByID(ctx context.Context, id string) (*EventRead, error) {
	var event EventRead
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&event)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &event, nil
}

// FindByClientID finds events by client ID with pagination
func (r *Repository) FindByClientID(ctx context.Context, clientID string, skip, limit int64) ([]*EventRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"clientId": clientID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindByEventType finds events by event type code
func (r *Repository) FindByEventType(ctx context.Context, eventTypeCode string, skip, limit int64) ([]*EventRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"type": eventTypeCode}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindByApplication finds events by application
func (r *Repository) FindByApplication(ctx context.Context, applicationID string, skip, limit int64) ([]*EventRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"applicationId": applicationID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindByAggregate finds events by aggregate
func (r *Repository) FindByAggregate(ctx context.Context, aggregateType, aggregateID string, skip, limit int64) ([]*EventRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "createdAt", Value: -1}})

	filter := bson.M{
		"aggregateType": aggregateType,
		"aggregateId":   aggregateID,
	}
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindByCorrelationID finds events by correlation ID
func (r *Repository) FindByCorrelationID(ctx context.Context, correlationID string) ([]*EventRead, error) {
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})

	cursor, err := r.collection.Find(ctx, bson.M{"correlationId": correlationID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// FindByTimeRange finds events within a time range
func (r *Repository) FindByTimeRange(ctx context.Context, start, end time.Time, skip, limit int64) ([]*EventRead, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "time", Value: -1}})

	filter := bson.M{
		"time": bson.M{
			"$gte": start,
			"$lte": end,
		},
	}
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var events []*EventRead
	if err := cursor.All(ctx, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// CountByEventType returns the count of events for a given event type
func (r *Repository) CountByEventType(ctx context.Context, eventTypeCode string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"type": eventTypeCode})
}

// CountByClient returns the count of events for a given client
func (r *Repository) CountByClient(ctx context.Context, clientID string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"clientId": clientID})
}

// Upsert inserts or updates an event read projection
func (r *Repository) Upsert(ctx context.Context, event *EventRead) error {
	filter := bson.M{"_id": event.ID}
	opts := options.Replace().SetUpsert(true)
	_, err := r.collection.ReplaceOne(ctx, filter, event, opts)
	return err
}

// Delete removes an event read projection
func (r *Repository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
