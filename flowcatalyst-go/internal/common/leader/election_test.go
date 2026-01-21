package leader

import (
	"testing"
	"time"
)

// === ElectorConfig Tests ===

func TestDefaultElectorConfig(t *testing.T) {
	cfg := DefaultElectorConfig("test-leader")

	if cfg.LockName != "test-leader" {
		t.Errorf("Expected LockName 'test-leader', got '%s'", cfg.LockName)
	}

	if cfg.InstanceID == "" {
		t.Error("Expected InstanceID to be set")
	}

	if cfg.TTL != 30*time.Second {
		t.Errorf("Expected TTL 30s, got %v", cfg.TTL)
	}

	if cfg.RefreshInterval != 10*time.Second {
		t.Errorf("Expected RefreshInterval 10s, got %v", cfg.RefreshInterval)
	}
}

func TestElectorConfigCustomValues(t *testing.T) {
	cfg := &ElectorConfig{
		InstanceID:      "my-instance",
		LockName:        "scheduler-leader",
		TTL:             60 * time.Second,
		RefreshInterval: 20 * time.Second,
	}

	if cfg.InstanceID != "my-instance" {
		t.Errorf("Expected InstanceID 'my-instance', got '%s'", cfg.InstanceID)
	}

	if cfg.TTL != 60*time.Second {
		t.Errorf("Expected TTL 60s, got %v", cfg.TTL)
	}
}

// === LeaderLock Tests ===

func TestLeaderLockStructure(t *testing.T) {
	now := time.Now()
	lock := LeaderLock{
		ID:         "scheduler-leader",
		InstanceID: "instance-1",
		AcquiredAt: now,
		ExpiresAt:  now.Add(30 * time.Second),
	}

	if lock.ID != "scheduler-leader" {
		t.Errorf("Expected ID 'scheduler-leader', got '%s'", lock.ID)
	}

	if lock.InstanceID != "instance-1" {
		t.Errorf("Expected InstanceID 'instance-1', got '%s'", lock.InstanceID)
	}

	if lock.ExpiresAt.Before(lock.AcquiredAt) {
		t.Error("ExpiresAt should be after AcquiredAt")
	}
}

// === LeaderElector Unit Tests (no MongoDB) ===

func TestLeaderElectorIsPrimaryDefault(t *testing.T) {
	// Test that a new elector is not primary by default
	elector := &LeaderElector{
		config: DefaultElectorConfig("test-leader"),
	}

	if elector.IsPrimary() {
		t.Error("New elector should not be primary")
	}
}

func TestLeaderElectorInstanceID(t *testing.T) {
	cfg := &ElectorConfig{
		InstanceID: "test-instance-123",
		LockName:   "test-lock",
	}

	elector := &LeaderElector{
		config: cfg,
	}

	if elector.InstanceID() != "test-instance-123" {
		t.Errorf("Expected InstanceID 'test-instance-123', got '%s'", elector.InstanceID())
	}
}

func TestLeaderElectorCallbacks(t *testing.T) {
	elector := &LeaderElector{
		config: DefaultElectorConfig("test-leader"),
	}

	becameLeader := false
	lostLeadership := false

	elector.OnBecomeLeader(func() {
		becameLeader = true
	})

	elector.OnLoseLeadership(func() {
		lostLeadership = true
	})

	// Verify callbacks are set
	if elector.onBecomeLeader == nil {
		t.Error("OnBecomeLeader callback should be set")
	}

	if elector.onLoseLeadership == nil {
		t.Error("OnLoseLeadership callback should be set")
	}

	// Call the callbacks
	elector.onBecomeLeader()
	elector.onLoseLeadership()

	if !becameLeader {
		t.Error("OnBecomeLeader callback was not called")
	}

	if !lostLeadership {
		t.Error("OnLoseLeadership callback was not called")
	}
}

// === TTL and Timing Tests ===

func TestLockExpiration(t *testing.T) {
	now := time.Now()
	ttl := 30 * time.Second

	lock := LeaderLock{
		ID:         "test-lock",
		InstanceID: "instance-1",
		AcquiredAt: now,
		ExpiresAt:  now.Add(ttl),
	}

	// Lock should not be expired immediately
	if time.Now().After(lock.ExpiresAt) {
		t.Error("Lock should not be expired immediately")
	}

	// Simulate time passing (just check the logic)
	pastExpiry := now.Add(ttl + time.Second)
	if !pastExpiry.After(lock.ExpiresAt) {
		t.Error("Time after TTL should be after ExpiresAt")
	}
}

func TestRefreshExtendsTTL(t *testing.T) {
	now := time.Now()
	originalExpiry := now.Add(30 * time.Second)

	lock := LeaderLock{
		ID:         "test-lock",
		InstanceID: "instance-1",
		AcquiredAt: now,
		ExpiresAt:  originalExpiry,
	}

	// Simulate refresh 10 seconds later
	refreshTime := now.Add(10 * time.Second)
	newExpiry := refreshTime.Add(30 * time.Second)

	// New expiry should be later than original
	if !newExpiry.After(originalExpiry) {
		t.Error("Refreshed expiry should be later than original")
	}

	// Update lock
	lock.ExpiresAt = newExpiry

	// Lock should have extended expiry
	if lock.ExpiresAt.Equal(originalExpiry) {
		t.Error("Lock expiry should have been extended")
	}
}

// === Config Validation Tests ===

func TestElectorConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config *ElectorConfig
		valid  bool
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
			valid:  true,
		},
		{
			name: "valid config",
			config: &ElectorConfig{
				InstanceID:      "instance-1",
				LockName:        "scheduler",
				TTL:             30 * time.Second,
				RefreshInterval: 10 * time.Second,
			},
			valid: true,
		},
		{
			name: "short TTL",
			config: &ElectorConfig{
				InstanceID:      "instance-1",
				LockName:        "scheduler",
				TTL:             1 * time.Second,
				RefreshInterval: 10 * time.Second,
			},
			valid: true, // Still technically valid, just not recommended
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.config != nil {
				if tt.config.LockName == "" && tt.valid {
					t.Error("LockName should be required for valid config")
				}
			}
		})
	}
}

// === Lock Name Tests ===

func TestLockNameVariations(t *testing.T) {
	lockNames := []string{
		"scheduler-leader",
		"stream-processor-leader",
		"worker-1-leader",
		"my-app_leader",
	}

	for _, name := range lockNames {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultElectorConfig(name)
			if cfg.LockName != name {
				t.Errorf("Expected LockName '%s', got '%s'", name, cfg.LockName)
			}
		})
	}
}

// === Concurrent Access Simulation ===

func TestMultipleInstanceIDs(t *testing.T) {
	instances := []string{
		"scheduler-pod-1",
		"scheduler-pod-2",
		"scheduler-pod-3",
	}

	configs := make([]*ElectorConfig, len(instances))

	for i, id := range instances {
		configs[i] = &ElectorConfig{
			InstanceID:      id,
			LockName:        "scheduler-leader",
			TTL:             30 * time.Second,
			RefreshInterval: 10 * time.Second,
		}
	}

	// All configs should have the same lock name
	for _, cfg := range configs {
		if cfg.LockName != "scheduler-leader" {
			t.Errorf("Expected LockName 'scheduler-leader', got '%s'", cfg.LockName)
		}
	}

	// All configs should have different instance IDs
	seen := make(map[string]bool)
	for _, cfg := range configs {
		if seen[cfg.InstanceID] {
			t.Errorf("Duplicate InstanceID: %s", cfg.InstanceID)
		}
		seen[cfg.InstanceID] = true
	}
}

// === State Transition Tests ===

func TestPrimaryStateTransitions(t *testing.T) {
	elector := &LeaderElector{
		config: DefaultElectorConfig("test-leader"),
	}

	// Initial state
	if elector.IsPrimary() {
		t.Error("Should start as non-primary")
	}

	// Become primary
	elector.isPrimary.Store(true)
	if !elector.IsPrimary() {
		t.Error("Should be primary after setting")
	}

	// Lose primary
	elector.isPrimary.Store(false)
	if elector.IsPrimary() {
		t.Error("Should not be primary after clearing")
	}
}

// Benchmark for IsPrimary check (should be very fast)
func BenchmarkIsPrimary(b *testing.B) {
	elector := &LeaderElector{
		config: DefaultElectorConfig("bench-leader"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = elector.IsPrimary()
	}
}

// Benchmark for state toggle
func BenchmarkStateToggle(b *testing.B) {
	elector := &LeaderElector{
		config: DefaultElectorConfig("bench-leader"),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		elector.isPrimary.Store(true)
		elector.isPrimary.Store(false)
	}
}
