package main

import (
	"errors"

	"github.com/spf13/cobra"
)

// newOutboxCmd is the fc-dev outbox subcommand. Mirrors fc-dev outbox
// in Rust: a standalone outbox poller for external apps whose database
// can't be the embedded one (e.g. PostGIS in Docker).
//
// TODO(outbox-runtime): wire internal/outbox processor here. For now
// we point at cmd/fc-outbox-processor.
func newOutboxCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "outbox",
		Short: "Standalone outbox poller (stub — exec fc-outbox-processor instead)",
		RunE: func(_ *cobra.Command, _ []string) error {
			return errors.New("fc-dev outbox is not yet wired — run cmd/fc-outbox-processor directly; see TODO(outbox-runtime) in cmd/fc-dev/outbox.go")
		},
	}
	cmd.Flags().String("source-db-url", "", "external app's Postgres URL (required)")
	cmd.Flags().String("target-url", "http://localhost:3000", "FlowCatalyst platform URL")
	cmd.Flags().String("client-id", "", "OAuth client_id for forwarding")
	cmd.Flags().String("client-secret", "", "OAuth client_secret for forwarding")
	return cmd
}
