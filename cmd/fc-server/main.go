// Command fc-server is the FlowCatalyst unified production server.
//
// Single binary combining the platform API, message router, dispatch
// scheduler, CQRS stream processor, and outbox processor. Each subsystem
// is independently togglable via FC_*_ENABLED env vars so the same
// binary can be deployed as the API tier, a worker tier, or both.
//
// Mirrors the Rust fc-server binary (bin/fc-server/src/main.rs) with
// the same env-var contract. Drop-in replacement for the Rust build.
//
// See envcfg.go for the full list of env vars.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

func main() {
	logging.Init()
	cfg := server.LoadEnv()

	slog.Info("starting fc-server",
		"platform", cfg.PlatformEnabled,
		"router", cfg.RouterEnabled,
		"scheduler", cfg.SchedulerEnabled,
		"stream", cfg.StreamEnabled,
		"outbox", cfg.OutboxEnabled,
		"standby", cfg.StandbyEnabled,
		"api_port", cfg.APIPort,
		"metrics_port", cfg.MetricsPort)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Database + migrations ──────────────────────────────────────────────
	pool, err := database.NewPool(rootCtx, config.DBConfig{URL: cfg.DatabaseURL})
	if err != nil {
		slog.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()
	slog.Info("postgres connected")

	if err := migrate.Run(rootCtx, pool); err != nil {
		slog.Error("migrations failed", "err", err)
		os.Exit(1)
	}
	slog.Info("migrations applied")

	if err := seed.NewSeeder(pool).Run(rootCtx); err != nil {
		slog.Error("seed failed", "err", err)
		os.Exit(1)
	}
	slog.Info("seed complete")

	// ── HTTP app (platform API) ───────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Get("/health", healthHandler)

	if cfg.PlatformEnabled {
		if err := server.WirePlatform(r, pool, cfg); err != nil {
			slog.Error("platform wiring failed", "err", err)
			os.Exit(1)
		}
		slog.Info("platform API wired")
		go server.StartPurger(rootCtx, pool)
	}

	// ── Background processors ──────────────────────────────────────────────
	var wg sync.WaitGroup
	if cfg.SchedulerEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.StartScheduler(rootCtx, pool, cfg)
		}()
		slog.Info("scheduler started")
	}
	if cfg.StreamEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.StartStreamProcessor(rootCtx, pool, cfg)
		}()
		slog.Info("stream processor started")
	}
	if cfg.OutboxEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.StartOutboxProcessor(rootCtx, pool, cfg)
		}()
		slog.Info("outbox processor started")
	}
	if cfg.RouterEnabled {
		wg.Add(1)
		go func() {
			defer wg.Done()
			server.StartRouter(rootCtx, pool, cfg)
		}()
		slog.Info("router started")
	}

	// ── HTTP servers ───────────────────────────────────────────────────────
	apiSrv := &http.Server{
		Addr:              ":" + intToStr(cfg.APIPort),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	metricsSrv := &http.Server{
		Addr:              ":" + intToStr(cfg.MetricsPort),
		Handler:           metricsRouter(cfg),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		slog.Info("api server listening", "addr", apiSrv.Addr)
		if err := apiSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api server error", "err", err)
			cancel()
		}
	}()
	go func() {
		slog.Info("metrics server listening", "addr", metricsSrv.Addr)
		if err := metricsSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("metrics server error", "err", err)
		}
	}()

	// ── Shutdown ───────────────────────────────────────────────────────────
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		slog.Info("shutdown signal received")
	case <-rootCtx.Done():
		slog.Info("root context cancelled")
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = apiSrv.Shutdown(shutdownCtx)
	_ = metricsSrv.Shutdown(shutdownCtx)

	wg.Wait()
	slog.Info("fc-server stopped")
}

// ── handlers ────────────────────────────────────────────────────────────

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func metricsRouter(cfg server.EnvCfg) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ready",
			"platform":  cfg.PlatformEnabled,
			"router":    cfg.RouterEnabled,
			"scheduler": cfg.SchedulerEnabled,
			"stream":    cfg.StreamEnabled,
			"outbox":    cfg.OutboxEnabled,
		})
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// TODO(observability): Prometheus exposition. The Rust impl emits a
		// gauge per subsystem; replicate via a small expvar→prometheus
		// bridge once we wire metric counters into each subsystem.
		_, _ = w.Write([]byte("# fc-server metrics placeholder\n"))
	})
	return r
}

func intToStr(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [10]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		return "-" + string(buf[i:])
	}
	return string(buf[i:])
}
