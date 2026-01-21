package traffic

import "errors"

// ErrTrafficManagement represents a traffic management operation failure
var ErrTrafficManagement = errors.New("traffic management error")

// TrafficStatus represents status information for monitoring/debugging
type TrafficStatus struct {
	StrategyType  string `json:"strategyType"`
	Registered    bool   `json:"registered"`
	TargetInfo    string `json:"targetInfo"`
	LastOperation string `json:"lastOperation"`
	LastError     string `json:"lastError,omitempty"`
}

// Strategy is the interface for managing traffic routing to this instance.
// Different deployment environments can implement this to control
// whether the load balancer routes traffic to this instance based
// on its PRIMARY/STANDBY role.
//
// Implementations should be:
// - Idempotent (safe to call multiple times)
// - Non-blocking (use async operations if needed)
// - Gracefully degrading (failures should log but not crash)
type Strategy interface {
	// RegisterAsActive registers this instance as active with the load balancer.
	// Called when instance becomes PRIMARY.
	RegisterAsActive() error

	// DeregisterFromActive deregisters this instance from the load balancer.
	// Called when instance becomes STANDBY or shuts down.
	DeregisterFromActive() error

	// IsRegistered checks if this instance is currently registered with the load balancer.
	IsRegistered() bool

	// GetStatus returns the current status for monitoring/debugging.
	GetStatus() *TrafficStatus
}
