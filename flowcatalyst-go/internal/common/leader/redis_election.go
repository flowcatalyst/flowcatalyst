// Package leader provides distributed leader election
package leader

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisElectorConfig holds configuration for Redis-based leader election
type RedisElectorConfig struct {
	// InstanceID uniquely identifies this instance (defaults to hostname)
	InstanceID string

	// LockName is the name of the lock to acquire (e.g., "outbox-processor-leader")
	LockName string

	// TTL is how long the lock is valid before expiring (default: 30s)
	TTL time.Duration

	// RefreshInterval is how often to refresh the lock while primary (default: 10s)
	RefreshInterval time.Duration
}

// DefaultRedisElectorConfig returns sensible defaults
func DefaultRedisElectorConfig(lockName string) *RedisElectorConfig {
	instanceID, _ := os.Hostname()
	if instanceID == "" {
		instanceID = "instance-" + time.Now().Format("20060102150405")
	}

	return &RedisElectorConfig{
		InstanceID:      instanceID,
		LockName:        lockName,
		TTL:             30 * time.Second,
		RefreshInterval: 10 * time.Second,
	}
}

// RedisLeaderElector provides distributed leader election using Redis
// This matches Java's Redisson-based approach for multi-instance deployments.
//
// The lock uses SET NX EX pattern for atomic lock acquisition:
//   - SET lockName instanceId NX EX ttlSeconds
//
// If the lock is acquired, the elector becomes primary and refreshes the lock
// periodically to maintain leadership.
type RedisLeaderElector struct {
	client          *redis.Client
	config          *RedisElectorConfig
	isPrimary       atomic.Bool
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	onBecomeLeader  func()
	onLoseLeadership func()
}

// NewRedisLeaderElector creates a new Redis-based leader elector
func NewRedisLeaderElector(client *redis.Client, config *RedisElectorConfig) *RedisLeaderElector {
	if config == nil {
		config = DefaultRedisElectorConfig("default-leader")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &RedisLeaderElector{
		client: client,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// OnBecomeLeader sets a callback for when this instance becomes leader
func (e *RedisLeaderElector) OnBecomeLeader(fn func()) {
	e.onBecomeLeader = fn
}

// OnLoseLeadership sets a callback for when this instance loses leadership
func (e *RedisLeaderElector) OnLoseLeadership(fn func()) {
	e.onLoseLeadership = fn
}

// Start begins the leader election process
func (e *RedisLeaderElector) Start(ctx context.Context) error {
	e.wg.Add(1)
	go e.electionLoop()

	slog.Info("Redis leader election started",
		"instanceId", e.config.InstanceID,
		"lockName", e.config.LockName,
		"ttl", e.config.TTL,
		"refreshInterval", e.config.RefreshInterval)

	return nil
}

// Stop stops the leader election and releases the lock if held
func (e *RedisLeaderElector) Stop() {
	e.cancel()
	e.wg.Wait()

	// Release the lock if we hold it
	if e.isPrimary.Load() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		e.Release(ctx)
	}

	slog.Info("Redis leader election stopped", "instanceId", e.config.InstanceID)
}

// IsPrimary returns true if this instance is currently the leader
func (e *RedisLeaderElector) IsPrimary() bool {
	return e.isPrimary.Load()
}

// InstanceID returns the instance ID of this elector
func (e *RedisLeaderElector) InstanceID() string {
	return e.config.InstanceID
}

// electionLoop continuously attempts to acquire/refresh the lock
func (e *RedisLeaderElector) electionLoop() {
	defer e.wg.Done()

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
func (e *RedisLeaderElector) tryAcquireOrRefresh() {
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

// tryAcquire attempts to acquire the lock using SET NX EX
// Returns true if the lock was acquired
func (e *RedisLeaderElector) tryAcquire(ctx context.Context) bool {
	// SET lockName instanceId NX EX ttlSeconds
	// NX = only set if not exists
	// EX = expire time in seconds
	ttlSeconds := int(e.config.TTL.Seconds())
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	success, err := e.client.SetNX(ctx, e.config.LockName, e.config.InstanceID, e.config.TTL).Result()
	if err != nil {
		slog.Error("Failed to acquire Redis leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return false
	}

	if success {
		slog.Debug("Acquired Redis leader lock",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
		return true
	}

	// Lock exists - check if we own it (could be our old lock after restart)
	owner, err := e.client.Get(ctx, e.config.LockName).Result()
	if err != nil {
		if err != redis.Nil {
			slog.Error("Failed to check lock owner", "error", err)
		}
		return false
	}

	if owner == e.config.InstanceID {
		// We already own it, refresh it
		return e.refresh(ctx)
	}

	slog.Debug("Lock held by another instance",
		"instanceId", e.config.InstanceID,
		"owner", owner,
		"lockName", e.config.LockName)

	return false
}

// refresh updates the expiration time on a lock we hold
// Uses a Lua script to atomically check ownership and extend TTL
// Returns true if the refresh succeeded
func (e *RedisLeaderElector) refresh(ctx context.Context) bool {
	// Lua script for atomic check-and-extend
	// Only extend if we still own the lock
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("expire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	ttlSeconds := int(e.config.TTL.Seconds())
	if ttlSeconds < 1 {
		ttlSeconds = 1
	}

	result, err := script.Run(ctx, e.client, []string{e.config.LockName}, e.config.InstanceID, ttlSeconds).Int()
	if err != nil {
		slog.Error("Failed to refresh Redis leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return false
	}

	if result == 0 {
		slog.Debug("Lock no longer held by this instance",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
		return false
	}

	slog.Debug("Refreshed Redis leader lock",
		"instanceId", e.config.InstanceID,
		"lockName", e.config.LockName,
		"ttlSeconds", ttlSeconds)

	return true
}

// Release explicitly releases the lock
// Uses a Lua script to atomically check ownership before deleting
func (e *RedisLeaderElector) Release(ctx context.Context) {
	// Lua script for atomic check-and-delete
	// Only delete if we own the lock
	script := redis.NewScript(`
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, e.client, []string{e.config.LockName}, e.config.InstanceID).Int()
	if err != nil {
		slog.Error("Failed to release Redis leader lock",
			"error", err,
			"lockName", e.config.LockName)
		return
	}

	if result > 0 {
		slog.Info("Released Redis leader lock",
			"instanceId", e.config.InstanceID,
			"lockName", e.config.LockName)
	}

	e.isPrimary.Store(false)
}

// GetCurrentLeader returns the instance ID of the current leader
// Returns empty string if no leader
func (e *RedisLeaderElector) GetCurrentLeader(ctx context.Context) (string, error) {
	owner, err := e.client.Get(ctx, e.config.LockName).Result()
	if err != nil {
		if err == redis.Nil {
			return "", nil
		}
		return "", err
	}
	return owner, nil
}
