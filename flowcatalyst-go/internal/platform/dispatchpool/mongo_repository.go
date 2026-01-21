package dispatchpool

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
	ErrNotFound      = errors.New("dispatch pool not found")
	ErrDuplicateCode = errors.New("dispatch pool code already exists")
)

// mongoRepository provides MongoDB access to dispatch pool data
type mongoRepository struct {
	pools *mongo.Collection
}

// NewRepository creates a new dispatch pool repository with instrumentation
func NewRepository(db *mongo.Database) Repository {
	return newInstrumentedRepository(&mongoRepository{
		pools: db.Collection("dispatch_pools"),
	})
}

// FindByID finds a dispatch pool by ID
func (r *mongoRepository) FindByID(ctx context.Context, id string) (*DispatchPool, error) {
	var pool DispatchPool
	err := r.pools.FindOne(ctx, bson.M{"_id": id}).Decode(&pool)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &pool, nil
}

// FindByCode finds a dispatch pool by code
func (r *mongoRepository) FindByCode(ctx context.Context, code string) (*DispatchPool, error) {
	var pool DispatchPool
	err := r.pools.FindOne(ctx, bson.M{"code": code}).Decode(&pool)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &pool, nil
}

// FindAll finds all dispatch pools
func (r *mongoRepository) FindAll(ctx context.Context) ([]*DispatchPool, error) {
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindAllEnabled finds all enabled dispatch pools
// Deprecated: Use FindAllActive instead
func (r *mongoRepository) FindAllEnabled(ctx context.Context) ([]*DispatchPool, error) {
	// Support both old 'enabled' field and new 'status' field for backwards compatibility
	filter := bson.M{
		"$or": []bson.M{
			{"status": DispatchPoolStatusActive},
			{"enabled": true, "status": bson.M{"$exists": false}},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindAllActive finds all active dispatch pools
func (r *mongoRepository) FindAllActive(ctx context.Context) ([]*DispatchPool, error) {
	filter := bson.M{"status": DispatchPoolStatusActive}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindByStatus finds pools by status
func (r *mongoRepository) FindByStatus(ctx context.Context, status DispatchPoolStatus) ([]*DispatchPool, error) {
	filter := bson.M{"status": status}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindAnchorLevel finds anchor-level pools (not bound to any client)
func (r *mongoRepository) FindAnchorLevel(ctx context.Context) ([]*DispatchPool, error) {
	filter := bson.M{
		"$or": []bson.M{
			{"clientId": nil},
			{"clientId": ""},
			{"clientId": bson.M{"$exists": false}},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindAllNonArchived finds all pools that are not archived
func (r *mongoRepository) FindAllNonArchived(ctx context.Context) ([]*DispatchPool, error) {
	filter := bson.M{"status": bson.M{"$ne": DispatchPoolStatusArchived}}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// FindByClientID finds dispatch pools for a specific client
func (r *mongoRepository) FindByClientID(ctx context.Context, clientID string) ([]*DispatchPool, error) {
	filter := bson.M{"clientId": clientID}
	opts := options.Find().SetSort(bson.D{{Key: "code", Value: 1}})

	cursor, err := r.pools.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var pools []*DispatchPool
	if err := cursor.All(ctx, &pools); err != nil {
		return nil, err
	}
	return pools, nil
}

// Insert creates a new dispatch pool
func (r *mongoRepository) Insert(ctx context.Context, pool *DispatchPool) error {
	if pool.ID == "" {
		pool.ID = tsid.Generate()
	}
	now := time.Now()
	pool.CreatedAt = now
	pool.UpdatedAt = now

	_, err := r.pools.InsertOne(ctx, pool)
	if mongo.IsDuplicateKeyError(err) {
		return ErrDuplicateCode
	}
	return err
}

// Update updates an existing dispatch pool
func (r *mongoRepository) Update(ctx context.Context, pool *DispatchPool) error {
	pool.UpdatedAt = time.Now()

	result, err := r.pools.ReplaceOne(ctx, bson.M{"_id": pool.ID}, pool)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateConfig updates pool configuration fields
func (r *mongoRepository) UpdateConfig(ctx context.Context, id string, concurrency, queueCapacity int, rateLimitPerMin *int) error {
	update := bson.M{
		"$set": bson.M{
			"concurrency":     concurrency,
			"queueCapacity":   queueCapacity,
			"rateLimitPerMin": rateLimitPerMin,
			"updatedAt":       time.Now(),
		},
	}

	result, err := r.pools.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// SetEnabled enables or disables a dispatch pool
// Deprecated: Use SetStatus instead
func (r *mongoRepository) SetEnabled(ctx context.Context, id string, enabled bool) error {
	status := DispatchPoolStatusSuspended
	if enabled {
		status = DispatchPoolStatusActive
	}
	return r.SetStatus(ctx, id, status)
}

// SetStatus updates pool status
func (r *mongoRepository) SetStatus(ctx context.Context, id string, status DispatchPoolStatus) error {
	update := bson.M{
		"$set": bson.M{
			"status":    status,
			"enabled":   status == DispatchPoolStatusActive, // Keep enabled field in sync for backwards compatibility
			"updatedAt": time.Now(),
		},
	}

	result, err := r.pools.UpdateOne(ctx, bson.M{"_id": id}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a dispatch pool
func (r *mongoRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pools.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return ErrNotFound
	}
	return nil
}

// Count returns the total number of dispatch pools
func (r *mongoRepository) Count(ctx context.Context) (int64, error) {
	return r.pools.CountDocuments(ctx, bson.M{})
}

// CountEnabled returns the number of enabled dispatch pools
// Deprecated: Use CountActive instead
func (r *mongoRepository) CountEnabled(ctx context.Context) (int64, error) {
	return r.CountActive(ctx)
}

// CountActive returns the number of active dispatch pools
func (r *mongoRepository) CountActive(ctx context.Context) (int64, error) {
	return r.pools.CountDocuments(ctx, bson.M{"status": DispatchPoolStatusActive})
}

// CountByStatus returns the number of pools with a specific status
func (r *mongoRepository) CountByStatus(ctx context.Context, status DispatchPoolStatus) (int64, error) {
	return r.pools.CountDocuments(ctx, bson.M{"status": status})
}

// ExistsByCode checks if a pool with the given code exists
func (r *mongoRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	count, err := r.pools.CountDocuments(ctx, bson.M{"code": code})
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
