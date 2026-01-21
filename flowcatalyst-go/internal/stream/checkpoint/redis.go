package checkpoint

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/bson"
)

// RedisStore stores checkpoints in Redis.
// Resume tokens are stored as binary data with optional TTL.
type RedisStore struct {
	client *redis.Client
	prefix string
	ttl    time.Duration
}

// RedisConfig holds configuration for the Redis checkpoint store
type RedisConfig struct {
	// Addr is the Redis server address (e.g., "localhost:6379")
	Addr string

	// Password is the Redis password (optional)
	Password string

	// DB is the Redis database number
	DB int

	// Prefix is the key prefix for all checkpoints (default: "flowcatalyst:checkpoint:")
	Prefix string

	// TTL is the time-to-live for checkpoint keys (0 = no expiration)
	TTL time.Duration
}

// NewRedisStore creates a new Redis checkpoint store
func NewRedisStore(cfg *RedisConfig) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Verify connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	prefix := cfg.Prefix
	if prefix == "" {
		prefix = "flowcatalyst:checkpoint:"
	}

	return &RedisStore{
		client: client,
		prefix: prefix,
		ttl:    cfg.TTL,
	}, nil
}

// NewRedisStoreFromClient creates a new Redis checkpoint store from an existing client
func NewRedisStoreFromClient(client *redis.Client, prefix string, ttl time.Duration) *RedisStore {
	if prefix == "" {
		prefix = "flowcatalyst:checkpoint:"
	}

	return &RedisStore{
		client: client,
		prefix: prefix,
		ttl:    ttl,
	}
}

// GetCheckpoint retrieves a checkpoint (resume token)
func (s *RedisStore) GetCheckpoint(key string) (bson.Raw, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := s.prefix + key

	data, err := s.client.Get(ctx, redisKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get checkpoint: %w", err)
	}

	if len(data) == 0 {
		return nil, nil
	}

	return bson.Raw(data), nil
}

// SaveCheckpoint saves a checkpoint
func (s *RedisStore) SaveCheckpoint(key string, token bson.Raw) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := s.prefix + key

	var err error
	if s.ttl > 0 {
		err = s.client.Set(ctx, redisKey, []byte(token), s.ttl).Err()
	} else {
		err = s.client.Set(ctx, redisKey, []byte(token), 0).Err()
	}

	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	return nil
}

// Delete removes a specific checkpoint
func (s *RedisStore) Delete(key string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	redisKey := s.prefix + key
	return s.client.Del(ctx, redisKey).Err()
}

// Close closes the Redis connection
func (s *RedisStore) Close() error {
	return s.client.Close()
}
