package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newMCPCmd is the fc-dev mcp subcommand. The Rust version exec's
// directly into the fc-mcp library; the Go side keeps cmd/fc-mcp-server
// as the canonical entry point, so this subcommand is a friendly
// shortcut that documents the equivalent invocation.
//
// TODO(mcp-runtime): once the MCP server library is pinned (mark3labs/
// mcp-go or equivalent), call mcp.RunHTTP(opts) here directly so the
// dev binary doesn't need a second executable on PATH.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the FlowCatalyst MCP server (stub — exec fc-mcp-server instead)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("fc-dev mcp is not yet wired — run cmd/fc-mcp-server directly; see TODO(mcp-runtime) in cmd/fc-dev/mcp.go")
		},
	}
	cmd.Flags().Bool("http", false, "run as a streamable HTTP server instead of stdio")
	cmd.Flags().String("bind", "127.0.0.1:3100", "bind address for --http mode")
	return cmd
}
