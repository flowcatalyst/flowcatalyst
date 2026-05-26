// Command fc-mcp-server is the read-only MCP server. Default transport
// is streamable HTTP at :8090. Future stdio transport lands when the
// official mark3labs/mcp-go library is pinned (Phase 5+).
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/logging"
	"github.com/flowcatalyst/flowcatalyst-go/internal/mcp"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/client"
)

func main() {
	logging.Init()

	platformURL := os.Getenv("FLOWCATALYST_URL")
	if platformURL == "" {
		slog.Error("FLOWCATALYST_URL not set")
		os.Exit(1)
	}
	clientID := os.Getenv("FLOWCATALYST_CLIENT_ID")
	clientSecret := os.Getenv("FLOWCATALYST_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		slog.Warn("FLOWCATALYST_CLIENT_ID/SECRET not set; calls will be unauthenticated")
	}

	// TODO(phase-3d): exchange (clientID, clientSecret) for a bearer via
	// /oauth/token client_credentials; for now, pass clientSecret through.
	pc := client.New(platformURL,
		client.WithToken(clientSecret),
		client.WithTimeout(10*time.Second),
	)
	srv := mcp.New(pc)

	r := chi.NewRouter()
	r.Post("/mcp", srv.HandleHTTP)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	addr := ":" + envOr("FC_MCP_PORT", "8090")
	httpSrv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		slog.Info("fc-mcp-server listening", "addr", addr, "platform_url", platformURL)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server error", "err", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
