package manager

import (
	"context"
	"sync"
)

// RouterService wraps a Router to implement lifecycle.Service interface.
// This enables coordinated startup/shutdown with other services.
type RouterService struct {
	router  *Router
	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// NewRouterService creates a service wrapper for the router.
func NewRouterService(router *Router) *RouterService {
	return &RouterService{
		router: router,
		stopCh: make(chan struct{}),
	}
}

// Name returns the service identifier.
func (s *RouterService) Name() string {
	return "message-router"
}

// Start begins message processing and blocks until ctx is cancelled.
func (s *RouterService) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	// Start the router
	s.router.Start()

	// Block until context cancelled or Stop called
	select {
	case <-ctx.Done():
	case <-s.stopCh:
	}

	return nil
}

// Stop gracefully stops message processing.
func (s *RouterService) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.router.Stop()
	s.running = false

	// Signal Start to return
	select {
	case <-s.stopCh:
		// Already closed
	default:
		close(s.stopCh)
	}

	return nil
}

// Health returns nil if the router is healthy.
func (s *RouterService) Health() error {
	// Router is healthy if it's running
	// Could be extended to check queue connectivity, etc.
	return nil
}

// Pause stops message processing but keeps connections alive.
// Used by standby service when becoming standby.
func (s *RouterService) Pause() {
	s.router.Stop()
}

// Resume starts message processing.
// Used by standby service when becoming primary.
func (s *RouterService) Resume() {
	s.router.Start()
}
