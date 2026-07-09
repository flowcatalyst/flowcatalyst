// Package scheduler is the port of fc-platform/src/scheduler. It is the
// dispatch-job scheduler: polls PENDING dispatch jobs, groups by
// message_group, applies pause/block filters, and publishes to the
// message queue (SQS in prod, in-process queue in dev) for the router
// to consume.
//
// Mirrors the Rust scheduler subdomain layout:
//
//	poller.go          — PendingJobPoller + PausedConnectionCache
//	dispatcher.go      — MessageGroupDispatcher with per-group FIFO + semaphore
//	stale_recovery.go  — StaleQueuedJobPoller recovers stuck QUEUED jobs
//	auth.go            — DispatchAuthService (HMAC tokens for dispatch callbacks)
//
// All long-running goroutines respect ctx.Done() for graceful shutdown.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// Config tunes the scheduler.
type Config struct {
	// PollInterval is how often the pending-job poller queries the DB.
	PollInterval time.Duration

	// BatchSize is the maximum number of jobs claimed per poll — and, since
	// the dispatcher publishes the whole claim in one PublishBatch, also the
	// upper bound on messages sent per tick (the SQS backend chunks these to
	// SendMessageBatch's 10-per-call limit).
	BatchSize int

	// PausedCacheTTL is how often to refresh the paused-connections set.
	PausedCacheTTL time.Duration

	// StaleAfter — jobs in QUEUED for longer than this are reclaimed
	// (their visibility lease has expired or the broker dropped them).
	StaleAfter time.Duration

	// StaleScanInterval is how often the stale-recovery loop runs.
	StaleScanInterval time.Duration

	// ProcessingEndpoint is the URL stamped into every dispatch message's
	// mediation_target. The router POSTs {messageId} there; that platform
	// endpoint (POST /api/dispatch/process) performs the real webhook
	// delivery + status transitions. Empty is a misconfiguration — the
	// dispatcher would publish messages the router can't route.
	ProcessingEndpoint string
}

// DefaultConfig holds the Go dispatch-job scheduler defaults. These are
// intentionally Go's own (more standard) values, NOT a 1:1 copy of the Rust
// scheduler: Rust uses poll 5s / batch 200 / max_concurrent_groups 10 /
// stale 15m. The owner chose to keep Go's faster, more conventional defaults
// (poll 1s / batch 100 / in-flight 1000 / stale 5m). All are env-overridable.
func DefaultConfig() Config {
	return Config{
		PollInterval:      1 * time.Second,
		BatchSize:         100,
		PausedCacheTTL:    60 * time.Second,
		StaleAfter:        5 * time.Minute,
		StaleScanInterval: 60 * time.Second,
	}
}

// Scheduler bundles the four loops. Construct with New, then call
// Start(ctx) to launch all goroutines. They share the broadcast
// shutdown signal via ctx.
type Scheduler struct {
	cfg       Config
	pool      *pgxpool.Pool
	publisher queue.Publisher

	poller      *PendingJobPoller
	dispatcher  *MessageGroupDispatcher
	stale       *StaleQueuedJobPoller
	pausedCache *PausedConnectionCache
	authService *DispatchAuthService

	// IsLeader, when set, gates the poller + stale-recovery loops so only the
	// single active scheduler claims/reclaims jobs. Required for within-
	// message-group ordering in HA (the per-group FIFO dispatcher is in-process
	// only). nil = always run (standby disabled). Mirrors Rust's active_rx gate
	// on spawn_scheduler.
	IsLeader func() bool
}

// New wires the scheduler. publisher publishes to the queue (typically
// SQS in prod). The HMAC secret is used to sign the dispatch-job IDs
// that the router callback verifies.
func New(cfg Config, pool *pgxpool.Pool, publisher queue.Publisher, hmacSecret string) *Scheduler {
	authSvc := NewDispatchAuthService(hmacSecret)
	pausedCache := NewPausedConnectionCache(pool, cfg.PausedCacheTTL)
	dispatcher := NewMessageGroupDispatcher(pool, publisher, authSvc, cfg.ProcessingEndpoint)
	poller := NewPendingJobPoller(cfg, pool, dispatcher, pausedCache)
	stale := NewStaleQueuedJobPoller(pool, cfg.StaleAfter, cfg.StaleScanInterval)
	return &Scheduler{
		cfg:         cfg,
		pool:        pool,
		publisher:   publisher,
		poller:      poller,
		dispatcher:  dispatcher,
		stale:       stale,
		pausedCache: pausedCache,
		authService: authSvc,
	}
}

// Poller exposes the poller for tests / external callers.
func (s *Scheduler) Poller() *PendingJobPoller { return s.poller }

// Dispatcher exposes the dispatcher.
func (s *Scheduler) Dispatcher() *MessageGroupDispatcher { return s.dispatcher }

// AuthService exposes the dispatch-callback HMAC service.
func (s *Scheduler) AuthService() *DispatchAuthService { return s.authService }

// Run starts the poller + stale-recovery loops and blocks until ctx is
// cancelled. The dispatcher is event-driven via Submit calls from the
// poller, so it doesn't need its own loop. fc-server uses this entry
// point when FC_SCHEDULER_ENABLED=true.
func (s *Scheduler) Run(ctx context.Context) {
	s.poller.IsLeader = s.IsLeader
	s.stale.IsLeader = s.IsLeader
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.poller.Run(ctx) }()
	go func() { defer wg.Done(); s.stale.Run(ctx) }()
	wg.Wait()
}
