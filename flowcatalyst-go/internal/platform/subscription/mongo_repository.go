package subscription

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
	ErrNotFound      = errors.New("not found")
	ErrDuplicateCode = errors.New("duplicate code")
)

// mongoRepository provides MongoDB access to subscription data
type mongoRepository struct {
	subscriptions *mongo.Collection
}

// NewRepository creates a new subscription repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		subscriptions: db.Collection("subscriptions"),
	})
}

// === Subscription operations ===

// FindSubscriptionByID finds a subscription by ID
func (r *mongoRepository) FindSubscriptionByID(ctx context.Context, id string) (*Subscription, error) {
	var sub Subscription
	err := r.subscriptions.FindOne(ctx, bson.M{"_id": id}).Decode(&sub)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

// FindSubscriptionByCode finds a subscription by code
func (r *mongoRepository) FindSubscriptionByCode(ctx context.Context, code string) (*Subscription, error) {
	var sub Subscription
	err := r.subscriptions.FindOne(ctx, bson.M{"code": code}).Decode(&sub)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &sub, nil
}

// FindSubscriptionsByClient finds all subscriptions for a client
func (r *mongoRepository) FindSubscriptionsByClient(ctx context.Context, clientID string) ([]*Subscription, error) {
	cursor, err := r.subscriptions.Find(ctx, bson.M{"clientId": clientID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subs []*Subscription
	if err := cursor.All(ctx, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// FindActiveSubscriptions finds all active subscriptions
func (r *mongoRepository) FindActiveSubscriptions(ctx context.Context) ([]*Subscription, error) {
	cursor, err := r.subscriptions.Find(ctx, bson.M{"status": SubscriptionStatusActive})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subs []*Subscription
	if err := cursor.All(ctx, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// FindSubscriptionsByEventType finds all active subscriptions matching an event type
func (r *mongoRepository) FindSubscriptionsByEventType(ctx context.Context, eventTypeCode string) ([]*Subscription, error) {
	filter := bson.M{
		"status":                    SubscriptionStatusActive,
		"eventTypes.eventTypeCode": eventTypeCode,
	}

	cursor, err := r.subscriptions.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subs []*Subscription
	if err := cursor.All(ctx, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// FindAllSubscriptions returns all subscriptions with pagination
func (r *mongoRepository) FindAllSubscriptions(ctx context.Context, skip, limit int64) ([]*Subscription, error) {
	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "name", Value: 1}})

	cursor, err := r.subscriptions.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var subs []*Subscription
	if err := cursor.All(ctx, &subs); err != nil {
		return nil, err
	}
	return subs, nil
}

// InsertSubscription creates a new subscription
func (r *mongoRepository) InsertSubscription(ctx context.Context, sub *Subscription) error {
	if sub.ID == "" {
		sub.ID = tsid.Generate()
	}
	now := time.Now()
	sub.CreatedAt = now
	sub.UpdatedAt = now

	_, err := r.subscriptions.InsertOne(ctx, sub)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateCode
	}
	return err
}

// UpdateSubscription updates an existing subscription
func (r *mongoRepository) UpdateSubscription(ctx context.Context, sub *Subscription) error {
	sub.UpdatedAt = time.Now()

	result, err := r.subscriptions.ReplaceOne(ctx, bson.M{"_id": sub.ID}, sub)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateSubscriptionStatus updates a subscription's status
func (r *mongoRepository) UpdateSubscriptionStatus(ctx context.Context, id string, status SubscriptionStatus) error {
	result, err := r.subscriptions.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"status":    status,
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

// DeleteSubscription removes a subscription
func (r *mongoRepository) DeleteSubscription(ctx context.Context, id string) error {
	result, err := r.subscriptions.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}
