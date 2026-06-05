// Command fc-dev is the FlowCatalyst developer monolith — a single
// binary that runs every subsystem against an embedded Postgres so
// engineers can iterate without Docker. Mirrors the Rust fc-dev's
// subcommand surface:
//
//	fc-dev start  — run the dev monolith (default; matches `fc-dev` no-arg).
//	fc-dev init   — bootstrap admin user + default tenant + .env file.
//	fc-dev fresh  — truncate every FlowCatalyst table (preserves schema).
//	fc-dev mcp     — run the FlowCatalyst MCP server.
//	fc-dev outbox  — standalone outbox poller for external apps.
//	fc-dev upgrade — self-update to the latest GitHub release.
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
		Use:   "fc-dev",
		Short: "FlowCatalyst Development Monolith — all components in one binary",
		Long: `fc-dev runs every FlowCatalyst subsystem in one process against an
embedded Postgres database. Designed for local development: no Docker,
no docker-compose, no separate migration step.

Invoking ` + "`fc-dev`" + ` with no subcommand is identical to ` + "`fc-dev start`" + `.`,
		// No-arg invocation runs start. Matches the Rust fc-dev UX.
		RunE: runStart,
		// Runtime failures (port in use, DB unreachable) shouldn't trigger
		// cobra's "did you mean…" help dump — that noise hides the real
		// slog.Error line. We log + exit ourselves in main.
		SilenceUsage:  true,
		SilenceErrors: true,
		// `fc-dev --version` prints the same string as `fc-dev version` (users
		// reach for the flag reflexively). The subcommand stays for parity with
		// the Rust CLI's surface.
		Version: version() + vcsSuffix(),
	}
	root.SetVersionTemplate("fc-dev {{.Version}}\n")
	// Mirror start's flag set onto the root command so `fc-dev` and
	// `fc-dev start` accept the same options.
	addStartFlags(root)

	root.AddCommand(newStartCmd())
	root.AddCommand(newInitCmd())
	root.AddCommand(newFreshCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newOutboxCmd())
	root.AddCommand(newUpgradeCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		slog.Error("fc-dev exited with error", "err", err)
		os.Exit(1)
	}
}
