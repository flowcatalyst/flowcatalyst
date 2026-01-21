package traffic

import "log/slog"

// NoOpStrategy is a no-op traffic management strategy.
// Does nothing - maintains current behavior where both PRIMARY and STANDBY
// instances remain registered with the load balancer.
// This is the default strategy when traffic management is disabled or
// when no specific strategy is needed.
type NoOpStrategy struct{}

// NewNoOpStrategy creates a new no-op strategy
func NewNoOpStrategy() *NoOpStrategy {
	return &NoOpStrategy{}
}

// RegisterAsActive does nothing for no-op strategy
func (s *NoOpStrategy) RegisterAsActive() error {
	slog.Debug("NoOp strategy: registerAsActive() - no action taken")
	return nil
}

// DeregisterFromActive does nothing for no-op strategy
func (s *NoOpStrategy) DeregisterFromActive() error {
	slog.Debug("NoOp strategy: deregisterFromActive() - no action taken")
	return nil
}

// IsRegistered always returns true since we don't manage registration
func (s *NoOpStrategy) IsRegistered() bool {
	return true
}

// GetStatus returns the current status
func (s *NoOpStrategy) GetStatus() *TrafficStatus {
	return &TrafficStatus{
		StrategyType:  "noop",
		Registered:    true,
		TargetInfo:    "No traffic management - all instances receive traffic",
		LastOperation: "none",
		LastError:     "",
	}
}
