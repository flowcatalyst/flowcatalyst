// Command fc-stream-processor runs the CQRS projections + fan-out +
// partition manager. Each loop is gated on its enable flag so a single
// binary can run any subset (matches the Rust fc-stream-processor envs).
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/stream"
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
	if cfg.DB.URL == "" {
		slog.Error("FC_DATABASE_URL not set")
		os.Exit(1)
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(rootCtx, cfg.DB)
	if err != nil {
		slog.Error("db pool", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	pcfg := stream.DefaultProjectorConfig()
	pcfg.BatchSize = cfg.Stream.BatchSize
	if pcfg.BatchSize == 0 {
		pcfg.BatchSize = 500
	}

	var wg sync.WaitGroup
	launch := func(name string, run func(context.Context)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			run(rootCtx)
		}()
		slog.Info("started subsystem", "name", name)
	}

	if cfg.Stream.EventsEnabled {
		ep := stream.NewEventProjection(pool).Projector(pcfg)
		launch("event_projection", ep.Run)
	}
	if cfg.Stream.DispatchJobsEnabled {
		dp := stream.NewDispatchJobProjection(pool).Projector(pcfg)
		launch("dispatch_job_projection", dp.Run)
	}
	if cfg.Stream.FanOutEnabled {
		fo := stream.NewFanOut(pool).Projector(pcfg)
		launch("event_fan_out", fo.Run)
	}
	if cfg.Stream.PartitionsEnabled {
		pm := stream.NewPartitionManager(pool)
		launch("partition_manager", pm.Run)
	}

	r := chi.NewRouter()
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	addr := ":" + envOr("FC_METRICS_PORT", "9090")
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("fc-stream-processor listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	cancel()
	wg.Wait()
	_ = srv.Shutdown(context.Background())
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
