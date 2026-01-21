// Package leader provides distributed leader election using MongoDB
package leader

import (
	"context"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LeaderLock represents a distributed lock document in MongoDB
type LeaderLock struct {
	ID         string    `bson:"_id"`        // Lock name (e.g., "scheduler-leader")
	InstanceID string    `bson:"instanceId"` // Unique instance identifier
	AcquiredAt time.Time `bson:"acquiredAt"` // When lock was acquired
	ExpiresAt  time.Time `bson:"expiresAt"`  // When lock expires
}

// ElectorConfig holds configuration for leader election
type ElectorConfig struct {
	// InstanceID uniquely identifies this instance (defaults to hostname)
	InstanceID string

	// LockName is the name of the lock to acquire (e.g., "scheduler-leader")
	LockName string

	// TTL is how long the lock is valid before expiring (default: 30s)
	TTL time.Duration

	// RefreshInterval is how often to refresh the lock while primary (default: 10s)
	RefreshInterval time.Duration
}

// DefaultElectorConfig returns sensible defaults
func DefaultElectorConfig(lockName string) *ElectorConfig {
	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "instance-" + time.Now().Format("20060102150405")
	}

	return &ElectorConfig{
		InstanceID:      instanceID,
		LockName:        lockName,
		TTL:             30 * time.Second,
		RefreshInterval: 10 * time.Second,
	}
}

// LeaderElector provides distributed leader election using MongoDB
type LeaderElector struct {
	collection      *mongo.Collection
	config          *ElectorConfig
	isPrimary       atomic.Bool
	ctx             context.Context
	cancel          context.CancelFunc
	refreshStopped  chan struct{}
	onBecomeLeader  func()
	onLoseLeadership func()
}

// NewLeaderElector creates a new leader elector
func NewLeaderElector(db *mongo.Database, config *ElectorConfig) *LeaderElector {
	if config == nil {
		config = DefaultElectorConfig("default-leader")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &LeaderElector{
		collection:     db.Collection("leader_locks"),
		config:         config,
		ctx:            ctx,
		cancel:         cancel,
		refreshStopped: make(chan struct{}),
	}
}

// OnBecomeLeader sets a callback for when this instance becomes leader
func (e *LeaderElector) OnBecomeLeader(fn func()) {
	e.onBecomeLeader = fn
}

// OnLoseLeadership sets a callback for when this instance loses leadership
func (e *LeaderElector) OnLoseLeadership(fn func()) {
	e.onLoseLeadership = fn
}

// Start begins the leader election process
// It attempts to acquire the lock and maintains it while primary
func (e *LeaderElector) Start(ctx context.Context) error {
	// Create TTL index on expiresAt if it doesn't exist
	// MongoDB will automatically remove expired documents
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "expiresAt", Value: 1}},
		Options: options.Index().
			SetExpireAfterSeconds(0). // Expire at the expiresAt time
			SetName("ttl_expiresAt"),
	}

	_, err := e.collection.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		// Index may already exist, log and continue
		slog.Debug("Could not create TTL index (may already exist)", "error", err)
	}

	// Start the election loop
	go e.electionLoop()

	slog.Info("Leader election started",
		"instanceId", e.config.InstanceID,
		"lockName", e.config.LockName,
		"ttl", e.config.TTL,
		"refreshInterval", e.config.RefreshInterval)

	return nil
}

// Stop stops the leader election and releases the lock if held
func (e *LeaderElector) Stop() {
	e.cancel()

	// Wait for refresh loop to stop
	<-e.refreshStopped

	// Release the lock if we hold it
	if e.isPrimary.Load() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.Release(ctx)
	}

	slog.Info("Leader election stopped", "instanceId", e.config.InstanceID)
}

// IsPrimary returns true if this instance is currently the leader
func (e *LeaderElector) IsPrimary() bool {
	return e.isPrimary.Load()
}

// InstanceID returns the instance ID of this elector
func (e *LeaderElector) InstanceID() string {
	return e.config.InstanceID
}

// electionLoop continuously attempts to acquire/refresh the lock
func (e *LeaderElector) electionLoop() {
	defer close(e.refreshStopped)

	ticker := time.NewTicker(e.config.RefreshInterval)
	defer ticker.Stop()

	// Try to acquire immediately
	e.tryAcquireOrRefresh()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.tryAcquireOrRefresh()
		}
	}
}

// tryAcquireOrRefresh tries to acquire the lock or refresh it if already held
func (e *LeaderElector) tryAcquireOrRefresh() {
	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Second)
	defer cancel()

	wasPrimary := e.isPrimary.Load()

	if wasPrimary {
		// Try to refresh
		if e.refresh(ctx) {
			return
		}
		// Refresh failed, we lost leadership
		e.isPrimary.Store(false)
		slog.Warn("Lost leadership - refresh failed",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
		if e.onLoseLeadership != nil {
			e.onLoseLeadership()
		}
	}

	// Try to acquire
	if e.tryAcquire(ctx) {
		if !wasPrimary {
			slog.Info("Acquired leadership",
				"instanceId", e.config.InstanceID,
				"lockName", e.config.LockName)
			if e.onBecomeLeader != nil {
				e.onBecomeLeader()
			}
		}
		e.isPrimary.Store(true)
	}
}

// tryAcquire attempts to acquire the lock
// Returns true if the lock was acquired
func (e *LeaderElector) tryAcquire(ctx context.Context) bool {
	now := time.Now()
	expiresAt := now.Add(e.config.TTL)

	// Use findOneAndUpdate with upsert to atomically acquire
	// Only acquire if:
	// 1. Lock doesn't exist, OR
	// 2. Lock is expired, OR
	// 3. We already own it (refresh case)
	filter := bson.M{
		"_id": e.config.LockName,
		"$or": []bson.M{
			{"expiresAt": bson.M{"$lt": now}},
			{"instanceId": e.config.InstanceID},
		},
	}

	update := bson.M{
		"$set": bson.M{
			"instanceId": e.config.InstanceID,
			"acquiredAt": now,
			"expiresAt":  expiresAt,
		},
	}

	opts := options.FindOneAndUpdate().
		SetUpsert(true).
		SetReturnDocument(options.After)

	var result LeaderLock
	err := e.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result)

	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			// Another instance beat us to it
			slog.Debug("Lock already held by another instance",
				"instanceId", e.config.InstanceID,
				"lockName", e.config.LockName)
			return false
		}

		// Try simple insert for new lock
		if err == mongo.ErrNoDocuments {
			lock := LeaderLock{
				ID:         e.config.LockName,
				InstanceID: e.config.InstanceID,
				AcquiredAt: now,
				ExpiresAt:  expiresAt,
			}
			_, insertErr := e.collection.InsertOne(ctx, lock)
			if insertErr != nil {
				if !mongo.IsDuplicateKeyError(insertErr) {
					slog.Error("Failed to insert leader lock", "error", insertErr)
				}
				return false
			}
			return true
		}

		slog.Error("Failed to acquire leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return false
	}

	// Check if we actually own the lock
	return result.InstanceID == e.config.InstanceID
}

// refresh updates the expiration time on a lock we hold
// Returns true if the refresh succeeded
func (e *LeaderElector) refresh(ctx context.Context) bool {
	now := time.Now()
	expiresAt := now.Add(e.config.TTL)

	filter := bson.M{
		"_id":        e.config.LockName,
		"instanceId": e.config.InstanceID,
	}

	update := bson.M{
		"$set": bson.M{
			"expiresAt": expiresAt,
		},
	}

	result, err := e.collection.UpdateOne(ctx, filter, update)
	if err != nil {
		slog.Error("Failed to refresh leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return false
	}

	if result.MatchedCount == 0 {
		// We don't hold the lock anymore
		slog.Debug("Lock no longer held by this instance",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
		return false
	}

	slog.Debug("Refreshed leader lock",
		"instanceId", e.config.InstanceID,
		"lockName", e.config.LockName,
		"expiresAt", expiresAt)

	return true
}

// Release explicitly releases the lock
func (e *LeaderElector) Release(ctx context.Context) {
	filter := bson.M{
		"_id":        e.config.LockName,
		"instanceId": e.config.InstanceID,
	}

	result, err := e.collection.DeleteOne(ctx, filter)
	if err != nil {
		slog.Error("Failed to release leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return
	}

	if result.DeletedCount > 0 {
		slog.Info("Released leader lock",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
	}

	e.isPrimary.Store(false)
}

// GetCurrentLeader returns the instance ID of the current leader
// Returns empty string if no leader
func (e *LeaderElector) GetCurrentLeader(ctx context.Context) (string, error) {
	filter := bson.M{
		"_id":       e.config.LockName,
		"expiresAt": bson.M{"$gt": time.Now()},
	}

	var lock LeaderLock
	err := e.collection.FindOne(ctx, filter).Decode(&lock)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", nil
		}
		return "", err
	}

	return lock.InstanceID, nil
}
