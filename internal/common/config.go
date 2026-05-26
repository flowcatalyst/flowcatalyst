package common

import "github.com/google/uuid"

// PoolConfig is the per-pool routing configuration.
type PoolConfig struct {
	Code               string `json:"code"`
	Concurrency        uint32 `json:"concurrency"`
	RateLimitPerMinute *uint32 `json:"rateLimitPerMinute,omitempty"`
}

// QueueConfig is the per-queue connection configuration.
type QueueConfig struct {
	Name              string `json:"name"`
	URI               string `json:"uri"`
	Connections       uint32 `json:"connections"`
	VisibilityTimeout uint32 `json:"visibilityTimeout"`
}

// RouterConfig is what the router fetches from its config source.
type RouterConfig struct {
	ProcessingPools []PoolConfig  `json:"processingPools"`
	Queues          []QueueConfig `json:"queues"`
}

// LeaderElectionConfig is the unified leader-election configuration
// shared by fc-outbox and fc-standby in Rust.
type LeaderElectionConfig struct {
	Enabled                    bool
	RedisURL                   string
	LockKey                    string
	LockTTLSeconds             uint64
	HeartbeatIntervalSeconds   uint64
	InstanceID                 string
}

// NewLeaderElectionConfig creates a config with sane defaults.
func NewLeaderElectionConfig(redisURL string) LeaderElectionConfig {
	return LeaderElectionConfig{
		Enabled:                  true,
		RedisURL:                 redisURL,
		LockKey:                  "fc:leader",
		LockTTLSeconds:           30,
		HeartbeatIntervalSeconds: 10,
		InstanceID:               uuid.NewString(),
	}
}

// StallConfig controls stall detection in the router.
type StallConfig struct {
	Enabled               bool   `json:"enabled"`
	StallThresholdSeconds uint64 `json:"stallThresholdSeconds"`
	ForceNackStalled      bool   `json:"forceNackStalled"`
	ForceNackAfterSeconds uint64 `json:"forceNackAfterSeconds"`
	NackDelaySeconds      uint32 `json:"nackDelaySeconds"`
}

// DefaultStallConfig matches the Rust defaults.
func DefaultStallConfig() StallConfig {
	return StallConfig{
		Enabled:               true,
		StallThresholdSeconds: 300,
		ForceNackStalled:      false,
		ForceNackAfterSeconds: 600,
		NackDelaySeconds:      30,
	}
}
