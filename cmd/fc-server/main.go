// Command fc-server is the FlowCatalyst unified production server.
//
// Single binary; every subsystem is independently togglable via
// FC_*_ENABLED env vars so the same image can be deployed as the API
// tier, a worker tier, or both. Mirrors the Rust fc-server's env
// contract. fc-dev wraps the same `server.Run` orchestrator with
// embedded-Postgres + dev defaults for local work.
//
// See internal/server/envcfg.go for the full env-var list.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/frontend"
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
		"mcp", cfg.MCPEnabled,
		"standby", cfg.StandbyEnabled,
		"api_port", cfg.APIPort,
		"metrics_port", cfg.MetricsPort,
	)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The platform database is only needed by subsystems that read/write
	// Postgres. A router-only deployment (the drop-in for the Rust standalone
	// fc-router) needs no database — it reads its config from the platform API
	// and processes the queue — so skip the connect/migrate/seed entirely. The
	// router path in server.Run never dereferences the pool, so a nil pool is
	// safe. MCP also dials the platform over HTTP rather than the pool.
	needsDB := cfg.PlatformEnabled || cfg.StreamEnabled || cfg.SchedulerEnabled ||
		cfg.ScheduledJobEnabled || cfg.OutboxEnabled

	var pool *pgxpool.Pool
	if needsDB {
		// AWS Secrets Manager DB mode: when DB_SECRET_ARN + DB_HOST are set (and no
		// explicit FC_DATABASE_URL/DATABASE_URL), resolve the connection from the
		// secret. Mirrors the Rust fc-server DB-secret resolution.
		if dbURL, ok, err := server.ResolveDBSecretURL(rootCtx); err != nil {
			slog.Error("resolve DB secret failed", "err", err)
			os.Exit(1) //nolint:gocritic // fatal startup error before the run loop; the deferred cancel is moot as the process is exiting
		} else if ok {
			cfg.DatabaseURL = dbURL
			slog.Info("resolved database URL from AWS Secrets Manager")
		}

		// DB-secret rotation: when SM mode + DB_SECRET_REFRESH_INTERVAL_MS != 0,
		// poll for rotated creds and inject them into new connections (no restart).
		dbCfg := database.Config{URL: cfg.DatabaseURL}
		var beforeConnect func(context.Context, *pgx.ConnConfig) error
		if refresher, err := server.NewDBSecretRefresher(rootCtx); err != nil {
			slog.Error("DB secret refresher init failed", "err", err)
			os.Exit(1)
		} else if refresher != nil {
			beforeConnect = refresher.BeforeConnect
			go refresher.Run(rootCtx)
			slog.Info("DB secret rotation enabled")
		}

		p, err := database.NewPoolWithBeforeConnect(rootCtx, dbCfg, beforeConnect)
		if err != nil {
			slog.Error("postgres connect failed", "err", err)
			os.Exit(1)
		}
		defer p.Close()
		pool = p
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
	} else {
		slog.Info("no database-backed subsystem enabled; skipping postgres connect/migrate/seed",
			"router", cfg.RouterEnabled, "mcp", cfg.MCPEnabled)
	}

	// SIGTERM / SIGINT → cancel rootCtx → server.Run drains.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("shutdown signal received")
		cancel()
	}()

	// Serve the embedded Vue SPA when it was built in AND the platform API is
	// enabled — it's the platform dashboard and it does client-side OIDC login.
	// A router-only or worker-only instance must NOT serve it, otherwise hitting
	// "/" redirects to an OIDC flow that instance can't satisfy; the router's own
	// UI/API lives under the /router prefix (basic-auth). No-op when dist wasn't
	// embedded.
	runOpts := server.RunOptions{}
	if cfg.PlatformEnabled && frontend.IsAvailable() {
		runOpts.Fallback = frontend.Handler()
		slog.Info("embedded Vue SPA available")
	}

	if err := server.Run(rootCtx, pool, cfg, runOpts); err != nil {
		slog.Error("fc-server exited with error", "err", err)
		os.Exit(1)
	}
}
