package subscription

import (
	"time"
)

// SubscriptionStatus defines the status of a subscription
type SubscriptionStatus string

const (
	SubscriptionStatusActive SubscriptionStatus = "ACTIVE"
	SubscriptionStatusPaused SubscriptionStatus = "PAUSED"
)

// SubscriptionSource defines how the subscription was created
type SubscriptionSource string

const (
	SubscriptionSourceAPI SubscriptionSource = "API"
	SubscriptionSourceUI  SubscriptionSource = "UI"
)

// DispatchMode defines how messages are processed for ordering
type DispatchMode string

const (
	DispatchModeImmediate    DispatchMode = "IMMEDIATE"      // No ordering guarantee
	DispatchModeNextOnError  DispatchMode = "NEXT_ON_ERROR"  // Skip failed, process next
	DispatchModeBlockOnError DispatchMode = "BLOCK_ON_ERROR" // Block group on failure
)

// Subscription represents a subscription to events
// Collection: subscriptions
type Subscription struct {
	ID               string             `bson:"_id" json:"id"`
	Code             string             `bson:"code" json:"code"` // Unique subscription code
	Name             string             `bson:"name" json:"name"`
	Description      string             `bson:"description,omitempty" json:"description,omitempty"`
	ClientID         string             `bson:"clientId,omitempty" json:"clientId,omitempty"` // Tenant isolation
	ClientIdentifier string             `bson:"clientIdentifier,omitempty" json:"clientIdentifier,omitempty"`
	EventTypes       []EventTypeBinding `bson:"eventTypes" json:"eventTypes"`
	Target           string             `bson:"target" json:"target"` // Webhook URL
	Queue            string             `bson:"queue,omitempty" json:"queue,omitempty"`
	CustomConfig     []ConfigEntry      `bson:"customConfig,omitempty" json:"customConfig,omitempty"`
	Source           SubscriptionSource `bson:"source" json:"source"`
	Status           SubscriptionStatus `bson:"status" json:"status"`
	MaxAgeSeconds    int                `bson:"maxAgeSeconds,omitempty" json:"maxAgeSeconds,omitempty"`
	DispatchPoolID   string             `bson:"dispatchPoolId,omitempty" json:"dispatchPoolId,omitempty"`
	DispatchPoolCode string             `bson:"dispatchPoolCode,omitempty" json:"dispatchPoolCode,omitempty"`
	DelaySeconds     int                `bson:"delaySeconds,omitempty" json:"delaySeconds,omitempty"`
	Sequence         int                `bson:"sequence,omitempty" json:"sequence,omitempty"`
	Mode             DispatchMode       `bson:"mode,omitempty" json:"mode,omitempty"`
	TimeoutSeconds   int                `bson:"timeoutSeconds,omitempty" json:"timeoutSeconds,omitempty"`
	MaxRetries       int                `bson:"maxRetries,omitempty" json:"maxRetries,omitempty"`         // Maximum retry attempts (default 3)
	ServiceAccountID string             `bson:"serviceAccountId,omitempty" json:"serviceAccountId,omitempty"` // For webhook credentials
	DataOnly         bool               `bson:"dataOnly" json:"dataOnly"`
	CreatedAt        time.Time          `bson:"createdAt" json:"createdAt"`
	UpdatedAt        time.Time          `bson:"updatedAt" json:"updatedAt"`
}

// Default values for Subscription
const (
	DefaultMaxAgeSeconds   = 86400 // 24 hours
	DefaultDelaySeconds    = 0
	DefaultSequence        = 99
	DefaultTimeoutSeconds  = 30
	DefaultMaxRetries      = 3
	DefaultDataOnly        = true
)

// EventTypeBinding binds an event type to a subscription
type EventTypeBinding struct {
	EventTypeID   string `bson:"eventTypeId" json:"eventTypeId"`
	EventTypeCode string `bson:"eventTypeCode" json:"eventTypeCode"`
	SpecVersion   string `bson:"specVersion,omitempty" json:"specVersion,omitempty"`
}

// ConfigEntry represents a custom configuration key-value pair
type ConfigEntry struct {
	Key   string `bson:"key" json:"key"`
	Value string `bson:"value" json:"value"`
}

// IsActive returns true if the subscription is active
func (s *Subscription) IsActive() bool {
	return s.Status == SubscriptionStatusActive
}

// IsPaused returns true if the subscription is paused
func (s *Subscription) IsPaused() bool {
	return s.Status == SubscriptionStatusPaused
}

// GetConfigValue returns the value for a config key
func (s *Subscription) GetConfigValue(key string) string {
	for _, c := range s.CustomConfig {
		if c.Key == key {
			return c.Value
		}
	}
	return ""
}

// MatchesEventType checks if the subscription matches an event type
func (s *Subscription) MatchesEventType(eventTypeCode string) bool {
	for _, et := range s.EventTypes {
		if et.EventTypeCode == eventTypeCode {
			return true
		}
	}
	return false
}
