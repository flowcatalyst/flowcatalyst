package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/bridge"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/payload"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"

	// Queue backend registrations needed by router.
	_ "github.com/flowcatalyst/flowcatalyst-go/internal/queue/postgres"
	_ "github.com/flowcatalyst/flowcatalyst-go/internal/queue/sqs"
)

// StartScheduler runs the dispatch-job scheduler (poller + dispatcher +
// stale recovery). Blocks until ctx is cancelled.
//
// The publisher is supplied by env-configured queue backend in
// production; in development the noop publisher below keeps the loops
// alive without external dependencies. TODO(scheduler-runtime): wire
// the real queue.Publisher via internal/queue.NewPublisher once the
// QueueConfig env knobs are exposed in envcfg.go.
func StartScheduler(ctx context.Context, pool *pgxpool.Pool, _ EnvCfg) {
	cfg := scheduler.DefaultConfig()
	s := scheduler.New(cfg, pool, NoopPublisher{}, "fc-dispatch-hmac-secret-todo")
	s.Run(ctx)
	slog.Info("scheduler stopped")
}

// StartStreamProcessor runs the CQRS projections (events + dispatch
// jobs) + fan-out + partition manager. Each sub-projection has its own
// env toggle and defaults to ON when FC_STREAM_PROCESSOR_ENABLED=true.
// Blocks until ctx is cancelled, at which point all child loops drain
// and the function returns.
func StartStreamProcessor(ctx context.Context, pool *pgxpool.Pool, cfg EnvCfg) {
	pcfg := stream.DefaultProjectorConfig()
	if cfg.StreamBatchSize > 0 {
		pcfg.BatchSize = cfg.StreamBatchSize
	}

	var wg sync.WaitGroup
	launch := func(name string, run func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(ctx)
		}()
		slog.Info("stream subsystem started", "name", name)
	}

	if cfg.StreamEventsEnabled {
		launch("event_projection", stream.NewEventProjection(pool).Projector(pcfg).Run)
	}
	if cfg.StreamDispatchJobsEnabled {
		launch("dispatch_job_projection", stream.NewDispatchJobProjection(pool).Projector(pcfg).Run)
	}
	if cfg.StreamFanOutEnabled {
		launch("event_fan_out", stream.NewFanOut(pool).Projector(pcfg).Run)
	}
	if cfg.StreamPartitionsEnabled {
		launch("partition_manager", stream.NewPartitionManager(pool).Run)
	}

	wg.Wait()
	slog.Info("stream processor stopped")
}

// StartOutboxProcessor runs the consumer-app SDK outbox poller against
// the same Postgres pool that hosts the platform. The standalone
// cmd/fc-outbox-processor binary remains the home for the (forthcoming)
// sqlite/mysql/mongo backends — fc-server only supports the Postgres
// path so it can reuse the shared pool.
func StartOutboxProcessor(ctx context.Context, pool *pgxpool.Pool, cfg EnvCfg) {
	if cfg.OutboxPlatformURL == "" {
		slog.Error("outbox processor enabled but FC_OUTBOX_PLATFORM_URL not set; skipping")
		return
	}

	repo := outboxpg.New(pool)
	if err := repo.InitSchema(ctx); err != nil {
		slog.Error("outbox init schema failed", "err", err)
		return
	}

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = cfg.OutboxPlatformURL
	pcfg.AuthToken = cfg.OutboxPlatformAuthToken
	if cfg.OutboxBatchSize > 0 {
		pcfg.BatchSize = cfg.OutboxBatchSize
	}
	if cfg.OutboxMaxInFlight > 0 {
		pcfg.MaxInFlight = int64(cfg.OutboxMaxInFlight)
	}
	if cfg.OutboxPollIntervalMS > 0 {
		pcfg.PollInterval = time.Duration(cfg.OutboxPollIntervalMS) * time.Millisecond
	}

	p := outbox.NewProcessor(pcfg, repo)
	slog.Info("outbox processor started", "platform_url", cfg.OutboxPlatformURL)
	p.Run(ctx)
	slog.Info("outbox processor stopped")
}

// StartRouter runs the SQS message router in-process. Shares the
// internal/router/Server with the standalone cmd/fc-router binary —
// the cmd binary keeps the HTTP listener + signal handler, this
// launcher just hosts the wiring inside fc-server.
//
// pool is unused today: the router reads its pool definitions from
// FLOWCATALYST_CONFIG_URL, and queue backends (Postgres/SQS) are
// constructed per-pool inside the router. The signature keeps pool in
// case a future co-tenanted Postgres queue backend wants to share it.
func StartRouter(ctx context.Context, _ *pgxpool.Pool, cfg EnvCfg) {
	rcfg := router.ServerConfig{
		DevMode:          cfg.RouterDevMode,
		ConfigURL:        cfg.RouterConfigURL,
		NotifyWebhookURL: cfg.RouterNotifyWebhookURL,
		DrainTimeout:     time.Duration(cfg.RouterDrainTimeoutSec) * time.Second,
		StandbyEnabled:   cfg.StandbyEnabled,
		StandbyRedisURL:  cfg.StandbyRedisURL,
		StandbyLockKey:   cfg.StandbyLockKey,
	}
	srv, err := router.NewServer(rcfg)
	if err != nil {
		slog.Error("router init failed", "err", err)
		return
	}
	if err := srv.Run(ctx); err != nil {
		slog.Error("router run failed", "err", err)
	}
}

// StartPurger runs the periodic housekeeping loop that drops expired
// rows from the three ephemeral auth tables: oauth_oidc_payloads
// (access/refresh tokens), oauth_oidc_login_states (the in-flight OIDC
// bridge state), and webauthn_ceremonies (in-flight registration /
// authentication challenges). Mirrors Rust's background
// payload_purge_loop. Always-on; no env toggle.
//
// Cadence: every minute. Idempotent — each purge is a DELETE WHERE
// expires_at < NOW(). Failures are logged and the loop keeps going.
func StartPurger(ctx context.Context, pool *pgxpool.Pool) {
	payloadRepo := payload.NewRepository(pool)
	loginStateRepo := bridge.NewLoginStateRepo(pool)
	ceremonyRepo := webauthn.NewCeremonyRepository(pool)

	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	slog.Info("auth purger started")
	for {
		select {
		case <-ctx.Done():
			slog.Info("auth purger stopped")
			return
		case <-tick.C:
			if n, err := payloadRepo.PurgeExpired(ctx); err != nil {
				slog.Warn("oauth payload purge failed", "err", err)
			} else if n > 0 {
				slog.Debug("oauth payload purge", "removed", n)
			}
			if n, err := loginStateRepo.PurgeExpired(ctx); err != nil {
				slog.Warn("oidc login state purge failed", "err", err)
			} else if n > 0 {
				slog.Debug("oidc login state purge", "removed", n)
			}
			if n, err := ceremonyRepo.PurgeExpired(ctx); err != nil {
				slog.Warn("webauthn ceremony purge failed", "err", err)
			} else if n > 0 {
				slog.Debug("webauthn ceremony purge", "removed", n)
			}
		}
	}
}

// NoopPublisher satisfies queue.Publisher without doing anything. Used
// when the scheduler is enabled but no queue backend is configured —
// the poller still runs (so QUEUED rows drain into the noop), but no
// downstream router consumes them. This keeps the boot path green
// during initial deployment validation.
type NoopPublisher struct{}

func (NoopPublisher) Identifier() string { return "noop" }
func (NoopPublisher) Publish(_ context.Context, _ common.Message) (string, error) {
	return "noop", nil
}
func (NoopPublisher) PublishBatch(_ context.Context, msgs []common.Message) ([]string, error) {
	out := make([]string, len(msgs))
	for i := range msgs {
		out[i] = "noop"
	}
	return out, nil
}

// keep the queue import live so the noop assertion compiles.
var _ queue.Publisher = NoopPublisher{}
