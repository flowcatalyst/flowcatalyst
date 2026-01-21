// Package standby provides high-availability standby/failover functionality
// using distributed locking for leader election.
//
// In HA mode, multiple instances compete for a distributed lock. The instance
// holding the lock is the PRIMARY and actively processes messages. Other
// instances are in STANDBY mode and will take over if the PRIMARY fails.
package standby

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"go.flowcatalyst.tech/internal/router/health"
)

// Role represents the current role of this instance
type Role string

const (
	// RolePrimary indicates this instance is the active leader
	RolePrimary Role = "PRIMARY"

	// RoleStandby indicates this instance is waiting to become leader
	RoleStandby Role = "STANDBY"

	// RoleUnknown indicates the role has not been determined yet
	RoleUnknown Role = "UNKNOWN"
)

// Config holds standby mode configuration
type Config struct {
	// Enabled controls whether standby mode is active
	Enabled bool

	// InstanceID is a unique identifier for this instance (auto-generated if empty)
	InstanceID string

	// LockKey is the distributed lock key (default: "flowcatalyst:router:leader")
	LockKey string

	// LockTTL is how long the lock is held before it expires
	LockTTL time.Duration

	// RefreshInterval is how often to refresh the lock
	RefreshInterval time.Duration

	// RedisURL is the Redis connection URL
	RedisURL string
}

// DefaultConfig returns default standby configuration
func DefaultConfig() *Config {
	return &Config{
		Enabled:         false,
		LockKey:         "flowcatalyst:router:leader",
		LockTTL:         30 * time.Second,
		RefreshInterval: 10 * time.Second,
	}
}

// Callbacks defines the callbacks invoked on role changes
type Callbacks struct {
	// OnBecomePrimary is called when this instance becomes the PRIMARY
	OnBecomePrimary func()

	// OnBecomeStandby is called when this instance becomes STANDBY
	OnBecomeStandby func()
}

// Service provides high-availability leader election and standby management.
// Implements StandbyStatusGetter interface for monitoring.
type Service struct {
	mu sync.RWMutex

	config    *Config
	callbacks *Callbacks

	// Current state
	instanceID            string
	currentRole           Role
	redisAvailable        bool
	currentLockHolder     string
	lastSuccessfulRefresh time.Time
	hasWarning            bool
	warningMessage        string

	// Lock provider (Redis, etc.)
	lockProvider LockProvider

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// LockProvider interface for distributed lock implementations
type LockProvider interface {
	// TryAcquire attempts to acquire the lock. Returns true if acquired.
	TryAcquire(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error)

	// Refresh extends the lock TTL. Returns false if lock was lost.
	Refresh(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error)

	// Release releases the lock
	Release(ctx context.Context, key, instanceID string) error

	// GetHolder returns the current lock holder instance ID
	GetHolder(ctx context.Context, key string) (string, error)

	// IsAvailable checks if the lock provider is available
	IsAvailable(ctx context.Context) bool

	// Close closes the lock provider connection
	Close() error
}

// NewService creates a new standby service
func NewService(config *Config, callbacks *Callbacks) *Service {
	if config == nil {
		config = DefaultConfig()
	}

	instanceID := config.InstanceID
	if instanceID == "" {
		instanceID = uuid.New().String()
	}

	ctx, cancel := context.WithCancel(context.Background())

	svc := &Service{
		config:      config,
		callbacks:   callbacks,
		instanceID:  instanceID,
		currentRole: RoleUnknown,
		ctx:         ctx,
		cancel:      cancel,
	}

	return svc
}

// SetLockProvider sets the distributed lock provider
func (s *Service) SetLockProvider(provider LockProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lockProvider = provider
}

// Start begins the standby service leader election loop
func (s *Service) Start() error {
	if !s.config.Enabled {
		slog.Info("Standby mode disabled - running as standalone PRIMARY")
		s.mu.Lock()
		s.currentRole = RolePrimary
		s.mu.Unlock()

		if s.callbacks != nil && s.callbacks.OnBecomePrimary != nil {
			s.callbacks.OnBecomePrimary()
		}
		return nil
	}

	slog.Info("Starting standby service with leader election",
		"instanceId", s.instanceID,
		"lockKey", s.config.LockKey,
		"lockTTL", s.config.LockTTL,
		"refreshInterval", s.config.RefreshInterval)

	// Initial lock acquisition attempt
	s.tryAcquireOrRefresh()

	// Start the leader election loop
	s.wg.Add(1)
	go s.leaderElectionLoop()

	return nil
}

// Stop stops the standby service and releases any held lock
func (s *Service) Stop() {
	slog.Info("Stopping standby service", "instanceId", s.instanceID)

	s.cancel()
	s.wg.Wait()

	// Release lock if we're holding it
	s.mu.RLock()
	role := s.currentRole
	provider := s.lockProvider
	s.mu.RUnlock()

	if role == RolePrimary && provider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := provider.Release(ctx, s.config.LockKey, s.instanceID); err != nil {
			slog.Warn("Failed to release lock during shutdown", "error", err)
		} else {
			slog.Info("Released leader lock")
		}
	}

	// Close the lock provider
	if provider != nil {
		provider.Close()
	}
}

// leaderElectionLoop runs the leader election loop
func (s *Service) leaderElectionLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.tryAcquireOrRefresh()
		}
	}
}

// tryAcquireOrRefresh attempts to acquire or refresh the distributed lock
func (s *Service) tryAcquireOrRefresh() {
	s.mu.RLock()
	provider := s.lockProvider
	currentRole := s.currentRole
	s.mu.RUnlock()

	if provider == nil {
		slog.Warn("No lock provider configured - running as standalone")
		s.setRole(RolePrimary)
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// Check if Redis is available
	available := provider.IsAvailable(ctx)
	s.mu.Lock()
	s.redisAvailable = available
	s.mu.Unlock()

	if !available {
		slog.Warn("Redis not available - maintaining current role")
		s.setWarning("Redis unavailable")
		return
	}

	if currentRole == RolePrimary {
		// Try to refresh our lock
		refreshed, err := provider.Refresh(ctx, s.config.LockKey, s.instanceID, s.config.LockTTL)
		if err != nil {
			slog.Error("Error refreshing lock", "error", err)
			s.setWarning("Lock refresh error: " + err.Error())
			return
		}

		if refreshed {
			s.mu.Lock()
			s.lastSuccessfulRefresh = time.Now()
			s.hasWarning = false
			s.warningMessage = ""
			s.mu.Unlock()
			slog.Debug("Lock refreshed successfully")
		} else {
			// Lost the lock!
			slog.Warn("Lost leader lock - transitioning to STANDBY")
			s.setRole(RoleStandby)
			s.updateLockHolder(ctx, provider)
		}
	} else {
		// Try to acquire the lock
		acquired, err := provider.TryAcquire(ctx, s.config.LockKey, s.instanceID, s.config.LockTTL)
		if err != nil {
			slog.Error("Error acquiring lock", "error", err)
			s.setWarning("Lock acquisition error: " + err.Error())
			s.updateLockHolder(ctx, provider)
			return
		}

		if acquired {
			slog.Info("Acquired leader lock - transitioning to PRIMARY")
			s.setRole(RolePrimary)
			s.mu.Lock()
			s.lastSuccessfulRefresh = time.Now()
			s.currentLockHolder = s.instanceID
			s.hasWarning = false
			s.warningMessage = ""
			s.mu.Unlock()
		} else {
			s.updateLockHolder(ctx, provider)
			if currentRole == RoleUnknown {
				s.setRole(RoleStandby)
			}
		}
	}
}

// setRole sets the current role and invokes callbacks
func (s *Service) setRole(role Role) {
	s.mu.Lock()
	oldRole := s.currentRole
	s.currentRole = role
	s.mu.Unlock()

	if oldRole == role {
		return
	}

	slog.Info("Role changed",
		"instanceId", s.instanceID,
		"oldRole", string(oldRole),
		"newRole", string(role))

	if s.callbacks == nil {
		return
	}

	switch role {
	case RolePrimary:
		if s.callbacks.OnBecomePrimary != nil {
			s.callbacks.OnBecomePrimary()
		}
	case RoleStandby:
		if s.callbacks.OnBecomeStandby != nil {
			s.callbacks.OnBecomeStandby()
		}
	}
}

// setWarning sets a warning message
func (s *Service) setWarning(message string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hasWarning = true
	s.warningMessage = message
}

// updateLockHolder updates the current lock holder from Redis
func (s *Service) updateLockHolder(ctx context.Context, provider LockProvider) {
	holder, err := provider.GetHolder(ctx, s.config.LockKey)
	if err != nil {
		slog.Debug("Failed to get current lock holder", "error", err)
		return
	}

	s.mu.Lock()
	s.currentLockHolder = holder
	s.mu.Unlock()
}

// IsPrimary returns true if this instance is the current leader
func (s *Service) IsPrimary() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRole == RolePrimary
}

// IsStandby returns true if this instance is in standby mode
func (s *Service) IsStandby() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRole == RoleStandby
}

// GetRole returns the current role
func (s *Service) GetRole() Role {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentRole
}

// GetInstanceID returns the instance ID
func (s *Service) GetInstanceID() string {
	return s.instanceID
}

// IsEnabled returns whether standby mode is enabled
// Implements StandbyStatusGetter interface
func (s *Service) IsEnabled() bool {
	return s.config.Enabled
}

// GetStatus returns the current standby status for monitoring
// Implements StandbyStatusGetter interface
func (s *Service) GetStatus() *health.StandbyStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastRefresh string
	if !s.lastSuccessfulRefresh.IsZero() {
		lastRefresh = s.lastSuccessfulRefresh.Format(time.RFC3339)
	}

	return &health.StandbyStatus{
		StandbyEnabled:        s.config.Enabled,
		InstanceID:            s.instanceID,
		Role:                  string(s.currentRole),
		RedisAvailable:        s.redisAvailable,
		CurrentLockHolder:     s.currentLockHolder,
		LastSuccessfulRefresh: lastRefresh,
		HasWarning:            s.hasWarning,
	}
}
