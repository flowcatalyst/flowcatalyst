// Command fc-outbox-processor polls the consumer application's outbox
// table and forwards rows to the FlowCatalyst platform API.
//
// Phase 4 ships the Postgres backend; SQLite/MySQL/Mongo follow the
// same Repository interface and plug in via the runtime registry.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
)

func main() {
	logging.Init()

	cfg, err := config.Load(os.Getenv("FC_CONFIG_FILE"))
	if err != nil {
		slog.Error("config", "err", err)
		os.Exit(1)
	}
	if cfg.DB.URL == "" {
		cfg.DB.URL = os.Getenv("FC_DATABASE_URL")
	}
	if cfg.Outbox.PlatformURL == "" {
		slog.Error("FC_OUTBOX_PLATFORM_URL not set")
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var repo outbox.Repository
	switch cfg.Outbox.DBType {
	case "postgres", "":
		pool, err := database.NewPool(rootCtx, cfg.DB)
		if err != nil {
			slog.Error("db pool", "err", err)
			os.Exit(1)
		}
		defer pool.Close()
		repo = outboxpg.New(pool)
	default:
		slog.Error("unsupported outbox db type", "type", cfg.Outbox.DBType,
			"hint", "Phase 4 ships Postgres only; sqlite/mysql/mongo backends are phased follow-ups")
		os.Exit(1)
	}

	if err := repo.InitSchema(rootCtx); err != nil {
		slog.Error("init schema", "err", err)
		os.Exit(1)
	}

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = cfg.Outbox.PlatformURL
	pcfg.AuthToken = cfg.Outbox.PlatformAuthToken
	if cfg.Outbox.BatchSize > 0 {
		pcfg.BatchSize = cfg.Outbox.BatchSize
	}
	if cfg.Outbox.MaxInFlight > 0 {
		pcfg.MaxInFlight = int64(cfg.Outbox.MaxInFlight)
	}
	if cfg.Outbox.PollIntervalMS > 0 {
		pcfg.PollInterval = time.Duration(cfg.Outbox.PollIntervalMS) * time.Millisecond
	}

	processor := outbox.NewProcessor(pcfg, repo)
	go processor.Run(rootCtx)

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		ok := repo.Healthy(rootCtx)
		s := "ok"
		if !ok {
			s = "degraded"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    s,
			"in_flight": processor.InFlight(),
		})
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		ok, fail := processor.Totals()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"in_flight": processor.InFlight(),
			"success":   ok,
			"failed":    fail,
		})
	})

	addr := ":" + envOr("FC_METRICS_PORT", "9091")
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("fc-outbox-processor listening", "addr", addr,
			"db", cfg.Outbox.DBType, "platform_url", cfg.Outbox.PlatformURL)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	cancel()
	time.Sleep(500 * time.Millisecond)
	_ = srv.Shutdown(context.Background())
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
