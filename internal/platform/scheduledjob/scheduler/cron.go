// Package scheduler is the scheduled-job cron + dispatch engine for
// ScheduledJob aggregates. Separate from internal/platform/scheduler, which
// schedules dispatch jobs.
//
// Two cooperating loops:
//
//   - poller — scans ACTIVE jobs, computes the LATEST cron slot in
//     (last_fired, now] per job (skip-missed semantics), claims the tick in
//     msg_scheduled_job_ticks and inserts a QUEUED instance row in one
//     transaction (so a tick fires at most once even with two pollers), and
//     advances last_fired_at monotonically. It then sleeps until the next due
//     slot across all jobs, at most PollInterval, so sub-PollInterval
//     schedules fire on time.
//   - dispatcher — every DispatchInterval, claims QUEUED instances, marks them
//     IN_FLIGHT, POSTs the firing webhook: any 2xx → DELIVERED (terminal
//     unless the job tracks completion); any other response or a transport
//     error → retry (back to QUEUED) until delivery_max_attempts, then
//     DELIVERY_FAILED. Deliberately widened from the original 202-only contract.
//     It also abandons instances stuck IN_FLIGHT after a crash (marked
//     DELIVERY_FAILED, never re-fired).
//
// Both loops are leader-gated (a single replica runs them; the loops have no
// SELECT … FOR UPDATE SKIP LOCKED claim — a single-active-replica design). The
// once-per-tick guarantee does not rest on the gate: the tick claim is a
// database primary key. All writes are direct infrastructure work
// (no UoW — instances are the firing-history projection, not an aggregate).
package scheduler

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
)

// Config controls the cron + dispatch loops.
type Config struct {
	PollInterval      time.Duration // poller wake-up (default 30s)
	DispatchInterval  time.Duration // dispatcher wake-up (default 5s)
	DispatchBatchSize int64         // max QUEUED instances per dispatch tick (default 32)
	HTTPTimeout       time.Duration // webhook HTTP timeout when the job sets no timeoutSeconds (default 10s)
	MaxHTTPTimeout    time.Duration // ceiling on a job's timeoutSeconds as the webhook timeout (default 300s)
	// InFlightMargin is added to MaxHTTPTimeout to get how long an instance may
	// sit IN_FLIGHT before it is declared abandoned (default 60s).
	InFlightMargin time.Duration
}

// DefaultConfig returns the standard defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:      30 * time.Second,
		DispatchInterval:  5 * time.Second,
		DispatchBatchSize: 32,
		HTTPTimeout:       10 * time.Second,
		MaxHTTPTimeout:    300 * time.Second,
		InFlightMargin:    60 * time.Second,
	}
}

// ConfigFromEnv builds a Config from the FC_SCHEDULED_JOB_* env vars,
// falling back to DefaultConfig:
//
//	FC_SCHEDULED_JOB_POLL_SECONDS
//	FC_SCHEDULED_JOB_DISPATCH_SECONDS
//	FC_SCHEDULED_JOB_DISPATCH_BATCH
//	FC_SCHEDULED_JOB_HTTP_TIMEOUT_SECONDS
//	FC_SCHEDULED_JOB_HTTP_TIMEOUT_MAX_SECONDS
func ConfigFromEnv() Config {
	c := DefaultConfig()
	if n, ok := envutil.Uint("FC_SCHEDULED_JOB_POLL_SECONDS"); ok {
		c.PollInterval = time.Duration(n) * time.Second
	}
	if n, ok := envutil.Uint("FC_SCHEDULED_JOB_DISPATCH_SECONDS"); ok {
		c.DispatchInterval = time.Duration(n) * time.Second
	}
	if n, ok := envutil.Uint("FC_SCHEDULED_JOB_DISPATCH_BATCH"); ok {
		c.DispatchBatchSize = int64(n)
	}
	if n, ok := envutil.Uint("FC_SCHEDULED_JOB_HTTP_TIMEOUT_SECONDS"); ok {
		c.HTTPTimeout = time.Duration(n) * time.Second
	}
	if n, ok := envutil.Uint("FC_SCHEDULED_JOB_HTTP_TIMEOUT_MAX_SECONDS"); ok {
		c.MaxHTTPTimeout = time.Duration(n) * time.Second
	}
	return c
}

// Service owns the poller + dispatcher loops.
type Service struct {
	cfg        Config
	poller     *poller
	dispatcher *dispatcher
}

// NewService wires the cron + dispatch loops. isLeader gates both loops; pass
// nil for always-leader (single-instance / standby disabled). creds
// (nil = deliver bare) lets the dispatcher stamp the owning application's
// service-account bearer token + HMAC signature on every firing.
func NewService(
	cfg Config,
	jobs *scheduledjob.Repository,
	instances *scheduledjob.InstanceRepository,
	isLeader func() bool,
	creds DeliveryCredsResolver,
) *Service {
	if isLeader == nil {
		isLeader = func() bool { return true }
	}
	// A job's TargetURL is customer input, so the dial refuses the destinations
	// netguard forbids (loopback, metadata, link-local, private by default).
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = netguard.Default.DialContext(&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
	// No client-level Timeout: each delivery carries its own deadline (the
	// job's timeoutSeconds, bounded — see requestTimeout).
	httpClient := &http.Client{Transport: transport}
	wake := make(chan struct{}, 1)
	return &Service{
		cfg:    cfg,
		poller: &poller{cfg: cfg, jobs: jobs, instances: instances, isLeader: isLeader, wake: wake},
		dispatcher: &dispatcher{
			cfg: cfg, jobs: jobs, instances: instances, http: httpClient,
			isLeader: isLeader, creds: creds, wake: wake,
		},
	}
}

// Run spawns the poller + dispatcher loops and blocks until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	slog.Info("scheduled-job scheduler starting",
		"poll", s.cfg.PollInterval, "dispatch", s.cfg.DispatchInterval, "batch", s.cfg.DispatchBatchSize)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); s.poller.run(ctx) }()
	go func() { defer wg.Done(); s.dispatcher.run(ctx) }()
	wg.Wait()
	slog.Info("scheduled-job scheduler stopped")
}
