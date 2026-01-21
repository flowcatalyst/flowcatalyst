package standby

import (
	"context"
	"time"
)

// NoOpLockProvider is a no-op implementation of LockProvider
// Used when standby mode is disabled or no distributed lock is needed.
// Always reports as lock holder (standalone mode).
type NoOpLockProvider struct {
	instanceID string
}

// NewNoOpLockProvider creates a new no-op lock provider
func NewNoOpLockProvider(instanceID string) *NoOpLockProvider {
	return &NoOpLockProvider{
		instanceID: instanceID,
	}
}

// TryAcquire always succeeds in no-op mode
func (p *NoOpLockProvider) TryAcquire(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error) {
	return true, nil
}

// Refresh always succeeds in no-op mode
func (p *NoOpLockProvider) Refresh(ctx context.Context, key, instanceID string, ttl time.Duration) (bool, error) {
	return true, nil
}

// Release is a no-op
func (p *NoOpLockProvider) Release(ctx context.Context, key, instanceID string) error {
	return nil
}

// GetHolder returns this instance as the holder
func (p *NoOpLockProvider) GetHolder(ctx context.Context, key string) (string, error) {
	return p.instanceID, nil
}

// IsAvailable always returns true in no-op mode
func (p *NoOpLockProvider) IsAvailable(ctx context.Context) bool {
	return true
}

// Close is a no-op
func (p *NoOpLockProvider) Close() error {
	return nil
}
