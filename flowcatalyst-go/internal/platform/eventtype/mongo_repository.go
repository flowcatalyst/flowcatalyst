package eventtype

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

// mongoRepository provides MongoDB access to event type data
type mongoRepository struct {
	collection *mongo.Collection
}

// NewRepository creates a new event type repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		collection: db.Collection("event_types"),
	})
}

// FindAll finds all event types
func (r *mongoRepository) FindAll(ctx context.Context) ([]*EventType, error) {
	cursor, err := r.collection.Find(ctx, bson.M{}, options.Find().SetSort(bson.M{"code": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var types []*EventType
	if err := cursor.All(ctx, &types); err != nil {
		return nil, err
	}
	return types, nil
}

// FindByID finds an event type by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*EventType, error) {
	var et EventType
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&et)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &et, nil
}

// FindByCode finds an event type by code
func (r *mongoRepository) FindByCode(ctx context.Context, code string) (*EventType, error) {
	var et EventType
	err := r.collection.FindOne(ctx, bson.M{"code": code}).Decode(&et)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &et, nil
}

// Insert inserts a new event type
func (r *mongoRepository) Insert(ctx context.Context, et *EventType) error {
	et.ID = tsid.Generate()
	et.CreatedAt = time.Now()
	et.UpdatedAt = time.Now()
	_, err := r.collection.InsertOne(ctx, et)
	return err
}

// Update updates an event type
func (r *mongoRepository) Update(ctx context.Context, et *EventType) error {
	et.UpdatedAt = time.Now()
	_, err := r.collection.UpdateByID(ctx, et.ID, bson.M{"$set": et})
	return err
}

// Delete deletes an event type
func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	_, err := r.collection.DeleteOne(ctx, bson.M{"_id": id})
	return err
}
