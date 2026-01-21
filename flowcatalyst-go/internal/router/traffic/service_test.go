package traffic

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Enabled {
		t.Error("Default config should have Enabled=false")
	}

	if config.Strategy != "noop" {
		t.Errorf("Expected default strategy 'noop', got %s", config.Strategy)
	}
}

func TestNewService(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	if svc.config != config {
		t.Error("Service should store the config")
	}
}

func TestNewService_NilConfig(t *testing.T) {
	svc := NewService(nil)

	if svc == nil {
		t.Fatal("NewService returned nil with nil config")
	}

	if svc.config == nil {
		t.Error("Service should have default config")
	}
}

func TestService_DisabledUsesNoOp(t *testing.T) {
	config := &Config{
		Enabled:  false,
		Strategy: "noop",
	}

	svc := NewService(config)

	if svc.activeStrategy == nil {
		t.Error("Should have an active strategy even when disabled")
	}

	// Should use NoOp strategy
	if _, ok := svc.activeStrategy.(*NoOpStrategy); !ok {
		t.Error("Disabled service should use NoOpStrategy")
	}
}

func TestService_RegisterAsActive(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	// Should not panic with NoOp strategy
	svc.RegisterAsActive()

	if !svc.IsRegistered() {
		t.Error("NoOp strategy should report as registered")
	}
}

func TestService_DeregisterFromActive(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	// Register first
	svc.RegisterAsActive()

	// Then deregister
	svc.DeregisterFromActive()

	// NoOp still reports as registered
	if !svc.IsRegistered() {
		t.Error("NoOp strategy should still report as registered after deregister")
	}
}

func TestService_IsEnabled(t *testing.T) {
	enabledConfig := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	disabledConfig := &Config{
		Enabled:  false,
		Strategy: "noop",
	}

	enabledSvc := NewService(enabledConfig)
	disabledSvc := NewService(disabledConfig)

	if !enabledSvc.IsEnabled() {
		t.Error("Should return true for enabled service")
	}

	if disabledSvc.IsEnabled() {
		t.Error("Should return false for disabled service")
	}
}

func TestService_GetStatus(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	status := svc.GetStatus()

	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	if status.StrategyType != "noop" {
		t.Errorf("Expected strategy type 'noop', got %s", status.StrategyType)
	}
}

func TestService_UnknownStrategy(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "unknown-strategy",
	}

	svc := NewService(config)

	// Should fall back to NoOp
	if _, ok := svc.activeStrategy.(*NoOpStrategy); !ok {
		t.Error("Unknown strategy should fall back to NoOpStrategy")
	}
}

func TestService_SetStrategy(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	// Create a custom mock strategy
	mockStrategy := &MockStrategy{
		registered: true,
	}

	svc.SetStrategy(mockStrategy)

	if svc.IsRegistered() != true {
		t.Error("Should use the new strategy's IsRegistered")
	}
}

func TestService_NilStrategy(t *testing.T) {
	config := &Config{
		Enabled:  true,
		Strategy: "noop",
	}

	svc := NewService(config)

	// Force nil strategy
	svc.mu.Lock()
	svc.activeStrategy = nil
	svc.mu.Unlock()

	// Should not panic
	svc.RegisterAsActive()
	svc.DeregisterFromActive()

	if svc.IsRegistered() {
		t.Error("Nil strategy should report not registered")
	}

	status := svc.GetStatus()
	if status.StrategyType != "uninitialized" {
		t.Error("Nil strategy should report uninitialized")
	}
}

// MockStrategy is a mock implementation of Strategy for testing
type MockStrategy struct {
	registered    bool
	registerErr   error
	deregisterErr error
}

func (m *MockStrategy) RegisterAsActive() error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.registered = true
	return nil
}

func (m *MockStrategy) DeregisterFromActive() error {
	if m.deregisterErr != nil {
		return m.deregisterErr
	}
	m.registered = false
	return nil
}

func (m *MockStrategy) IsRegistered() bool {
	return m.registered
}

func (m *MockStrategy) GetStatus() *TrafficStatus {
	return &TrafficStatus{
		StrategyType: "mock",
		Registered:   m.registered,
	}
}
