// Package standby implements Redis-backed leader election. Mirrors the
// Rust fc-standby crate.
//
// One process acquires the lock via SET NX EX; while it holds the
// lock, it periodically refreshes the TTL. If the leader crashes or
// is partitioned, the lock expires and another instance acquires it.
//
// Consumers query IsLeader() (atomic, lock-free) or subscribe to a
// channel of LeadershipChange events.
package standby

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// LeadershipChange is emitted on transitions.
type LeadershipChange struct {
	IsLeader bool
	At       time.Time
}

// Election is a single instance of the leader-election state machine.
type Election struct {
	cfg    common.LeaderElectionConfig
	client *redis.Client

	isLeader atomic.Bool
	stopOnce sync.Once
	stopCh   chan struct{}
	doneCh   chan struct{}

	subsMu sync.RWMutex
	subs   []chan LeadershipChange
}

// New constructs an Election. The caller is responsible for calling
// Start to spawn the heartbeat goroutine and Stop on shutdown.
func New(cfg common.LeaderElectionConfig) (*Election, error) {
	opts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	return &Election{
		cfg:    cfg,
		client: redis.NewClient(opts),
		stopCh: make(chan struct{}),
		doneCh: make(chan struct{}),
	}, nil
}

// IsLeader reports whether this instance currently holds the lock.
// Safe to call from any goroutine.
func (e *Election) IsLeader() bool { return e.isLeader.Load() }

// Subscribe returns a channel that receives LeadershipChange events.
// Buffer size 1; older events are dropped if the receiver lags.
func (e *Election) Subscribe() <-chan LeadershipChange {
	ch := make(chan LeadershipChange, 1)
	e.subsMu.Lock()
	e.subs = append(e.subs, ch)
	e.subsMu.Unlock()
	return ch
}

// Start spawns the lease loop. Returns immediately. The loop runs
// until Stop is called or ctx is canceled.
func (e *Election) Start(ctx context.Context) error {
	if !e.cfg.Enabled {
		// Disabled: assume leader (single-instance mode).
		e.setLeader(true)
		close(e.doneCh)
		return nil
	}
	if err := e.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("redis ping: %w", err)
	}
	go e.loop(ctx)
	return nil
}

// Stop releases the lock (if held) and signals the loop to exit.
// Blocks until the loop returns or ctx is cancelled.
func (e *Election) Stop(ctx context.Context) error {
	e.stopOnce.Do(func() { close(e.stopCh) })
	select {
	case <-e.doneCh:
	case <-ctx.Done():
		return ctx.Err()
	}
	if e.IsLeader() {
		_ = e.releaseUnsafe(ctx)
	}
	return e.client.Close()
}

func (e *Election) loop(ctx context.Context) {
	defer close(e.doneCh)
	tickInterval := time.Duration(e.cfg.HeartbeatIntervalSeconds) * time.Second
	if tickInterval <= 0 {
		tickInterval = 10 * time.Second
	}
	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	e.tryAcquire(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			e.tryAcquire(ctx)
		}
	}
}

// tryAcquire attempts to take or refresh the lock. The acquisition
// strategy is uniform: SET key id EX ttl NX (acquire) or, if we
// already hold it, SET key id EX ttl XX (refresh). We use a Lua
// script for safe extend-if-mine.
func (e *Election) tryAcquire(ctx context.Context) {
	ttl := time.Duration(e.cfg.LockTTLSeconds) * time.Second
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	// Try acquire (NX). On success: we are the leader.
	ok, err := e.client.SetNX(ctx, e.cfg.LockKey, e.cfg.InstanceID, ttl).Result()
	if err != nil {
		// Network blip; demote to safe.
		e.setLeader(false)
		return
	}
	if ok {
		e.setLeader(true)
		return
	}
	// SetNX failed — somebody owns the lock. Try refresh-if-mine.
	res, err := refreshIfMine.Run(ctx, e.client,
		[]string{e.cfg.LockKey}, e.cfg.InstanceID, int(ttl.Seconds())).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		e.setLeader(false)
		return
	}
	e.setLeader(res == 1)
}

func (e *Election) releaseUnsafe(ctx context.Context) error {
	_, err := releaseIfMine.Run(ctx, e.client,
		[]string{e.cfg.LockKey}, e.cfg.InstanceID).Result()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	return err
}

func (e *Election) setLeader(now bool) {
	prev := e.isLeader.Swap(now)
	if prev == now {
		return
	}
	change := LeadershipChange{IsLeader: now, At: time.Now()}
	e.subsMu.RLock()
	for _, ch := range e.subs {
		select {
		case ch <- change:
		default:
			// Drop if receiver hasn't drained; they'll see the next change.
		}
	}
	e.subsMu.RUnlock()
}

var refreshIfMine = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("EXPIRE", KEYS[1], ARGV[2])
end
return 0
`)

var releaseIfMine = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  return redis.call("DEL", KEYS[1])
end
return 0
`)
