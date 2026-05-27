package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/frontend"
	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/server"
)

// startOpts captures the flag set for `fc-dev start`. Defaults match
// the Rust fc-dev so existing dev workflows transfer 1:1.
type startOpts struct {
	APIPort           int
	MetricsPort       int
	EmbeddedDB        bool
	EmbeddedDBPort    int
	EmbeddedDBPath    string
	EmbeddedDBReset   bool
	DatabaseURL       string
	SchedulerEnabled  bool
	StreamEnabled     bool
	OutboxEnabled     bool
	RouterEnabled     bool
}

func newStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Run the dev monolith (identical to invoking fc-dev with no subcommand)",
		RunE:  runStart,
	}
	addStartFlags(cmd)
	return cmd
}

func addStartFlags(cmd *cobra.Command) {
	cmd.Flags().Int("api-port", envIntDefault("FC_API_PORT", 3000), "API server port")
	cmd.Flags().Int("metrics-port", envIntDefault("FC_METRICS_PORT", 9090), "metrics server port")
	cmd.Flags().Bool("embedded-db", envBoolDefault("FC_EMBEDDED_DB", true), "start an embedded Postgres")
	cmd.Flags().Int("embedded-db-port", envIntDefault("FC_EMBEDDED_DB_PORT", 5433), "embedded Postgres port")
	cmd.Flags().String("embedded-db-path", envStrDefault("FC_EMBEDDED_DB_PATH", defaultEmbeddedPath()), "embedded Postgres data directory")
	cmd.Flags().Bool("embedded-db-reset", false, "wipe the embedded Postgres data directory before starting")
	cmd.Flags().String("database-url", envStrDefault("FC_DATABASE_URL", ""), "Postgres URL (overrides --embedded-db)")
	cmd.Flags().Bool("scheduler", envBoolDefault("FC_SCHEDULER_ENABLED", true), "run the dispatch scheduler")
	cmd.Flags().Bool("stream", envBoolDefault("FC_STREAM_PROCESSOR_ENABLED", true), "run the stream processor")
	cmd.Flags().Bool("outbox", envBoolDefault("FC_OUTBOX_ENABLED", false), "run the outbox processor")
	cmd.Flags().Bool("router", envBoolDefault("FC_ROUTER_ENABLED", false), "run the message router")
}

func optsFromFlags(cmd *cobra.Command) startOpts {
	getInt := func(k string) int { v, _ := cmd.Flags().GetInt(k); return v }
	getStr := func(k string) string { v, _ := cmd.Flags().GetString(k); return v }
	getBool := func(k string) bool { v, _ := cmd.Flags().GetBool(k); return v }
	return startOpts{
		APIPort:          getInt("api-port"),
		MetricsPort:      getInt("metrics-port"),
		EmbeddedDB:       getBool("embedded-db"),
		EmbeddedDBPort:   getInt("embedded-db-port"),
		EmbeddedDBPath:   getStr("embedded-db-path"),
		EmbeddedDBReset:  getBool("embedded-db-reset"),
		DatabaseURL:      getStr("database-url"),
		SchedulerEnabled: getBool("scheduler"),
		StreamEnabled:    getBool("stream"),
		OutboxEnabled:    getBool("outbox"),
		RouterEnabled:    getBool("router"),
	}
}

func runStart(cmd *cobra.Command, _ []string) error {
	opts := optsFromFlags(cmd)
	banner(opts)

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── Embedded Postgres ──────────────────────────────────────────────
	databaseURL := opts.DatabaseURL
	var pg *embeddedpostgres.EmbeddedPostgres
	if databaseURL == "" {
		if !opts.EmbeddedDB {
			return errors.New("no --database-url given and --embedded-db=false; nothing to connect to")
		}
		if opts.EmbeddedDBReset {
			slog.Warn("wiping embedded Postgres data directory", "path", opts.EmbeddedDBPath)
			_ = os.RemoveAll(opts.EmbeddedDBPath)
		}
		if err := os.MkdirAll(opts.EmbeddedDBPath, 0o755); err != nil {
			return fmt.Errorf("create data dir: %w", err)
		}
		pg = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
			Port(uint32(opts.EmbeddedDBPort)).
			DataPath(filepath.Join(opts.EmbeddedDBPath, "data")).
			RuntimePath(filepath.Join(opts.EmbeddedDBPath, "runtime")).
			BinariesPath(filepath.Join(opts.EmbeddedDBPath, "bin")).
			Username("postgres").
			Password("postgres").
			Database("flowcatalyst").
			StartTimeout(60 * time.Second))
		if err := pg.Start(); err != nil {
			return fmt.Errorf("embedded postgres start: %w", err)
		}
		defer func() {
			slog.Info("stopping embedded postgres")
			_ = pg.Stop()
		}()
		databaseURL = fmt.Sprintf("postgresql://postgres:postgres@localhost:%d/flowcatalyst?sslmode=disable", opts.EmbeddedDBPort)
		slog.Info("embedded postgres started", "port", opts.EmbeddedDBPort, "path", opts.EmbeddedDBPath)
	}

	// ── Connect + migrate + seed ───────────────────────────────────────
	pool, err := database.NewPool(rootCtx, config.DBConfig{URL: databaseURL})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := migrate.Run(rootCtx, pool); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := seed.NewSeeder(pool).Run(rootCtx); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	// ── Platform API ───────────────────────────────────────────────────
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// fc-dev shares server.WirePlatform with fc-server but with a dev-friendly
	// envCfg (ephemeral JWT signing key, dev OAuth secret).
	if err := server.WirePlatform(r, pool, devEnvCfg(databaseURL)); err != nil {
		return fmt.Errorf("wire platform: %w", err)
	}
	go server.StartPurger(rootCtx, pool)

	// Embedded Vue SPA (built by `make frontend`). Mounted as the
	// NotFound handler so every API route registered above takes
	// precedence; only paths the API doesn't know fall through to
	// asset-or-SPA-fallback. When the binary was built without
	// `make frontend` the handler reports a clear "frontend not
	// built" message instead of an opaque 404.
	if frontend.IsAvailable() {
		r.NotFound(frontend.Handler().ServeHTTP)
		slog.Info("embedded Vue SPA mounted on /")
	} else {
		slog.Warn("frontend not embedded — run `make frontend` and rebuild to ship the SPA")
	}

	// ── Optional subsystems ────────────────────────────────────────────
	var wg sync.WaitGroup
	if opts.SchedulerEnabled {
		wg.Add(1)
		go func() { defer wg.Done(); server.StartScheduler(rootCtx, pool, devEnvCfg(databaseURL)) }()
	}
	if opts.StreamEnabled {
		wg.Add(1)
		go func() { defer wg.Done(); server.StartStreamProcessor(rootCtx, pool, devEnvCfg(databaseURL)) }()
	}
	if opts.OutboxEnabled {
		wg.Add(1)
		go func() { defer wg.Done(); server.StartOutboxProcessor(rootCtx, pool, devEnvCfg(databaseURL)) }()
	}
	if opts.RouterEnabled {
		wg.Add(1)
		go func() { defer wg.Done(); server.StartRouter(rootCtx, pool, devEnvCfg(databaseURL)) }()
	}

	// ── HTTP server ────────────────────────────────────────────────────
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", opts.APIPort),
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		slog.Info("fc-dev API listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("api server error", "err", err)
			cancel()
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		slog.Info("shutdown signal received")
	case <-rootCtx.Done():
	}
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	_ = srv.Shutdown(shutdownCtx)
	wg.Wait()
	slog.Info("fc-dev stopped")
	return nil
}

// banner prints the startup summary the way Rust fc-dev does.
func banner(opts startOpts) {
	slog.Info("=== FlowCatalyst Dev Monolith ===")
	slog.Info("subsystem configuration",
		"api_port", opts.APIPort,
		"embedded_db", opts.EmbeddedDB,
		"embedded_db_port", opts.EmbeddedDBPort,
		"scheduler", opts.SchedulerEnabled,
		"stream", opts.StreamEnabled,
		"outbox", opts.OutboxEnabled,
		"router", opts.RouterEnabled,
	)
}

func defaultEmbeddedPath() string {
	if v := os.Getenv("FC_EMBEDDED_DB_PATH"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "fc-dev-pg")
	}
	return filepath.Join(home, ".flowcatalyst", "embedded-pg")
}
