// Command fcdev is the FlowCatalyst developer monolith — a single
// binary that runs every subsystem against an embedded Postgres so
// engineers can iterate without Docker. Mirrors the Rust fcdev's
// subcommand surface:
//
//	fcdev start  — run the dev monolith (default; matches `fcdev` no-arg).
//	fcdev stop   — stop a running dev monolith (graceful; via its PID file).
//	fcdev init   — bootstrap admin user + default tenant + .env file.
//	fcdev fresh  — truncate every FlowCatalyst table (preserves schema).
//	fcdev mcp     — run the FlowCatalyst MCP server.
//	fcdev outbox  — standalone outbox poller for external apps.
//	fcdev upgrade — self-update to the latest GitHub release.
package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
)

func main() {
	logging.Init()

	root := &cobra.Command{
		Use:   "fcdev",
		Short: "FlowCatalyst Development Monolith — all components in one binary",
		Long: `fcdev runs every FlowCatalyst subsystem in one process against an
embedded Postgres database. Designed for local development: no Docker,
no docker-compose, no separate migration step.

Invoking ` + "`fcdev`" + ` with no subcommand is identical to ` + "`fcdev start`" + `.`,
		// No-arg invocation runs start. Matches the Rust fcdev UX.
		RunE: runStart,
		// Runtime failures (port in use, DB unreachable) shouldn't trigger
		// cobra's "did you mean…" help dump — that noise hides the real
		// slog.Error line. We log + exit ourselves in main.
		SilenceUsage:  true,
		SilenceErrors: true,
		// `fcdev --version` prints the same string as `fcdev version` (users
		// reach for the flag reflexively). The subcommand stays for parity with
		// the Rust CLI's surface.
		Version: version() + vcsSuffix(),
	}
	root.SetVersionTemplate("fcdev {{.Version}}\n")
	// Mirror start's flag set onto the root command so `fcdev` and
	// `fcdev start` accept the same options.
	addStartFlags(root)

	root.AddCommand(newStartCmd())
	root.AddCommand(newStopCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newFreshCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newOutboxCmd())
	root.AddCommand(newDBCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		slog.Error("fcdev exited with error", "err", err)
		os.Exit(1)
	}
}
