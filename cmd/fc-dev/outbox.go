package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxpg "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/postgres"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
)

// newOutboxCmd runs the SDK outbox poller pointed at an external app's
// Postgres + an external FlowCatalyst platform. Use this when the
// consumer app's DB can't be the embedded one (e.g. it already has
// extensions like PostGIS in its own Docker stack).
func newOutboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outbox",
		Short: "Standalone outbox poller against an external app DB → external platform",
		RunE:  runOutbox,
	}
	cmd.Flags().String("source-db-url", envStrDefault("FC_OUTBOX_SOURCE_DB_URL", ""), "external app's Postgres URL (required)")
	cmd.Flags().String("target-url", envStrDefault("FC_OUTBOX_PLATFORM_URL", "http://localhost:8080"), "FlowCatalyst platform URL")
	cmd.Flags().String("auth-token", envStrDefault("FC_OUTBOX_PLATFORM_AUTH_TOKEN", ""), "bearer token / OAuth client_secret used to authenticate to the platform")
	cmd.Flags().Int("batch-size", envIntDefault("FC_OUTBOX_BATCH_SIZE", 0), "rows per poll (0 = library default)")
	cmd.Flags().Int("max-in-flight", envIntDefault("FC_OUTBOX_MAX_IN_FLIGHT", 0), "outstanding HTTP requests cap (0 = library default)")
	cmd.Flags().Int("poll-interval-ms", envIntDefault("FC_OUTBOX_POLL_INTERVAL_MS", 0), "sleep between empty polls in ms (0 = library default)")
	return cmd
}

func runOutbox(cmd *cobra.Command, _ []string) error {
	sourceURL, _ := cmd.Flags().GetString("source-db-url")
	targetURL, _ := cmd.Flags().GetString("target-url")
	authToken, _ := cmd.Flags().GetString("auth-token")
	batchSize, _ := cmd.Flags().GetInt("batch-size")
	maxInFlight, _ := cmd.Flags().GetInt("max-in-flight")
	pollInterval, _ := cmd.Flags().GetInt("poll-interval-ms")

	if sourceURL == "" {
		return errors.New("--source-db-url (or FC_OUTBOX_SOURCE_DB_URL) is required")
	}

	rootCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(rootCtx, database.Config{URL: sourceURL})
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer pool.Close()

	repo := outboxpg.New(pool)
	if err := repo.InitSchema(rootCtx); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = targetURL
	pcfg.AuthToken = authToken
	if batchSize > 0 {
		pcfg.BatchSize = batchSize
	}
	if maxInFlight > 0 {
		pcfg.MaxInFlight = int64(maxInFlight)
	}
	if pollInterval > 0 {
		pcfg.PollInterval = time.Duration(pollInterval) * time.Millisecond
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		slog.Info("shutdown signal received")
		cancel()
	}()

	slog.Info("fc-dev outbox started", "source", sourceURL, "target", targetURL)
	outbox.NewProcessor(pcfg, repo).Run(rootCtx)
	slog.Info("fc-dev outbox stopped")
	return nil
}
