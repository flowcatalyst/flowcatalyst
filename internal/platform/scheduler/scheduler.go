// Package scheduler is the
// dispatch-job scheduler: polls PENDING dispatch jobs, groups by
// message_group, applies pause/block filters, and publishes to the
// message queue (SQS in prod, in-process queue in dev) for the router
// to consume.
//
// Layout:
//
//	poller.go          — PendingJobPoller (claims, hold-backs) + PausedConnectionCache
//	lane.go            — the dispatcher lanes: publish, mark QUEUED, ordering under failure
//	dispatcher.go      — MessageGroupDispatcher: renders and publishes a claimed batch
//	stale_recovery.go  — StaleQueuedJobPoller recovers stuck QUEUED / PROCESSING jobs
//	backlog.go         — PENDING backlog gauges (leader only)
//	auth.go            — DispatchAuthService (HMAC tokens for dispatch callbacks)
//
// All long-running goroutines respect ctx.Done() for graceful shutdown.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Config tunes the scheduler.
type Config struct {
	// PollInterval is how often the pending-job poller queries the DB.
	PollInterval time.Duration

	// BatchSize is the maximum number of rows one claim asks for. A claim never
	// asks for more than the free buffer (BufferCapacity).
	BatchSize int

	// BufferCapacity bounds the jobs between a claim and a lane finishing them
	// (published and marked QUEUED, or dropped). The poller blocks only when this
	// many are in flight.
	BufferCapacity int

	// Dispatchers is the number of lanes: independent workers that publish and
	// mark claimed jobs. A message group is always handled by one lane.
	Dispatchers int

	// LaneBatch is the most jobs a lane publishes (and marks QUEUED) in one go.
	LaneBatch int

	// PausedCacheTTL is how often to refresh the paused-connections set.
	PausedCacheTTL time.Duration

	// StaleQueuedAfter — jobs in QUEUED for longer than this are returned to
	// PENDING (the broker dropped the message, or the QUEUED update failed
	// after a publish).
	StaleQueuedAfter time.Duration

	// StaleProcessingAfter — jobs in PROCESSING for longer than this, with no
	// outcome recorded, are returned to PENDING.
	StaleProcessingAfter time.Duration

	// StaleScanInterval is how often the stale-recovery loop runs.
	StaleScanInterval time.Duration

	// ProcessingEndpoint is the URL stamped into every dispatch message's
	// mediation_target. The router POSTs {messageId} there; that platform
	// endpoint (POST /api/dispatch/process) performs the real webhook
	// delivery + status transitions. Empty is a misconfiguration — the
	// dispatcher would publish messages the router can't route.
	ProcessingEndpoint string
}

// Defaults for the poller/lane engine.
const (
	DefaultBufferCapacity = 1000
	DefaultDispatchers    = 10
	DefaultBatchSize      = 500
	DefaultLaneBatch      = 100
)

// normalized fills the zero values the engine cannot run with.
func (c Config) normalized() Config {
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.BufferCapacity <= 0 {
		c.BufferCapacity = DefaultBufferCapacity
	}
	if c.Dispatchers <= 0 {
		c.Dispatchers = DefaultDispatchers
	}
	if c.BatchSize <= 0 {
		c.BatchSize = DefaultBatchSize
	}
	if c.LaneBatch <= 0 {
		c.LaneBatch = DefaultLaneBatch
	}
	return c
}

// DefaultConfig holds the dispatch-job scheduler defaults: poll 1s, claim up to
// 500, 10 dispatchers publishing 100 at a time, 1000 jobs in flight, stale QUEUED
// 15m, stale PROCESSING 75m.
// fc-server overrides the buffer, dispatcher and batch sizes from
// FC_SCHEDULER_BUFFER_CAPACITY / FC_SCHEDULER_DISPATCHERS /
// FC_SCHEDULER_BATCH_SIZE (see docs/environment-variables.md).
func DefaultConfig() Config {
	return Config{
		PollInterval:   1 * time.Second,
		BatchSize:      DefaultBatchSize,
		BufferCapacity: DefaultBufferCapacity,
		Dispatchers:    DefaultDispatchers,
		LaneBatch:      DefaultLaneBatch,
		PausedCacheTTL: 60 * time.Second,
		// A QUEUED job whose message the router has parked for capacity can sit
		// QUEUED well past 15 minutes; reverting it republishes it, and the copy is
		// dropped by the router (original still in its pipeline) or skipped by the
		// delivery callback. Owner ruling 2026-10-04: that cost is acceptable and a
		// genuinely lost message is recovered in 15 minutes rather than 75.
		StaleQueuedAfter: 15 * time.Minute,
		// PROCESSING stays far past every legitimate attempt (the router's mediator
		// timeout is a 15-minute-per-attempt contract).
		StaleProcessingAfter: 75 * time.Minute,
		StaleScanInterval:    60 * time.Second,
	}
}

// Scheduler bundles the loops. Construct with New, then call
// Start(ctx) to launch all goroutines. They share the broadcast
// shutdown signal via ctx.
type Scheduler struct {
	cfg       Config
	pool      *pgxpool.Pool
	publisher DispatchPublisher

	poller      *PendingJobPoller
	dispatcher  *MessageGroupDispatcher
	stale       *StaleQueuedJobPoller
	backlog     *backlogSampler
	pausedCache *PausedConnectionCache
	authService *DispatchAuthService

	// IsLeader, when set, gates the poller + stale-recovery loops so only the
	// single active scheduler claims/reclaims jobs. Required for within-
	// message-group ordering in HA (the per-group FIFO dispatcher is in-process
	// only). nil = always run (standby disabled).
	IsLeader func() bool
}

// New wires the scheduler. publisher hands each claimed job to its destination
// queue (SQS FIFO in prod, the built-in Postgres broker in dev). The HMAC
// secret is used to sign the dispatch-job IDs that the router callback
// verifies.
func New(cfg Config, pool *pgxpool.Pool, publisher DispatchPublisher, hmacSecret string) *Scheduler {
	authSvc := NewDispatchAuthService(hmacSecret)
	pausedCache := NewPausedConnectionCache(pool, cfg.PausedCacheTTL)
	dispatcher := NewMessageGroupDispatcher(pool, publisher, authSvc, cfg.ProcessingEndpoint)
	poller := NewPendingJobPoller(cfg, pool, dispatcher, pausedCache)
	stale := NewStaleQueuedJobPoller(pool, cfg.StaleQueuedAfter, cfg.StaleProcessingAfter, cfg.StaleScanInterval)
	backlog := newBacklogSampler(pool)
	return &Scheduler{
		cfg:         cfg,
		pool:        pool,
		publisher:   publisher,
		poller:      poller,
		dispatcher:  dispatcher,
		stale:       stale,
		backlog:     backlog,
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
	s.backlog.IsLeader = s.IsLeader
	var wg sync.WaitGroup
	wg.Add(3)
	go func() { defer wg.Done(); s.poller.Run(ctx) }()
	go func() { defer wg.Done(); s.stale.Run(ctx) }()
	go func() { defer wg.Done(); s.backlog.Run(ctx) }()
	wg.Wait()
}
