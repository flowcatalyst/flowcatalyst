package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisConnectTimeout = 2 * time.Second

// RedisStore is a fixed-window counter over Redis: INCR a per-window key,
// EXPIRE it to the window length on the 0→1 transition. The key ages out
// via TTL — no reaper needed. A fresh window starts at floor(now/window),
// so the worst case is a 2× spike at the window boundary, acceptable for a
// cluster-wide ceiling.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore connects, PINGs to confirm liveness, and returns the
// store. Returns an error (never panics) so Build can fall back to
// Postgres.
func NewRedisStore(ctx context.Context, url string) (*RedisStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("invalid redis url: %w", err)
	}
	client := redis.NewClient(opts)

	pingCtx, cancel := context.WithTimeout(ctx, redisConnectTimeout)
	defer cancel()
	if pong, err := client.Ping(pingCtx).Result(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping: %w", err)
	} else if pong != "PONG" {
		_ = client.Close()
		return nil, fmt.Errorf("unexpected redis PING response: %s", pong)
	}
	return &RedisStore{client: client}, nil
}

func redisKey(bucket Bucket, key string, windowIndex int64) string {
	return "fc:rl:" + string(bucket) + ":" + key + ":" + strconv.FormatInt(windowIndex, 10)
}

// CheckAndRecord increments the current window's counter and rejects when
// it exceeds the limit.
func (s *RedisStore) CheckAndRecord(ctx context.Context, bucket Bucket, key string, policy Policy) (Decision, error) {
	windowSecs := int64(policy.Window.Seconds())
	if windowSecs < 1 {
		windowSecs = 1
	}
	nowSecs := time.Now().Unix()
	windowIndex := nowSecs / windowSecs
	rk := redisKey(bucket, key, windowIndex)

	newCount, err := s.client.Incr(ctx, rk).Result()
	if err != nil {
		return Decision{}, fmt.Errorf("redis incr: %w", err)
	}
	if newCount == 1 {
		// Best-effort TTL; a miss just lets the next call's EXPIRE catch it.
		if err := s.client.Expire(ctx, rk, time.Duration(windowSecs)*time.Second).Err(); err != nil {
			slog.Warn("redis EXPIRE failed; TTL may be missing on this window", "key", rk, "err", err)
		}
	}

	if uint64(newCount) > uint64(policy.Limit) {
		elapsedInWindow := nowSecs % windowSecs
		retryAfter := windowSecs - elapsedInWindow
		if retryAfter < 1 {
			retryAfter = 1
		}
		return Decision{Allowed: false, RetryAfterSecs: clampU32(retryAfter)}, nil
	}
	return Decision{Allowed: true}, nil
}

// Prune is a no-op for Redis (TTLs auto-expire fixed-window keys).
func (s *RedisStore) Prune(context.Context, time.Duration) (int64, error) { return 0, nil }

func clampU32(v int64) uint32 {
	if v < 0 {
		return 0
	}
	if v > int64(^uint32(0)) {
		return ^uint32(0)
	}
	return uint32(v)
}
