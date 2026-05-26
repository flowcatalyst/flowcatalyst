// Command fc-router is the standalone webhook delivery router. Mirrors
// the Rust fc-router binary:
//   - per-pool drains with rate-limit + circuit-breaker + FIFO group ordering;
//   - HTTP webhook delivery with HMAC-SHA256 signing;
//   - in-flight tracker + stall detector + queue-health monitor;
//   - hot config reload from FLOWCATALYST_CONFIG_URL;
//   - optional Redis leader election (FC_STANDBY_ENABLED=true);
//   - graceful drain on SIGTERM (waits for in-flight messages to finish).
//
// The wiring lives in internal/router/Server so fc-server can host the
// router in-process under FC_ROUTER_ENABLED=true. This binary only adds
// signal handling and the /health, /ready, /metrics HTTP surface.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"

	// Backend registrations.
	_ "github.com/flowcatalyst/flowcatalyst-go/internal/queue/postgres"
	_ "github.com/flowcatalyst/flowcatalyst-go/internal/queue/sqs"
)

func main() {
	logging.Init()

	apiPort := envOr("API_PORT", "8080")

	srv, err := router.NewServer(router.ServerConfig{
		DevMode:          os.Getenv("FLOWCATALYST_DEV_MODE") == "true",
		ConfigURL:        os.Getenv("FLOWCATALYST_CONFIG_URL"),
		NotifyWebhookURL: os.Getenv("FC_NOTIFY_WEBHOOK_URL"),
		DrainTimeout:     durSecondsEnv("FC_DRAIN_TIMEOUT_SECONDS", 60*time.Second),
		StandbyEnabled:   os.Getenv("FC_STANDBY_ENABLED") == "true",
		StandbyRedisURL:  envOr("FC_REDIS_URL", "redis://127.0.0.1:6379"),
		StandbyLockKey:   os.Getenv("FC_STANDBY_LOCK_KEY"),
	})
	if err != nil {
		slog.Error("router init failed", "err", err)
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		if err := srv.Run(rootCtx); err != nil {
			slog.Error("router run failed", "err", err)
		}
	}()

	// HTTP surface: health probes, monitoring reads, warnings management,
	// config read — all wired by the shared internal/router/api package
	// so fc-server's StartRouter and this binary expose identical
	// endpoints. The basic /ready leader-gate + /metrics JSON snapshot
	// stay inline here as router-binary-specific concerns.
	r := chi.NewRouter()
	routerapi.RegisterRoutes(r, routerapi.FromServer(srv))
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		// Leader-gated readiness — distinct from /health/ready (which
		// checks HealthService.Status). This endpoint is the standby-
		// election signal an ALB target group would poll.
		if srv.IsLeader() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		// Prometheus emitter is deferred (HANDOFF.md §4 #7); this is
		// a JSON snapshot for ad-hoc inspection until that lands.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"pools":` + strconv.Itoa(srv.Manager.PoolCount()) +
			`,"in_flight":` + strconv.Itoa(srv.Tracker.Count()) + `}`))
	})

	httpSrv := &http.Server{
		Addr:              ":" + apiPort,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("fc-router listening", "addr", httpSrv.Addr,
			"dev_mode", srv.Cfg.DevMode, "standby", srv.Cfg.StandbyEnabled)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server error", "err", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop

	cancel()
	<-runDone

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		slog.Warn("http shutdown error", "err", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func durSecondsEnv(k string, def time.Duration) time.Duration {
	v := os.Getenv(k)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return time.Duration(n) * time.Second
}
