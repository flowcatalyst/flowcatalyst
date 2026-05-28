package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/mcp"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/client"
)

// newMCPCmd runs the FlowCatalyst MCP HTTP server as a standalone process
// pointed at an external (non-embedded) platform. For the normal dev
// workflow `fc-dev start --mcp` boots MCP alongside everything else in
// one process; this subcommand is for plugging an MCP client at a
// remote platform from a developer laptop.
func newMCPCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "Run the FlowCatalyst MCP HTTP server against a remote platform",
		RunE:  runMCP,
	}
	cmd.Flags().String("bind", envStrDefault("FC_MCP_BIND", "127.0.0.1:8090"), "bind address for the MCP listener")
	cmd.Flags().String("platform-url", envStrDefault("FLOWCATALYST_URL", ""), "FlowCatalyst platform URL (required)")
	cmd.Flags().String("client-id", envStrDefault("FLOWCATALYST_CLIENT_ID", ""), "OAuth client_id")
	cmd.Flags().String("client-secret", envStrDefault("FLOWCATALYST_CLIENT_SECRET", ""), "OAuth client_secret / bearer token")
	return cmd
}

func runMCP(cmd *cobra.Command, _ []string) error {
	bind, _ := cmd.Flags().GetString("bind")
	platformURL, _ := cmd.Flags().GetString("platform-url")
	_, _ = cmd.Flags().GetString("client-id")
	clientSecret, _ := cmd.Flags().GetString("client-secret")
	if platformURL == "" {
		return errors.New("--platform-url (or FLOWCATALYST_URL) is required")
	}

	pc := client.New(platformURL,
		client.WithToken(clientSecret),
		client.WithTimeout(10*time.Second),
	)
	srv := mcp.New(pc)

	r := chi.NewRouter()
	r.Post("/mcp", srv.HandleHTTP)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	httpSrv := &http.Server{Addr: bind, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("fc-dev mcp listening", "addr", bind, "platform_url", platformURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-stop:
		slog.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("mcp listener: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}
