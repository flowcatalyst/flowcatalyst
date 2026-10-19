// Command fcrunner-echo is the in-repo echo test runner (internal/functions/
// echorunner): a Go v1 control-protocol client for the made-up runtime "echo".
// It exists to exercise the generic runner plumbing end to end; it runs no
// functions. Configuration uses the same names as the Go runner:
// FC_FUNCTIONS_PLATFORM_URL, FC_FUNCTIONS_CLIENT_ID, FC_FUNCTIONS_CLIENT_SECRET,
// FC_FUNCTIONS_POOL (default "default"), FC_FUNCTIONS_PORT (default 8096).
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/echorunner"
	fcauth "github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/auth"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--protocol" {
		_, _ = os.Stdout.WriteString("1\n")
		return
	}
	platform := os.Getenv("FC_FUNCTIONS_PLATFORM_URL")
	id, secret := os.Getenv("FC_FUNCTIONS_CLIENT_ID"), os.Getenv("FC_FUNCTIONS_CLIENT_SECRET")
	if platform == "" || id == "" || secret == "" {
		slog.Error("FC_FUNCTIONS_PLATFORM_URL, FC_FUNCTIONS_CLIENT_ID and FC_FUNCTIONS_CLIENT_SECRET are required")
		os.Exit(2)
	}
	pool := os.Getenv("FC_FUNCTIONS_POOL")
	if pool == "" {
		pool = "default"
	}
	port := os.Getenv("FC_FUNCTIONS_PORT")
	if port == "" {
		port = "8096"
	}

	tokens := fcauth.NewClientCredentialsProvider(fcauth.ClientCredentialsConfig{IssuerURL: platform, ClientID: id, ClientSecret: secret})
	client := control.NewClient(platform, tokens.Token, nil)
	client.Runtimes = []string{echorunner.Runtime}
	r := echorunner.New(echorunner.Config{Plane: client, Pool: pool, Version: "dev"})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: r.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("listener failed", "err", err)
			stop()
		}
	}()
	_ = r.Run(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdown)
}
