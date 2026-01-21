package outbox

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoRepository implements Repository for MongoDB.
// Uses simple find/updateMany with status codes - NO findOneAndUpdate loop.
// Safe because only one poller runs (enforced by leader election).
type MongoRepository struct {
	db     *mongo.Database
	config *RepositoryConfig
}

// NewMongoRepository creates a new MongoDB outbox repository
func NewMongoRepository(db *mongo.Database, config *RepositoryConfig) *MongoRepository {
	if config == nil {
		config = DefaultRepositoryConfig()
	}
	return &MongoRepository{
		db:     db,
		config: config,
	}
}

// GetTableName returns the collection name for the item type
func (r *MongoRepository) GetTableName(itemType OutboxItemType) string {
	switch itemType {
	case OutboxItemTypeEvent:
		return r.config.EventsTable
	case OutboxItemTypeDispatchJob:
		return r.config.DispatchJobsTable
	default:
		return r.config.EventsTable
	}
}

// getCollection returns the MongoDB collection for the item type
func (r *MongoRepository) getCollection(itemType OutboxItemType) *mongo.Collection {
	return r.db.Collection(r.GetTableName(itemType))
}

// FetchPending fetches pending items (status=0) ordered by messageGroup, createdAt.
// Simple find with no atomic update - safe because only one poller runs.
func (r *MongoRepository) FetchPending(ctx context.Context, itemType OutboxItemType, limit int) ([]*OutboxItem, error) {
	collection := r.getCollection(itemType)

	filter := bson.M{"status": int(StatusPending)}
	opts := options.Find().
		SetSort(bson.D{{Key: "messageGroup", Value: 1}, {Key: "createdAt", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("fetch pending: %w", err)
	}
	defer cursor.Close(ctx)

	return r.decodeCursor(ctx, cursor)
}

// MarkAsInProgress marks items as in-progress (status=9).
func (r *MongoRepository) MarkAsInProgress(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":    int(StatusInProgress),
			"updatedAt": time.Now(),
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("mark as in-progress: %w", err)
	}
	return nil
}

// MarkWithStatus updates items to the specified status code.
func (r *MongoRepository) MarkWithStatus(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":    int(status),
			"updatedAt": time.Now(),
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("mark with status %d: %w", status, err)
	}
	return nil
}

// MarkWithStatusAndError updates items to the specified status with an error message.
func (r *MongoRepository) MarkWithStatusAndError(ctx context.Context, itemType OutboxItemType, ids []string, status OutboxStatus, errorMessage string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":       int(status),
			"errorMessage": errorMessage,
			"updatedAt":    time.Now(),
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("mark with status %d and error: %w", status, err)
	}
	return nil
}

// FetchStuckItems fetches items stuck in in-progress status (status=9).
func (r *MongoRepository) FetchStuckItems(ctx context.Context, itemType OutboxItemType) ([]*OutboxItem, error) {
	collection := r.getCollection(itemType)

	filter := bson.M{"status": int(StatusInProgress)}
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("fetch stuck items: %w", err)
	}
	defer cursor.Close(ctx)

	return r.decodeCursor(ctx, cursor)
}

// ResetStuckItems resets stuck items back to pending (status=0).
func (r *MongoRepository) ResetStuckItems(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":    int(StatusPending),
			"updatedAt": time.Now(),
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("reset stuck items: %w", err)
	}
	return nil
}

// IncrementRetryCount increments the retry count for items and resets to pending.
func (r *MongoRepository) IncrementRetryCount(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":    int(StatusPending),
			"updatedAt": time.Now(),
		},
		"$inc": bson.M{
			"retryCount": 1,
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("increment retry count: %w", err)
	}
	return nil
}

// FetchRecoverableItems fetches items eligible for periodic recovery.
func (r *MongoRepository) FetchRecoverableItems(ctx context.Context, itemType OutboxItemType, timeoutSeconds int, limit int) ([]*OutboxItem, error) {
	collection := r.getCollection(itemType)

	// Calculate cutoff time
	cutoff := time.Now().Add(-time.Duration(timeoutSeconds) * time.Second)

	// Filter for error statuses older than timeout
	filter := bson.M{
		"status": bson.M{
			"$in": []int{
				int(StatusInProgress),
				int(StatusBadRequest),
				int(StatusInternalError),
				int(StatusUnauthorized),
				int(StatusForbidden),
				int(StatusGatewayError),
			},
		},
		"updatedAt": bson.M{"$lt": cutoff},
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "createdAt", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("fetch recoverable items: %w", err)
	}
	defer cursor.Close(ctx)

	return r.decodeCursor(ctx, cursor)
}

// ResetRecoverableItems resets recoverable items back to PENDING status.
func (r *MongoRepository) ResetRecoverableItems(ctx context.Context, itemType OutboxItemType, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	collection := r.getCollection(itemType)
	filter := bson.M{"_id": bson.M{"$in": ids}}
	update := bson.M{
		"$set": bson.M{
			"status":    int(StatusPending),
			"updatedAt": time.Now(),
		},
	}

	_, err := collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return fmt.Errorf("reset recoverable items: %w", err)
	}
	return nil
}

// CountPending returns the count of pending items.
func (r *MongoRepository) CountPending(ctx context.Context, itemType OutboxItemType) (int64, error) {
	collection := r.getCollection(itemType)
	filter := bson.M{"status": int(StatusPending)}

	count, err := collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, fmt.Errorf("count pending: %w", err)
	}
	return count, nil
}

// CreateSchema creates indexes on the outbox collections.
// MongoDB collections are created implicitly, so we only need to create indexes.
func (r *MongoRepository) CreateSchema(ctx context.Context) error {
	for _, itemType := range []OutboxItemType{OutboxItemTypeEvent, OutboxItemTypeDispatchJob} {
		collection := r.getCollection(itemType)
		collName := r.GetTableName(itemType)

		// Index for fetching pending items (status=0, ordered by messageGroup, createdAt)
		// MongoDB supports partial indexes via partialFilterExpression
		_, err := collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "messageGroup", Value: 1},
				{Key: "createdAt", Value: 1},
			},
			Options: options.Index().
				SetName("idx_pending").
				SetPartialFilterExpression(bson.M{"status": int(StatusPending)}),
		})
		if err != nil {
			return fmt.Errorf("create pending index on %s: %w", collName, err)
		}

		// Index for finding stuck items (status=9)
		_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
			Keys: bson.D{
				{Key: "status", Value: 1},
				{Key: "createdAt", Value: 1},
			},
			Options: options.Index().
				SetName("idx_stuck").
				SetPartialFilterExpression(bson.M{"status": int(StatusInProgress)}),
		})
		if err != nil {
			return fmt.Errorf("create stuck index on %s: %w", collName, err)
		}
	}
	return nil
}

// decodeCursor decodes MongoDB cursor into OutboxItem slice
func (r *MongoRepository) decodeCursor(ctx context.Context, cursor *mongo.Cursor) ([]*OutboxItem, error) {
	var items []*OutboxItem
	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode document: %w", err)
		}

		item := &OutboxItem{}

		// _id
		if id, ok := doc["_id"].(string); ok {
			item.ID = id
		}

		// type
		if t, ok := doc["type"].(string); ok {
			item.Type = OutboxItemType(t)
		}

		// messageGroup
		if mg, ok := doc["messageGroup"].(string); ok {
			item.MessageGroup = mg
		}

		// payload
		if p, ok := doc["payload"].(string); ok {
			item.Payload = p
		}

		// status (stored as int32 in MongoDB)
		if s, ok := doc["status"].(int32); ok {
			item.Status = OutboxStatus(s)
		} else if s, ok := doc["status"].(int64); ok {
			item.Status = OutboxStatus(s)
		} else if s, ok := doc["status"].(int); ok {
			item.Status = OutboxStatus(s)
		}

		// retryCount
		if rc, ok := doc["retryCount"].(int32); ok {
			item.RetryCount = int(rc)
		} else if rc, ok := doc["retryCount"].(int64); ok {
			item.RetryCount = int(rc)
		} else if rc, ok := doc["retryCount"].(int); ok {
			item.RetryCount = rc
		}

		// createdAt
		if ca, ok := doc["createdAt"].(time.Time); ok {
			item.CreatedAt = ca
		}

		// updatedAt
		if ua, ok := doc["updatedAt"].(time.Time); ok {
			item.UpdatedAt = ua
		}

		// errorMessage
		if em, ok := doc["errorMessage"].(string); ok {
			item.ErrorMessage = em
		}

		items = append(items, item)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor iteration: %w", err)
	}

	return items, nil
}
