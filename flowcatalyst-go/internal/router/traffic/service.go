package traffic

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
)

// Config holds traffic management configuration
type Config struct {
	// Enabled controls whether traffic management is active
	Enabled bool

	// Strategy specifies which strategy to use (noop, aws-alb, etc.)
	Strategy string
}

// DefaultConfig returns default traffic management configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:  false,
		Strategy: "noop",
	}
}

// Service orchestrates traffic management strategies.
// Selects the appropriate strategy based on configuration and
// provides a unified interface for registering/deregistering
// this instance with load balancers.
//
// Handles errors gracefully - traffic management failures are logged
// but don't crash the application or affect standby mode operation.
type Service struct {
	mu sync.RWMutex

	config         *Config
	activeStrategy Strategy
	noOpStrategy   *NoOpStrategy
}

// NewService creates a new traffic management service
func NewService(config *Config) *Service {
	if config == nil {
		config = DefaultConfig()
	}

	svc := &Service{
		config:       config,
		noOpStrategy: NewNoOpStrategy(),
	}

	// Initialize and select the appropriate strategy
	svc.initStrategy()

	return svc
}

// initStrategy initializes and selects the appropriate strategy
func (s *Service) initStrategy() {
	if !s.config.Enabled {
		slog.Info("Traffic management disabled - using no-op strategy")
		s.activeStrategy = s.noOpStrategy
		return
	}

	strategyType := strings.ToLower(s.config.Strategy)
	slog.Info("Traffic management enabled", "strategy", strategyType)

	switch strategyType {
	case "noop":
		s.activeStrategy = s.noOpStrategy
		slog.Info("Using no-op traffic strategy")

	default:
		slog.Warn("Unknown traffic management strategy - using no-op", "strategy", strategyType)
		s.activeStrategy = s.noOpStrategy
	}
}

// RegisterAsActive registers this instance as active with the load balancer.
// Should be called when instance becomes PRIMARY.
// Failures are logged but don't throw errors - graceful degradation.
func (s *Service) RegisterAsActive() {
	s.mu.RLock()
	strategy := s.activeStrategy
	s.mu.RUnlock()

	if strategy == nil {
		slog.Warn("Traffic management strategy not initialized - skipping registration")
		return
	}

	slog.Info("Registering instance as active with load balancer")
	if err := strategy.RegisterAsActive(); err != nil {
		slog.Error("Failed to register instance with load balancer - Instance may receive traffic despite being STANDBY", "error", err)
		// Don't return error - allow standby mode to continue working
	}
}

// DeregisterFromActive deregisters this instance from the load balancer.
// Should be called when instance becomes STANDBY or shuts down.
// Failures are logged but don't throw errors - graceful degradation.
func (s *Service) DeregisterFromActive() {
	s.mu.RLock()
	strategy := s.activeStrategy
	s.mu.RUnlock()

	if strategy == nil {
		slog.Warn("Traffic management strategy not initialized - skipping deregistration")
		return
	}

	slog.Info("Deregistering instance from load balancer")
	if err := strategy.DeregisterFromActive(); err != nil {
		slog.Error("Failed to deregister instance from load balancer - Instance may continue receiving traffic despite being STANDBY", "error", err)
		// Don't return error - allow standby mode to continue working
	}
}

// IsRegistered checks if this instance is currently registered with the load balancer.
func (s *Service) IsRegistered() bool {
	s.mu.RLock()
	strategy := s.activeStrategy
	s.mu.RUnlock()

	if strategy == nil {
		return false
	}
	return strategy.IsRegistered()
}

// IsEnabled returns whether traffic management is enabled
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// GetStatus returns the current traffic management status for monitoring.
func (s *Service) GetStatus() *TrafficStatus {
	s.mu.RLock()
	strategy := s.activeStrategy
	s.mu.RUnlock()

	if strategy == nil {
		return &TrafficStatus{
			StrategyType:  "uninitialized",
			Registered:    false,
			TargetInfo:    "Strategy not initialized",
			LastOperation: "none",
			LastError:     "Strategy not initialized",
		}
	}
	return strategy.GetStatus()
}

// SetStrategy allows setting a custom strategy at runtime
func (s *Service) SetStrategy(strategy Strategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeStrategy = strategy
	slog.Info("Traffic strategy updated", "strategy", fmt.Sprintf("%T", strategy))
}
