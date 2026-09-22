package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/oauthtoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"
)

// RunOptions lets the caller (fc-server / fcdev) extend the unified
// HTTP server without forking it. ExtraAPIRoutes runs after platform +
// router are mounted; Fallback runs as the NotFound handler (used by
// fcdev to mount the embedded Vue SPA).
type RunOptions struct {
	ExtraAPIRoutes func(r chi.Router)
	Fallback       http.Handler
}

// Run is the single orchestrator that fc-server and fcdev both call.
// Responsibilities:
//   - build the API chi mux + huma API
//   - wire the platform aggregates (when cfg.PlatformEnabled)
//   - mount the router HTTP surface under cfg.RouterHTTPPrefix (when cfg.RouterEnabled)
//   - spawn background subsystems (scheduler, stream, outbox, router engine, mcp, purger)
//   - bridge stream.HealthService → router StreamHealthProvider so the
//     dashboard reflects live projection state when co-tenanted
//   - bind the API + metrics + (optional) MCP listeners
//   - block until ctx is cancelled, then drain and return
//
// Run never panics; it returns the first listener error or nil on
// graceful shutdown.
func Run(ctx context.Context, pool *pgxpool.Pool, cfg EnvCfg, opts RunOptions) error {
	// Derive a runCtx so a listener failure can cut subsystems off
	// before the caller's defer pool.Close() / pg.Stop() fires. Without
	// this the subsystem goroutines outlive the pool and spew
	// "closed pool" errors during shutdown.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Always build a stream HealthService — empty when stream is off so
	// the router's StreamHealthProvider reports zero streams gracefully.
	streamHealth := stream.NewHealthService()

	// Dispatch queue settings are resolved ONCE, here, before any subsystem
	// starts: a deployment that asks for SQS queues without the prefix (or
	// without an addressable account/region) is misconfigured, and must fail
	// loudly at boot rather than compose meaningless queue names at runtime.
	// The same value feeds the served router-config document and the
	// scheduler's publisher, so the two can never disagree about queue type.
	dispatchSettings, err := dispatch.ResolveSettings(
		cfg.DispatchQueueType, cfg.DispatchQueueURL, cfg.DispatchQueueRegion,
		cfg.DispatchQueuePrefix, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("dispatch queue settings: %w", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Get("/health", healthHandler)

	var routerSrv *router.Server
	var routerErr error

	if cfg.PlatformEnabled {
		if err := WirePlatform(r, pool, cfg, dispatchSettings); err != nil {
			return fmt.Errorf("platform wiring: %w", err)
		}
		slog.Info("platform API wired")
	}

	if cfg.RouterEnabled {
		routerSrv, routerErr = newRouterServer(cfg, pool)
		if routerErr != nil {
			return fmt.Errorf("router init: %w", routerErr)
		}
		prefix := cfg.RouterHTTPPrefix
		if prefix == "" {
			prefix = "/router"
		}
		MountRouterHTTP(r, prefix, routerSrv, streamHealth, cfg)
		slog.Info("router HTTP mounted", "prefix", prefix)
	}

	if opts.ExtraAPIRoutes != nil {
		opts.ExtraAPIRoutes(r)
	}
	if opts.Fallback != nil {
		r.NotFound(opts.Fallback.ServeHTTP)
		// SPA history-mode routes (e.g. GET /auth/login, which an /oauth/authorize
		// redirect lands on) can collide with an API path registered for another
		// method (POST /auth/login). chi answers the method mismatch with 405
		// instead of falling through to NotFound, so the SPA never renders. Serve
		// the SPA for a GET method-mismatch; keep 405 for genuine API method
		// errors (non-GET).
		r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			if req.Method == http.MethodGet {
				opts.Fallback.ServeHTTP(w, req)
				return
			}
			w.WriteHeader(http.StatusMethodNotAllowed)
		})
	}

	// ── Listeners: BIND before any subsystem starts ───────────────────────
	// The router fetches its configuration document over HTTP, and in a
	// co-tenanted process (fcdev, or a single-binary deployment) that document
	// is served by this very listener. Binding first means the router's first
	// fetch finds an open port instead of connection-refused — the kernel
	// accepts as soon as Listen returns, and Serve below picks the connection
	// up. Serving still starts after the subsystems, so nothing is answered
	// before its dependencies are running.
	apiSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.APIPort),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	metricsSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.MetricsPort),
		Handler:           metricsRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}
	apiLn, err := net.Listen("tcp", apiSrv.Addr)
	if err != nil {
		return fmt.Errorf("api listener: %w", err)
	}
	metricsLn, err := net.Listen("tcp", metricsSrv.Addr)
	if err != nil {
		_ = apiLn.Close()
		return fmt.Errorf("metrics listener: %w", err)
	}

	// ── Background subsystems ─────────────────────────────────────────────
	var wg sync.WaitGroup
	if cfg.PlatformEnabled {
		go StartPurger(ctx, pool)

		// A-01 backstop. The router→platform settled hook
		// (POST /api/dispatch/settled) is the fast path for marking the
		// untried siblings a BLOCK_ON_ERROR group leaves behind; this sweep
		// catches what the hook misses — a dropped call, or a router that
		// died between the ACK and the callback. Without it those rows sit
		// at QUEUED/PROCESSING forever, which is the failure A-01 exists to
		// close, so the hook alone is not sufficient.
		//
		// Gated with the purger (platform housekeeping over the platform's
		// own tables) rather than with the scheduler: the rows are stranded
		// whether or not this process happens to be the one dispatching.
		// Not leader-gated — each sweep is a conditional UPDATE guarded on
		// status IN (QUEUED, PROCESSING), so concurrent instances are safe.
		go dispatchjob.RunReaper(ctx, dispatchjob.NewRepository(pool),
			dispatchjob.DefaultReaperInterval, dispatchjob.DefaultProcessingLiveAfter)
	}
	if cfg.SchedulerEnabled {
		wg.Go(func() { StartScheduler(ctx, pool, cfg, dispatchSettings) })
		slog.Info("scheduler started")
	}
	if cfg.ScheduledJobEnabled {
		wg.Go(func() { StartScheduledJobScheduler(ctx, pool, cfg) })
		slog.Info("scheduled-job scheduler started")
	}
	if cfg.StreamEnabled {
		wg.Go(func() {
			StartStreamProcessorWithHealth(ctx, pool, cfg, streamHealth)
		})
		slog.Info("stream processor started")
	}
	if cfg.OutboxEnabled {
		wg.Go(func() { StartOutboxProcessor(ctx, pool, cfg) })
		slog.Info("outbox processor started")
	}
	if cfg.RouterEnabled {
		wg.Go(func() {
			if err := routerSrv.Run(ctx); err != nil {
				slog.Warn("router run failed", "err", err)
			}
		})
		slog.Info("router engine started")
	}
	if cfg.MCPEnabled {
		wg.Go(func() { StartMCP(ctx, cfg) })
		slog.Info("mcp started")
	}

	// ── Serve on the already-bound listeners ──────────────────────────────
	listenErr := make(chan error, 2)
	go func() {
		slog.Info("api server listening", "addr", apiSrv.Addr)
		if err := apiSrv.Serve(apiLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- fmt.Errorf("api server: %w", err)
		}
	}()
	go func() {
		slog.Info("metrics server listening", "addr", metricsSrv.Addr)
		if err := metricsSrv.Serve(metricsLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			listenErr <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-listenErr:
		slog.Error("listener exited", "err", err)
		runErr = err
	}

	// Cancel every subsystem ctx BEFORE pool.Close() / pg.Stop() can fire
	// in the parent's defer chain — otherwise projectors keep polling a
	// closed pool and we spew "closed pool" errors during shutdown.
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = apiSrv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("server stopped")
	return runErr
}

// MountRouterHTTP nests the router API + dashboard + Prometheus under
// the supplied prefix. Authentication is BasicAuth (env-driven). The
// router engine itself must be started separately — this only wires
// the HTTP surface that reads its state.
func MountRouterHTTP(r chi.Router, prefix string, srv *router.Server, streamHealth *stream.HealthService, cfg EnvCfg) {
	state := routerapi.FromServer(srv)
	if streamHealth != nil {
		state.StreamHealth = streamHealthBridge{svc: streamHealth}
	}
	r.Route(prefix, func(sub chi.Router) {
		// BasicAuth on the router prefix. Disabled when no creds set.
		sub.Use(routerapi.BasicAuthMiddleware(resolveRouterAuth()))
		humaCfg := huma.DefaultConfig("FlowCatalyst Router API", routerapi.Version)
		// Nest the spec under the prefix so external tooling can grab
		// the OpenAPI doc at <prefix>/openapi.json.
		// Drop huma's $schema link injection (the wire contract never emits it), matching
		// the platform API config in wire.go.
		humaCfg.SchemasPath = ""
		api := humachi.New(sub, humaCfg)
		routerapi.Register(api, state)
		routerapi.MountDashboard(sub)
		sub.Mount("/metrics", routerapi.PrometheusHandler(state))
	})
}

// resolveRouterAuth reads the router HTTP BasicAuth config, accepting the legacy
// AUTH_BASIC_USERNAME / AUTH_BASIC_PASSWORD names as aliases for
// FC_ROUTER_AUTH_USER / FC_ROUTER_AUTH_PASS. AUTH_MODE=NONE (case-insensitive)
// forces auth off regardless of creds; any other value (incl. BASIC or unset)
// uses the resolved creds — an empty username disables auth.
// (The router HTTP surface supports basic/none
// only; OIDC modes are not honoured here.)
func resolveRouterAuth() routerapi.BasicAuthConfig {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("AUTH_MODE")), "NONE") {
		return routerapi.BasicAuthConfig{}
	}
	return routerapi.BasicAuthConfig{
		Username: envFirst("FC_ROUTER_AUTH_USER", "AUTH_BASIC_USERNAME", ""),
		Password: envFirst("FC_ROUTER_AUTH_PASS", "AUTH_BASIC_PASSWORD", ""),
	}
}

// streamHealthBridge adapts the in-process stream.HealthService into
// the routerapi.StreamHealthProvider surface. Conversion is per-call so
// the router always sees fresh counters.
type streamHealthBridge struct{ svc *stream.HealthService }

func (b streamHealthBridge) Aggregate() routerapi.StreamHealthAggregate {
	agg := b.svc.Aggregate()
	streams := make([]routerapi.StreamHealth, 0, len(agg.Streams))
	for _, s := range agg.Streams {
		streams = append(streams, routerapi.StreamHealth{
			Name:           s.Name,
			Status:         string(s.Status),
			Running:        s.Running,
			Healthy:        s.Healthy,
			BatchSequence:  s.BatchSequence,
			ErrorCount:     s.ErrorCount,
			LastPollTimeMs: s.LastPollTimeMs,
		})
	}
	return routerapi.StreamHealthAggregate{
		Healthy:          agg.Healthy,
		TotalStreams:     agg.TotalStreams,
		HealthyStreams:   agg.HealthyStreams,
		UnhealthyStreams: agg.UnhealthyStreams,
		Streams:          streams,
	}
}

func (b streamHealthBridge) IsLive() bool  { return b.svc.IsLive() }
func (b streamHealthBridge) IsReady() bool { return b.svc.IsReady() }

// newRouterServer wraps router.NewServer with the env-driven router config.
//
// There is ONE code path, dev and prod alike: the router learns its queues and
// pools from the config URL it polls, and nothing else. In dev that URL points
// at this platform's own served document, which names Postgres-backed queues
// instead of SQS ones — the dev/prod difference is the queue TYPE inside one
// document, not a separate branch here. Without a config URL the router starts
// with no queues and no pools at all; FC_DEFAULT_BROKER no longer changes that.
func newRouterServer(cfg EnvCfg, pool *pgxpool.Pool) (*router.Server, error) {
	rcfg := router.ServerConfig{
		DevMode:             cfg.RouterDevMode,
		ConfigURL:           cfg.RouterConfigURL,
		ConfigPollInterval:  time.Duration(cfg.RouterConfigIntervalSec) * time.Second,
		NotifyWebhookURL:    cfg.RouterNotifyWebhookURL,
		NotifyMinSeverity:   cfg.RouterNotifyMinSeverity,
		NotifyBatchInterval: time.Duration(cfg.RouterNotifyBatchIntervalSec) * time.Second,
		DrainTimeout:        time.Duration(cfg.RouterDrainTimeoutSec) * time.Second,
		SynthPoolIdleAge:    time.Duration(cfg.RouterSynthPoolIdleSecs) * time.Second,
		DeferralMaxDelay:    time.Duration(cfg.RouterDeferralMaxDelaySecs) * time.Second,
		DeferralBudget:      cfg.RouterDeferralBudget,
		StrictRouting:       cfg.RouterStrictRouting,
		StandbyEnabled:      cfg.StandbyEnabled,
		StandbyRedisURL:     cfg.StandbyRedisURL,
		StandbyLockKey:      cfg.StandbyLockKey,
		// ALB self-registration: register on leader-gain / non-standby start,
		// deregister on leader-loss / drain. No-op unless FC_ALB_ENABLED + the
		// target group ARN + instance IP are set.
		Traffic: router.TrafficConfig{
			Enabled:                    cfg.ALBEnabled,
			TargetGroupARN:             cfg.ALBTargetGroupARN,
			InstanceIP:                 cfg.ALBInstanceIP,
			Port:                       int32(cfg.ALBPort),
			Region:                     cfg.ALBRegion,
			DeregistrationDelaySeconds: int64(cfg.ALBDeregDelaySec),
		},
	}
	srv, err := router.NewServer(rcfg)
	if err != nil {
		return nil, err
	}

	// A-01: report the untried siblings ackBuffered ACKs off the broker when a
	// BLOCK_ON_ERROR head fails terminally, so the platform can mark the group
	// pending instead of leaving those job rows stranded at QUEUED/PROCESSING.
	// Set BEFORE any Reconfigure below, so every pool — configured and
	// synthesised alike — is built holding it.
	//
	// Left nil when no platform URL is configured: that is a standalone router
	// with nothing to report to, and it must behave exactly as it did before
	// this feature existed. The platform-side reaper is the backstop in both
	// cases, so a missing URL degrades the recovery latency (to the reaper's
	// 2-minute sweep) rather than breaking it.
	if cfg.RouterPlatformURL != "" {
		srv.Manager.SetSettledReporter(router.NewHTTPSettledReporter(
			router.SettledReporterConfig{Endpoint: cfg.RouterPlatformURL},
		))
		slog.Info("router: settled-message hook enabled", "platform_url", cfg.RouterPlatformURL)
	}

	// The router authenticates to its own platform to fetch the config
	// document. Refused here, at the composition root, rather than failing on
	// the first fetch: a half-configured credential is a deployment mistake,
	// and the router would otherwise 401 against its own platform every
	// attempt, for ever, with only a warning to show for it.
	haveID := strings.TrimSpace(cfg.RouterClientID) != ""
	haveSecret := strings.TrimSpace(cfg.RouterClientSecret) != ""
	switch {
	case haveID != haveSecret:
		return nil, errors.New(
			"FC_ROUTER_CLIENT_ID and FC_ROUTER_CLIENT_SECRET must be set together (one without the other " +
				"cannot authenticate to the platform's router-config document)")
	case haveID && strings.TrimSpace(cfg.RouterPlatformURL) == "":
		return nil, errors.New(
			"FC_ROUTER_CLIENT_ID/SECRET need FC_ROUTER_PLATFORM_URL: the credential belongs to one platform, " +
				"and a comma-separated FLOWCATALYST_CONFIG_URL may list third-party config services the " +
				"credential must never be sent to")
	case haveID && srv.ConfigSource != nil:
		srv.ConfigSource.SetCredentials(
			oauthtoken.New(cfg.RouterPlatformURL, cfg.RouterClientID, cfg.RouterClientSecret, nil),
			cfg.RouterPlatformURL)
		slog.Info("router: config document fetched with client credentials",
			"platform_url", cfg.RouterPlatformURL, "client_id", cfg.RouterClientID)
	}

	_ = pool // the router's consumers open their own connections per queue
	return srv, nil
}
