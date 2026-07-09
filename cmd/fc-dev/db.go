package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
)

// newDBCmd groups embedded-database management subcommands.
func newDBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "db",
		Short: "Manage the embedded dev database",
	}
	cmd.AddCommand(newDBUpgradeCmd())
	return cmd
}

func newDBUpgradeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Re-initialise the embedded Postgres onto the major version this fc-dev embeds",
		Long: `Bring the embedded Postgres data directory onto the major version this fc-dev
embeds (PG` + pinnedPGMajor() + `).

A Postgres major upgrade is NOT in-place, and the bundled distribution ships no
pg_dump/pg_upgrade tools, so this re-initialises the cluster from scratch:

  1. the existing data dir is moved aside to a timestamped backup (unless --no-backup),
  2. a fresh PG` + pinnedPGMajor() + ` cluster is initialised,
  3. migrations + the bootstrap seed are re-run.

A dev database is reproducible from migrations + seed, so this is the intended
path. The backup lets you recover anything bespoke by pointing your own Postgres
of the old major at it. Stop 'fc-dev start' before running this.`,
		Args: cobra.NoArgs,
		RunE: runDBUpgrade,
	}
	cmd.Flags().Int("embedded-db-port", envIntDefault("FC_EMBEDDED_DB_PORT", 15432), "embedded Postgres port")
	cmd.Flags().String("embedded-db-path", envStrDefault("FC_EMBEDDED_DB_PATH", defaultEmbeddedPath()), "embedded Postgres data directory")
	cmd.Flags().Bool("no-backup", false, "delete the old data dir instead of backing it up")
	cmd.Flags().Bool("yes", false, "skip the confirmation prompt")
	return cmd
}

func runDBUpgrade(cmd *cobra.Command, _ []string) error {
	port, _ := cmd.Flags().GetInt("embedded-db-port")
	dataPath, _ := cmd.Flags().GetString("embedded-db-path")
	noBackup, _ := cmd.Flags().GetBool("no-backup")
	yes, _ := cmd.Flags().GetBool("yes")

	have, err := embeddedDataMajor(dataPath)
	if err != nil {
		return fmt.Errorf("read embedded PG_VERSION: %w", err)
	}
	target := pinnedPGMajor()
	switch have {
	case "":
		slog.Info("no embedded cluster yet — nothing to upgrade; 'fc-dev start' will initialise it", "target", "PG"+target)
		return nil
	case target:
		slog.Info("embedded Postgres already on the target major — nothing to do", "version", "PG"+target)
		return nil
	}

	dataDir := filepath.Join(dataPath, "data")
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Embedded Postgres upgrade: PG%s → PG%s\n  data dir: %s\n", have, target, dataDir)
	if noBackup {
		fmt.Fprintln(out, "  the old cluster will be DELETED (--no-backup)")
	} else {
		fmt.Fprintln(out, "  the old cluster will be moved aside to a timestamped backup")
	}
	fmt.Fprintln(out, "  a fresh cluster is initialised; migrations + bootstrap seed re-run")

	if !yes && !confirm(cmd, "Proceed?") {
		return errors.New("aborted")
	}

	// 1. Move aside (or delete) the old cluster. The start-time version guard
	// guarantees no server is running against it (a mismatch blocks `start`).
	if noBackup {
		if err := os.RemoveAll(dataDir); err != nil {
			return fmt.Errorf("remove old data dir: %w", err)
		}
	} else {
		backup := fmt.Sprintf("%s.bak-pg%s-%s", dataDir, have, time.Now().Format("20060102-150405"))
		if err := os.Rename(dataDir, backup); err != nil {
			return fmt.Errorf("back up old data dir: %w", err)
		}
		slog.Info("old cluster backed up", "path", backup)
	}

	// 2 + 3. Fresh cluster on the target major, then migrate + seed.
	if err := initEmbeddedAndSeed(cmd.Context(), dataPath, port); err != nil {
		return err
	}
	slog.Info("embedded Postgres upgraded", "version", "PG"+target)
	fmt.Fprintf(out, "Done — now on PG%s. Sign in with the bootstrap admin (admin@flowcatalyst.local).\n", target)
	return nil
}

// initEmbeddedAndSeed boots a fresh embedded cluster, runs migrations, and seeds
// the bootstrap admin — the same first-run sequence `fc-dev start` performs.
func initEmbeddedAndSeed(ctx context.Context, dataPath string, port int) error {
	pg, err := newEmbeddedPG(dataPath, port)
	if err != nil {
		return err
	}
	if err := pg.Start(); err != nil {
		return fmt.Errorf("embedded postgres start: %w", err)
	}
	defer func() { _ = pg.Stop() }()

	url := fmt.Sprintf("postgresql://postgres:postgres@localhost:%d/flowcatalyst?sslmode=disable", port)
	pool, err := database.NewPool(ctx, database.Config{URL: url})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	if err := migrate.Run(ctx, pool); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	setEnvDefault(seed.EnvBootstrapEmail, "admin@flowcatalyst.local")
	setEnvDefault(seed.EnvBootstrapPassword, "DevPassword123!")
	setEnvDefault(seed.EnvBootstrapName, "Local Admin")
	if err := seed.NewSeeder(pool).Run(ctx); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

// confirm prompts on stdout and reads a yes/no answer from stdin.
func confirm(cmd *cobra.Command, prompt string) bool {
	fmt.Fprintf(cmd.OutOrStdout(), "%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}
