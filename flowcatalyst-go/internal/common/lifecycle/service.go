// Package lifecycle provides infrastructure for managing application services
// with coordinated startup, shutdown, and health monitoring.
//
// This follows the "structured monolith" pattern - each major component
// (Router, Scheduler, API, Outbox) implements the Service interface,
// allowing them to be supervised, tested, and controlled independently.
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"log/slog"
)

// Service represents a startable/stoppable component.
// Each major application component should implement this interface.
type Service interface {
	// Name returns the service identifier for logging
	Name() string

	// Start begins the service. It should block until ctx is cancelled
	// or return an error if startup fails.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the service.
	// Should complete within the given timeout.
	Stop(ctx context.Context) error

	// Health returns nil if the service is healthy, error otherwise.
	// Used by supervisors and health endpoints.
	Health() error
}

// Supervisor manages multiple services with coordinated lifecycle.
type Supervisor struct {
	services []Service
	mu       sync.RWMutex
	running  bool
}

// NewSupervisor creates a supervisor for the given services.
func NewSupervisor(services ...Service) *Supervisor {
	return &Supervisor{
		services: services,
	}
}

// Run starts all services and blocks until ctx is cancelled.
// Services are started in order and stopped in reverse order.
func (s *Supervisor) Run(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("supervisor already running")
	}
	s.running = true
	s.mu.Unlock()

	// Start services in order
	var startedServices []Service
	for _, svc := range s.services {
		slog.Info("Starting service", "service", svc.Name())

		// Start service in background
		errCh := make(chan error, 1)
		go func(service Service) {
			errCh <- service.Start(ctx)
		}(svc)

		// Wait briefly for immediate startup failures
		select {
		case err := <-errCh:
			if err != nil {
				// Startup failed - stop already started services
				s.stopServices(startedServices)
				return fmt.Errorf("service %s failed to start: %w", svc.Name(), err)
			}
		case <-time.After(100 * time.Millisecond):
			// Service started (or is starting async) - continue
		}

		startedServices = append(startedServices, svc)
		slog.Info("Service started", "service", svc.Name())
	}

	// Wait for context cancellation
	<-ctx.Done()
	slog.Info("Shutdown signal received, stopping services")

	// Stop services in reverse order
	s.stopServices(startedServices)

	s.mu.Lock()
	s.running = false
	s.mu.Unlock()

	return nil
}

// stopServices stops services in reverse order
func (s *Supervisor) stopServices(services []Service) {
	for i := len(services) - 1; i >= 0; i-- {
		svc := services[i]
		slog.Info("Stopping service", "service", svc.Name())

		stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		if err := svc.Stop(stopCtx); err != nil {
			slog.Error("Service stop error", "service", svc.Name(), "error", err)
		} else {
			slog.Info("Service stopped", "service", svc.Name())
		}
		cancel()
	}
}

// Health returns the health status of all services.
// Returns nil only if ALL services are healthy.
func (s *Supervisor) Health() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, svc := range s.services {
		if err := svc.Health(); err != nil {
			return fmt.Errorf("service %s unhealthy: %w", svc.Name(), err)
		}
	}
	return nil
}

// ServiceFunc adapts a simple function to the Service interface.
// Useful for wrapping goroutines that don't need complex lifecycle.
type ServiceFunc struct {
	name      string
	startFunc func(ctx context.Context) error
	stopFunc  func(ctx context.Context) error
	healthFn  func() error
}

// NewServiceFunc creates a Service from functions.
func NewServiceFunc(name string, start func(ctx context.Context) error, stop func(ctx context.Context) error) *ServiceFunc {
	return &ServiceFunc{
		name:      name,
		startFunc: start,
		stopFunc:  stop,
		healthFn:  func() error { return nil },
	}
}

func (s *ServiceFunc) Name() string                       { return s.name }
func (s *ServiceFunc) Start(ctx context.Context) error    { return s.startFunc(ctx) }
func (s *ServiceFunc) Stop(ctx context.Context) error     { return s.stopFunc(ctx) }
func (s *ServiceFunc) Health() error                      { return s.healthFn() }
func (s *ServiceFunc) WithHealth(fn func() error) *ServiceFunc {
	s.healthFn = fn
	return s
}
