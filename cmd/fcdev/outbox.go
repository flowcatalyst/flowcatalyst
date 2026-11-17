package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
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
		Long: `Run the outbox poller against an application's Postgres database: it claims the rows
the application writes to outbox_messages and delivers them to the FlowCatalyst platform.

Who creates outbox_messages (--init-schema, env FC_OUTBOX_INIT_SCHEMA, default true):

  true   At start the poller runs CREATE TABLE IF NOT EXISTS outbox_messages and its three
         indexes (the same DDL as 'fcdev outbox create-table'). For an application that has
         no migration of its own for the table.

  false  The poller runs no DDL at all against the database. If outbox_messages is not there
         yet it waits for it (it checks every 2 seconds and logs that it is waiting, again
         every minute while it waits), then starts polling. A database it cannot reach, or a
         wrong password, is still an error.

Set --init-schema=false (FC_OUTBOX_INIT_SCHEMA=false) whenever the application's own
migrations create outbox_messages: the Go framework's outbox module, or an SDK migration
(clients/*/migrations) run by the application's migration tool. Those migrations use a plain
CREATE TABLE; if the poller starts first (a second container of the same task, started
together with the application) and creates the table, the application's first migration fails
with 'relation "outbox_messages" already exists', the application exits, and every restart
fails the same way because the table stays and no migration is recorded.

The poller reads Postgres only; there is no other backend for this command.`,
		// Load a dotenv file (default ./.env) before resolving flags, so the
		// FC_OUTBOX_* vars can live in the app's .env instead of being exported.
		// Existing env vars always win; this also runs for `create-table`.
		PersistentPreRunE: func(c *cobra.Command, _ []string) error {
			envFile, _ := c.Flags().GetString("env-file")
			loadDotEnv(envFile)
			return nil
		},
		Args: cobra.NoArgs, // `fcdev outbox poll` / `init` do not exist: fail loudly rather than run the poller
		RunE: runOutbox,
	}
	cmd.PersistentFlags().String("env-file", ".env", "load environment from this dotenv file (does not override existing env)")
	cmd.Flags().String("source-db-url", envStrDefault("FC_OUTBOX_SOURCE_DB_URL", ""), "external app's Postgres URL (required)")
	cmd.Flags().String("target-url", envStrDefault("FC_OUTBOX_PLATFORM_URL", "http://localhost:8080"), "FlowCatalyst platform URL")
	cmd.Flags().String("auth-token", envStrDefault("FC_OUTBOX_PLATFORM_AUTH_TOKEN", ""), "static bearer token for the platform (used when client-id/secret are not set)")
	cmd.Flags().String("client-id", "", "OAuth client_credentials client id (env FC_OUTBOX_CLIENT_ID, falls back to FLOWCATALYST_CLIENT_ID)")
	cmd.Flags().String("client-secret", "", "OAuth client_credentials client secret (env FC_OUTBOX_CLIENT_SECRET, falls back to FLOWCATALYST_CLIENT_SECRET)")
	cmd.Flags().String("token-url", "", "OAuth token endpoint (env FC_OUTBOX_TOKEN_URL, default <target-url>/oauth/token)")
	cmd.Flags().String("scope", "", "optional requested scope to narrow the minted token (env FC_OUTBOX_SCOPE)")
	cmd.Flags().Int("batch-size", envIntDefault("FC_OUTBOX_BATCH_SIZE", 0), "rows per poll (0 = library default)")
	cmd.Flags().Int("max-in-flight", envIntDefault("FC_OUTBOX_MAX_IN_FLIGHT", 0), "outstanding HTTP requests cap (0 = library default)")
	cmd.Flags().Int("poll-interval-ms", envIntDefault("FC_OUTBOX_POLL_INTERVAL_MS", 0), "sleep between empty polls in ms (0 = library default)")
	cmd.Flags().Bool("init-schema", envBoolDefault("FC_OUTBOX_INIT_SCHEMA", true),
		"create outbox_messages and its indexes at start if missing (env FC_OUTBOX_INIT_SCHEMA); false: run no DDL and wait for the application's migrations to create the table")
	cmd.AddCommand(newOutboxCreateTableCmd())
	return cmd
}

// outboxTableWait paces the wait for outbox_messages when --init-schema is
// false (zero: the repository's defaults). Tests shorten it.
var outboxTableWait outboxpg.WaitOptions

func runOutbox(cmd *cobra.Command, _ []string) error {
	// Re-resolve from env (the dotenv file is loaded in PersistentPreRunE,
	// after the flag defaults were baked at command-build time). Precedence:
	// explicit flag > env (incl. .env) > default.
	sourceURL := resolveEnvFlag(cmd, "source-db-url", "FC_OUTBOX_SOURCE_DB_URL")
	targetURL := resolveEnvFlag(cmd, "target-url", "FC_OUTBOX_PLATFORM_URL")
	authToken := resolveEnvFlag(cmd, "auth-token", "FC_OUTBOX_PLATFORM_AUTH_TOKEN")
	clientID := resolveEnvFlagMulti(cmd, "client-id", "FC_OUTBOX_CLIENT_ID", "FLOWCATALYST_CLIENT_ID")
	clientSecret := resolveEnvFlagMulti(cmd, "client-secret", "FC_OUTBOX_CLIENT_SECRET", "FLOWCATALYST_CLIENT_SECRET")
	tokenURL := resolveEnvFlag(cmd, "token-url", "FC_OUTBOX_TOKEN_URL")
	scope := resolveEnvFlag(cmd, "scope", "FC_OUTBOX_SCOPE")
	batchSize := resolveEnvFlagInt(cmd, "batch-size", "FC_OUTBOX_BATCH_SIZE")
	maxInFlight := resolveEnvFlagInt(cmd, "max-in-flight", "FC_OUTBOX_MAX_IN_FLIGHT")
	pollInterval := resolveEnvFlagInt(cmd, "poll-interval-ms", "FC_OUTBOX_POLL_INTERVAL_MS")
	initSchema, err := resolveEnvFlagBool(cmd, "init-schema", "FC_OUTBOX_INIT_SCHEMA")
	if err != nil {
		return err
	}

	if sourceURL == "" {
		return errors.New("--source-db-url (or FC_OUTBOX_SOURCE_DB_URL) is required")
	}

	rootCtx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Stop on SIGINT/SIGTERM from here on, so a poller still waiting for its
	// table stops promptly too.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		select {
		case <-stop:
			slog.Info("shutdown signal received")
			cancel()
		case <-rootCtx.Done():
		}
	}()

	pool, err := database.NewPool(rootCtx, database.Config{URL: sourceURL})
	if err != nil {
		return fmt.Errorf("connect source: %w", err)
	}
	defer pool.Close()

	repo := outboxpg.New(pool)
	if initSchema {
		if err := repo.InitSchema(rootCtx); err != nil {
			return fmt.Errorf("init schema: %w", err)
		}
	} else if err := repo.WaitForTable(rootCtx, outboxTableWait); err != nil {
		if rootCtx.Err() != nil {
			slog.Info("fcdev outbox stopped before outbox_messages was created")
			return nil
		}
		return err
	}

	pcfg := outbox.DefaultConfig()
	pcfg.PlatformURL = targetURL
	pcfg.AuthToken = authToken

	// When a service-account client_id/secret is available, mint and auto-refresh
	// the platform token via client_credentials (preferred — survives long runs).
	// Otherwise fall back to the static auth-token.
	authMode := "none"
	if authToken != "" {
		authMode = "static-token"
	}
	if clientID != "" && clientSecret != "" {
		if tokenURL == "" {
			tokenURL = strings.TrimRight(targetURL, "/") + "/oauth/token"
		}
		pcfg.TokenSource = newClientCredentialsTokenSource(tokenURL, clientID, clientSecret, scope)
		authMode = "client_credentials"
	}

	if batchSize > 0 {
		pcfg.BatchSize = batchSize
	}
	if maxInFlight > 0 {
		pcfg.MaxInFlight = int64(maxInFlight)
	}
	if pollInterval > 0 {
		pcfg.PollInterval = time.Duration(pollInterval) * time.Millisecond
	}

	slog.Info("fcdev outbox started", "source", sourceURL, "target", targetURL, "auth", authMode)
	outbox.NewProcessor(pcfg, repo).Run(rootCtx)
	slog.Info("fcdev outbox stopped")
	return nil
}
