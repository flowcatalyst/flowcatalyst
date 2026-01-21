package standby

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisLockProvider implements distributed locking using Redis
type RedisLockProvider struct {
	client *redis.Client
}

// NewRedisLockProvider creates a new Redis-based lock provider
func NewRedisLockProvider(redisURL string) (*RedisLockProvider, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}

	client := redis.NewClient(opts)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return nil, err
	}

	slog.Info("Connected to Redis for distributed locking", "url", redisURL)

	return &RedisLockProvider{
		client: client,
	}, nil
}

// TryAcquire attempts to acquire the lock using SET NX (set if not exists)
func (p *RedisLockProvider) TryAcquire(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error) {
	// SET key instanceID NX EX ttl
	ok, err := p.client.SetNX(ctx, key, instanceID, ttl).Result()
	if err != nil {
		return false, err
	}

	if ok {
		slog.Debug("Lock acquired",
			"key", key,
			"instanceId", instanceID,
			"ttl", ttl)
	}

	return ok, nil
}

// Refresh extends the lock TTL if we're still the owner
// Uses a Lua script to atomically check ownership and extend TTL
func (p *RedisLockProvider) Refresh(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error) {
	// Lua script: if lock is held by us, extend TTL; return 1 if successful
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("PEXPIRE", KEYS[1], ARGV[2])
		else
			return 0
		end
	`)

	result, err := script.Run(ctx, p.client, []string{key}, instanceID, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}

	return result == 1, nil
}

// Release releases the lock if we're the owner
// Uses a Lua script to atomically check ownership and delete
func (p *RedisLockProvider) Release(ctx context.Context, key, instanceID string) error {
	// Lua script: if lock is held by us, delete it
	script := redis.NewScript(`
		if redis.call("GET", KEYS[1]) == ARGV[1] then
			return redis.call("DEL", KEYS[1])
		else
			return 0
		end
	`)

	_, err := script.Run(ctx, p.client, []string{key}, instanceID).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return err
	}

	slog.Debug("Lock released",
		"key", key,
		"instanceId", instanceID)

	return nil
}

// GetHolder returns the current lock holder instance ID
func (p *RedisLockProvider) GetHolder(ctx context.Context, key string) (string, error) {
	holder, err := p.client.Get(ctx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return "", nil // No holder
		}
		return "", err
	}
	return holder, nil
}

// IsAvailable checks if Redis is reachable
func (p *RedisLockProvider) IsAvailable(ctx context.Context) bool {
	err := p.client.Ping(ctx).Err()
	return err == nil
}

// Close closes the Redis connection
func (p *RedisLockProvider) Close() error {
	if p.client != nil {
		return p.client.Close()
	}
	return nil
}
