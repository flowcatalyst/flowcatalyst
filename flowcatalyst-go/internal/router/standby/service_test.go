package standby

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Enabled {
		t.Error("Default config should have Enabled=false")
	}

	if config.LockKey != "flowcatalyst:router:leader" {
		t.Errorf("Expected lock key 'flowcatalyst:router:leader', got %s", config.LockKey)
	}

	if config.LockTTL != 30*time.Second {
		t.Errorf("Expected lock TTL 30s, got %v", config.LockTTL)
	}

	if config.RefreshInterval != 10*time.Second {
		t.Errorf("Expected refresh interval 10s, got %v", config.RefreshInterval)
	}
}

func TestNewService(t *testing.T) {
	config := &Config{
		Enabled:         true,
		LockKey:         "test:lock",
		LockTTL:         10 * time.Second,
		RefreshInterval: 5 * time.Second,
	}

	svc := NewService(config, nil)

	if svc == nil {
		t.Fatal("NewService returned nil")
	}

	if svc.config != config {
		t.Error("Service should store the config")
	}

	if svc.instanceID == "" {
		t.Error("Service should have an instance ID")
	}
}

func TestNewService_CustomInstanceID(t *testing.T) {
	config := &Config{
		Enabled:    true,
		InstanceID: "my-custom-instance",
	}

	svc := NewService(config, nil)

	if svc.instanceID != "my-custom-instance" {
		t.Errorf("Expected instance ID 'my-custom-instance', got %s", svc.instanceID)
	}
}

func TestNewService_NilConfig(t *testing.T) {
	svc := NewService(nil, nil)

	if svc == nil {
		t.Fatal("NewService returned nil with nil config")
	}

	if svc.config == nil {
		t.Error("Service should have default config")
	}
}

func TestService_StartStop_Disabled(t *testing.T) {
	config := &Config{
		Enabled: false,
	}

	svc := NewService(config, nil)

	if err := svc.Start(); err != nil {
		t.Errorf("Start should not return error: %v", err)
	}

	// Disabled service should immediately be PRIMARY
	if !svc.IsPrimary() {
		t.Error("Disabled service should be PRIMARY")
	}

	svc.Stop()
}

func TestService_StartStop_Enabled_NoProvider(t *testing.T) {
	config := &Config{
		Enabled:         true,
		LockKey:         "test:lock",
		LockTTL:         100 * time.Millisecond,
		RefreshInterval: 50 * time.Millisecond,
	}

	callbackCalled := false
	callbacks := &Callbacks{
		OnBecomePrimary: func() {
			callbackCalled = true
		},
	}

	svc := NewService(config, callbacks)

	if err := svc.Start(); err != nil {
		t.Errorf("Start should not return error: %v", err)
	}

	// Wait for leader election loop to run
	time.Sleep(100 * time.Millisecond)

	// Without a lock provider, should default to PRIMARY
	if !svc.IsPrimary() {
		t.Error("Service without lock provider should be PRIMARY")
	}

	if !callbackCalled {
		t.Error("OnBecomePrimary callback should have been called")
	}

	svc.Stop()
}

func TestService_IsEnabled(t *testing.T) {
	enabledConfig := &Config{Enabled: true}
	disabledConfig := &Config{Enabled: false}

	enabledSvc := NewService(enabledConfig, nil)
	disabledSvc := NewService(disabledConfig, nil)

	if !enabledSvc.IsEnabled() {
		t.Error("Should return true for enabled service")
	}

	if disabledSvc.IsEnabled() {
		t.Error("Should return false for disabled service")
	}
}

func TestService_GetStatus(t *testing.T) {
	config := &Config{
		Enabled:    true,
		InstanceID: "test-instance",
	}

	svc := NewService(config, nil)

	status := svc.GetStatus()

	if status == nil {
		t.Fatal("GetStatus returned nil")
	}

	if !status.StandbyEnabled {
		t.Error("Status should show standby enabled")
	}

	if status.InstanceID != "test-instance" {
		t.Errorf("Expected instance ID 'test-instance', got %s", status.InstanceID)
	}
}

func TestService_GetInstanceID(t *testing.T) {
	config := &Config{
		InstanceID: "my-instance",
	}

	svc := NewService(config, nil)

	if svc.GetInstanceID() != "my-instance" {
		t.Errorf("Expected 'my-instance', got %s", svc.GetInstanceID())
	}
}

func TestService_GetRole(t *testing.T) {
	svc := NewService(nil, nil)

	// Initially unknown
	if svc.GetRole() != RoleUnknown {
		t.Errorf("Expected UNKNOWN role, got %s", svc.GetRole())
	}

	// After start (disabled mode), should be PRIMARY
	svc.Start()
	defer svc.Stop()

	if svc.GetRole() != RolePrimary {
		t.Errorf("Expected PRIMARY role after start, got %s", svc.GetRole())
	}
}

func TestService_WithNoOpLockProvider(t *testing.T) {
	config := &Config{
		Enabled:         true,
		LockKey:         "test:lock",
		LockTTL:         100 * time.Millisecond,
		RefreshInterval: 50 * time.Millisecond,
	}

	svc := NewService(config, nil)
	svc.SetLockProvider(NewNoOpLockProvider("test-instance"))

	if err := svc.Start(); err != nil {
		t.Errorf("Start should not return error: %v", err)
	}

	// Wait for election
	time.Sleep(100 * time.Millisecond)

	// NoOp provider always succeeds, so should be PRIMARY
	if !svc.IsPrimary() {
		t.Error("Should be PRIMARY with NoOp lock provider")
	}

	if svc.IsStandby() {
		t.Error("Should not be STANDBY with NoOp lock provider")
	}

	svc.Stop()
}

func TestNoOpLockProvider(t *testing.T) {
	provider := NewNoOpLockProvider("test-instance")
	ctx := context.Background()

	// TryAcquire always succeeds
	acquired, err := provider.TryAcquire(ctx, "key", "instance", time.Second)
	if err != nil {
		t.Errorf("TryAcquire error: %v", err)
	}
	if !acquired {
		t.Error("TryAcquire should always return true")
	}

	// Refresh always succeeds
	refreshed, err := provider.Refresh(ctx, "key", "instance", time.Second)
	if err != nil {
		t.Errorf("Refresh error: %v", err)
	}
	if !refreshed {
		t.Error("Refresh should always return true")
	}

	// Release never fails
	if err := provider.Release(ctx, "key", "instance"); err != nil {
		t.Errorf("Release error: %v", err)
	}

	// GetHolder returns this instance
	holder, err := provider.GetHolder(ctx, "key")
	if err != nil {
		t.Errorf("GetHolder error: %v", err)
	}
	if holder != "test-instance" {
		t.Errorf("Expected holder 'test-instance', got %s", holder)
	}

	// IsAvailable always true
	if !provider.IsAvailable(ctx) {
		t.Error("IsAvailable should always return true")
	}

	// Close never fails
	if err := provider.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestService_Callbacks(t *testing.T) {
	config := &Config{
		Enabled:         false, // Disabled so it goes directly to PRIMARY
	}

	primaryCalled := false
	standbyCalled := false

	callbacks := &Callbacks{
		OnBecomePrimary: func() {
			primaryCalled = true
		},
		OnBecomeStandby: func() {
			standbyCalled = true
		},
	}

	svc := NewService(config, callbacks)

	svc.Start()
	defer svc.Stop()

	if !primaryCalled {
		t.Error("OnBecomePrimary should have been called")
	}

	if standbyCalled {
		t.Error("OnBecomeStandby should not have been called")
	}
}

func TestRoleConstants(t *testing.T) {
	if RolePrimary != "PRIMARY" {
		t.Errorf("Expected 'PRIMARY', got %s", RolePrimary)
	}

	if RoleStandby != "STANDBY" {
		t.Errorf("Expected 'STANDBY', got %s", RoleStandby)
	}

	if RoleUnknown != "UNKNOWN" {
		t.Errorf("Expected 'UNKNOWN', got %s", RoleUnknown)
	}
}
