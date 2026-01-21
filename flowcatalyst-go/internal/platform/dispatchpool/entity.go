// Package dispatchpool provides dispatch pool configuration entities
package dispatchpool

import (
	"time"
)

// MediatorType defines the type of mediator for a dispatch pool
type MediatorType string

const (
	MediatorTypeHTTPWebhook MediatorType = "HTTP_WEBHOOK"
)

// DispatchPoolStatus represents the status of a dispatch pool
type DispatchPoolStatus string

const (
	DispatchPoolStatusActive    DispatchPoolStatus = "ACTIVE"
	DispatchPoolStatusSuspended DispatchPoolStatus = "SUSPENDED"
	DispatchPoolStatusArchived  DispatchPoolStatus = "ARCHIVED"
)

// DispatchPool represents a dispatch pool configuration
// Collection: dispatch_pools
type DispatchPool struct {
	ID               string             `bson:"_id" json:"id"`
	Code             string             `bson:"code" json:"code"`
	Name             string             `bson:"name,omitempty" json:"name,omitempty"`
	Description      string             `bson:"description,omitempty" json:"description,omitempty"`
	ClientID         string             `bson:"clientId,omitempty" json:"clientId,omitempty"`
	ClientIdentifier string             `bson:"clientIdentifier,omitempty" json:"clientIdentifier,omitempty"` // Denormalized client identifier
	MediatorType     MediatorType       `bson:"mediatorType" json:"mediatorType"`
	Concurrency      int                `bson:"concurrency" json:"concurrency"`
	QueueCapacity    int                `bson:"queueCapacity" json:"queueCapacity"`
	RateLimitPerMin  *int               `bson:"rateLimitPerMin,omitempty" json:"rateLimitPerMin,omitempty"`
	Status           DispatchPoolStatus `bson:"status" json:"status"`
	// Enabled is deprecated - use Status instead
	// Kept for backwards compatibility with older data
	Enabled   bool      `bson:"enabled,omitempty" json:"enabled,omitempty"`
	CreatedAt time.Time `bson:"createdAt" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt" json:"updatedAt"`
}

// IsActive returns true if the pool is active
func (p *DispatchPool) IsActive() bool {
	return p.Status == DispatchPoolStatusActive
}

// IsSuspended returns true if the pool is suspended
func (p *DispatchPool) IsSuspended() bool {
	return p.Status == DispatchPoolStatusSuspended
}

// IsArchived returns true if the pool is archived
func (p *DispatchPool) IsArchived() bool {
	return p.Status == DispatchPoolStatusArchived
}

// IsAnchorLevel returns true if this is an anchor-level pool (not client-specific)
func (p *DispatchPool) IsAnchorLevel() bool {
	return p.ClientID == ""
}

// IsEnabled returns true if the pool is enabled (active)
// Deprecated: Use IsActive() instead
func (p *DispatchPool) IsEnabled() bool {
	// Check new status field first, fall back to legacy Enabled field
	if p.Status != "" {
		return p.Status == DispatchPoolStatusActive
	}
	return p.Enabled
}

// IsHTTPWebhook returns true if the mediator type is HTTP_WEBHOOK
func (p *DispatchPool) IsHTTPWebhook() bool {
	return p.MediatorType == MediatorTypeHTTPWebhook
}

// GetConcurrencyOrDefault returns concurrency or default value
func (p *DispatchPool) GetConcurrencyOrDefault(defaultVal int) int {
	if p.Concurrency <= 0 {
		return defaultVal
	}
	return p.Concurrency
}

// GetQueueCapacityOrDefault returns queue capacity or default value
func (p *DispatchPool) GetQueueCapacityOrDefault(defaultVal int) int {
	if p.QueueCapacity <= 0 {
		return defaultVal
	}
	return p.QueueCapacity
}
