package audit

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"go.flowcatalyst.tech/internal/common/tsid"
)

const collectionName = "audit_logs"

var (
	ErrNotFound = errors.New("audit log not found")
)

// Repository provides access to audit log data
type Repository struct {
	collection *mongo.Collection
}

// NewRepository creates a new audit log repository
func NewRepository(db *mongo.Database) *Repository {
	return &Repository{
		collection: db.Collection(collectionName),
	}
}

// Insert creates a new audit log entry
func (r *Repository) Insert(ctx context.Context, log *AuditLog) error {
	if log.ID == "" {
		log.ID = tsid.Generate()
	}
	if log.PerformedAt.IsZero() {
		log.PerformedAt = time.Now()
	}

	_, err := r.collection.InsertOne(ctx, log)
	return err
}

// FindByID finds an audit log by ID
func (r *Repository) FindByID(ctx context.Context, id string) (*AuditLog, error) {
	var log AuditLog
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&log)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &log, nil
}

// FindByEntity finds audit logs for a specific entity
func (r *Repository) FindByEntity(ctx context.Context, entityType, entityID string) ([]*AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "performedAt", Value: -1}})

	filter := bson.M{"entityType": entityType, "entityId": entityID}
	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindByPrincipal finds audit logs by principal ID
func (r *Repository) FindByPrincipal(ctx context.Context, principalID string) ([]*AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "performedAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"principalId": principalID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindByTimeRange finds audit logs within a time range
func (r *Repository) FindByTimeRange(ctx context.Context, from, to time.Time) ([]*AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "performedAt", Value: -1}})

	filter := bson.M{
		"performedAt": bson.M{
			"$gte": from,
			"$lte": to,
		},
	}

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindByOperation finds audit logs for a specific operation type
func (r *Repository) FindByOperation(ctx context.Context, operation string) ([]*AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "performedAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"operation": operation}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindPaged returns audit logs with pagination
func (r *Repository) FindPaged(ctx context.Context, page, pageSize int) ([]*AuditLog, error) {
	skip := int64(page * pageSize)
	limit := int64(pageSize)

	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "performedAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindByEntityType finds audit logs for a specific entity type
func (r *Repository) FindByEntityType(ctx context.Context, entityType string) ([]*AuditLog, error) {
	opts := options.Find().SetSort(bson.D{{Key: "performedAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"entityType": entityType}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// FindByEntityTypePaged finds audit logs for a specific entity type with pagination
func (r *Repository) FindByEntityTypePaged(ctx context.Context, entityType string, page, pageSize int) ([]*AuditLog, error) {
	skip := int64(page * pageSize)
	limit := int64(pageSize)

	opts := options.Find().
		SetSkip(skip).
		SetLimit(limit).
		SetSort(bson.D{{Key: "performedAt", Value: -1}})

	cursor, err := r.collection.Find(ctx, bson.M{"entityType": entityType}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*AuditLog
	if err := cursor.All(ctx, &logs); err != nil {
		return nil, err
	}
	return logs, nil
}

// Count returns the total number of audit logs
func (r *Repository) Count(ctx context.Context) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{})
}

// CountByEntityType returns the count of audit logs by entity type
func (r *Repository) CountByEntityType(ctx context.Context, entityType string) (int64, error) {
	return r.collection.CountDocuments(ctx, bson.M{"entityType": entityType})
}

// GetDistinctEntityTypes returns distinct entity types that have audit logs
func (r *Repository) GetDistinctEntityTypes(ctx context.Context) ([]string, error) {
	results, err := r.collection.Distinct(ctx, "entityType", bson.M{})
	if err != nil {
		return nil, err
	}

	entityTypes := make([]string, 0, len(results))
	for _, v := range results {
		if s, ok := v.(string); ok {
			entityTypes = append(entityTypes, s)
		}
	}
	return entityTypes, nil
}

// GetDistinctOperations returns distinct operations that have audit logs
func (r *Repository) GetDistinctOperations(ctx context.Context) ([]string, error) {
	results, err := r.collection.Distinct(ctx, "operation", bson.M{})
	if err != nil {
		return nil, err
	}

	operations := make([]string, 0, len(results))
	for _, v := range results {
		if s, ok := v.(string); ok {
			operations = append(operations, s)
		}
	}
	return operations, nil
}

// EnsureIndexes creates indexes for audit log queries
func (r *Repository) EnsureIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{
				{Key: "entityType", Value: 1},
				{Key: "entityId", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "principalId", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "performedAt", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "operation", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "entityType", Value: 1}},
		},
	}

	_, err := r.collection.Indexes().CreateMany(ctx, indexes)
	return err
}
