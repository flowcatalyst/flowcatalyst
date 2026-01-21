package lifecycle

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Run starts services and blocks until shutdown signal is received.
// This is the standard "main loop" for FlowCatalyst binaries.
//
// Usage:
//
//	lifecycle.Run(ctx, routerService, httpServer)
func Run(ctx context.Context, services ...Service) error {
	// Create cancellable context for shutdown
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Set up signal handler
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Create and start supervisor
	supervisor := NewSupervisor(services...)

	// Run supervisor in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- supervisor.Run(ctx)
	}()

	// Wait for shutdown signal or supervisor error
	select {
	case sig := <-quit:
		slog.Info("Shutdown signal received", "signal", sig)
		cancel()
	case err := <-errCh:
		if err != nil {
			slog.Error("Supervisor error", "error", err)
			return err
		}
	}

	// Wait for supervisor to complete shutdown
	select {
	case err := <-errCh:
		return err
	case <-time.After(35 * time.Second):
		slog.Error("Shutdown timed out")
		return nil
	}
}

// HTTPService wraps an http.Server as a Service.
type HTTPService struct {
	server *http.Server
	name   string
}

// NewHTTPService creates a Service from an http.Server.
func NewHTTPService(name string, server *http.Server) *HTTPService {
	return &HTTPService{
		server: server,
		name:   name,
	}
}

func (s *HTTPService) Name() string { return s.name }

func (s *HTTPService) Start(ctx context.Context) error {
	slog.Info("Starting HTTP server", "addr", s.server.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait briefly for immediate startup failures
	select {
	case err := <-errCh:
		return err
	case <-time.After(100 * time.Millisecond):
		// Server started successfully
	}

	// Block until context is cancelled
	<-ctx.Done()
	return nil
}

func (s *HTTPService) Stop(ctx context.Context) error {
	slog.Info("Stopping HTTP server")
	return s.server.Shutdown(ctx)
}

func (s *HTTPService) Health() error {
	// HTTP server is healthy if it's running (no simple way to check)
	return nil
}
