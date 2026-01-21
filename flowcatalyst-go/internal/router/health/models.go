package health

import (
	"time"
)

// InfrastructureHealth represents the result of an infrastructure health check
type InfrastructureHealth struct {
	Healthy bool     `json:"healthy"`
	Message string   `json:"message"`
	Issues  []string `json:"issues,omitempty"`
}

// ReadinessStatus represents Kubernetes liveness/readiness probe response
type ReadinessStatus struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Issues    []string  `json:"issues,omitempty"`
}

// NewHealthyStatus creates a healthy readiness status
func NewHealthyStatus(status string) *ReadinessStatus {
	return &ReadinessStatus{
		Status:    status,
		Timestamp: time.Now(),
		Issues:    []string{},
	}
}

// NewUnhealthyStatus creates an unhealthy readiness status with issues
func NewUnhealthyStatus(status string, issues []string) *ReadinessStatus {
	return &ReadinessStatus{
		Status:    status,
		Timestamp: time.Now(),
		Issues:    issues,
	}
}

// HealthStatus represents detailed system health for the monitoring dashboard
type HealthStatus struct {
	Status                   string       `json:"status"`
	UpSince                  time.Time    `json:"upSince"`
	TotalMessagesProcessed   int64        `json:"totalMessagesProcessed"`
	TotalMessagesSucceeded   int64        `json:"totalMessagesSucceeded"`
	TotalMessagesFailed      int64        `json:"totalMessagesFailed"`
	OverallSuccessRate       float64      `json:"overallSuccessRate"`
	ActivePoolCount          int          `json:"activePoolCount"`
	TotalActiveWorkers       int          `json:"totalActiveWorkers"`
	CurrentQueueDepth        int64        `json:"currentQueueDepth"`
	Throughput               float64      `json:"throughput"`
	CircuitBreakersOpen      int          `json:"circuitBreakersOpen"`
	UnacknowledgedWarnings   int          `json:"unacknowledgedWarnings"`
	InfrastructureHealth     string       `json:"infrastructureHealth"`
	LastInfrastructureCheck  time.Time    `json:"lastInfrastructureCheck"`
	BrokerType               string       `json:"brokerType"`
	BrokerConnected          bool         `json:"brokerConnected"`
	PoolHealth               []PoolHealth `json:"poolHealth,omitempty"`
}

// PoolHealth represents health status of a single processing pool
type PoolHealth struct {
	PoolCode           string    `json:"poolCode"`
	Status             string    `json:"status"`
	ActiveWorkers      int       `json:"activeWorkers"`
	QueueSize          int       `json:"queueSize"`
	LastActivityAt     time.Time `json:"lastActivityAt,omitempty"`
	CircuitBreakerOpen bool      `json:"circuitBreakerOpen"`
}

// QueueStats represents statistics for a queue
type QueueStats struct {
	Name               string  `json:"name"`
	TotalMessages      int64   `json:"totalMessages"`
	TotalConsumed      int64   `json:"totalConsumed"`
	TotalFailed        int64   `json:"totalFailed"`
	SuccessRate        float64 `json:"successRate"`
	CurrentSize        int64   `json:"currentSize"`
	Throughput         float64 `json:"throughput"`
	PendingMessages    int64   `json:"pendingMessages"`
	MessagesNotVisible int64   `json:"messagesNotVisible"`
}

// PoolStats represents statistics for a processing pool
type PoolStats struct {
	PoolCode                string  `json:"poolCode"`
	TotalProcessed          int64   `json:"totalProcessed"`
	TotalSucceeded          int64   `json:"totalSucceeded"`
	TotalFailed             int64   `json:"totalFailed"`
	TotalRateLimited        int64   `json:"totalRateLimited"`
	SuccessRate             float64 `json:"successRate"`
	ActiveWorkers           int     `json:"activeWorkers"`
	AvailablePermits        int     `json:"availablePermits"`
	MaxConcurrency          int     `json:"maxConcurrency"`
	QueueSize               int     `json:"queueSize"`
	MaxQueueCapacity        int     `json:"maxQueueCapacity"`
	AverageProcessingTimeMs float64 `json:"averageProcessingTimeMs"`
}

// CircuitBreakerStats represents statistics for a circuit breaker
type CircuitBreakerStats struct {
	Name            string  `json:"name"`
	State           string  `json:"state"`
	SuccessfulCalls int64   `json:"successfulCalls"`
	FailedCalls     int64   `json:"failedCalls"`
	RejectedCalls   int64   `json:"rejectedCalls"`
	FailureRate     float64 `json:"failureRate"`
	BufferedCalls   int     `json:"bufferedCalls"`
	BufferSize      int     `json:"bufferSize"`
}

// Warning represents a system warning
type Warning struct {
	ID           string    `json:"id"`
	Category     string    `json:"category"`
	Severity     string    `json:"severity"`
	Message      string    `json:"message"`
	Source       string    `json:"source"`
	Timestamp    time.Time `json:"timestamp"`
	Acknowledged bool      `json:"acknowledged"`
}

// InFlightMessage represents a message currently being processed
type InFlightMessage struct {
	MessageID      string    `json:"messageId"`
	PoolCode       string    `json:"poolCode"`
	MessageGroup   string    `json:"messageGroup,omitempty"`
	TargetURL      string    `json:"targetUrl"`
	StartedAt      time.Time `json:"startedAt"`
	DurationMs     int64     `json:"durationMs"`
	RetryCount     int       `json:"retryCount"`
	CircuitBreaker string    `json:"circuitBreaker,omitempty"`
}

// StandbyStatus represents the standby mode status
type StandbyStatus struct {
	StandbyEnabled        bool   `json:"standbyEnabled"`
	InstanceID            string `json:"instanceId,omitempty"`
	Role                  string `json:"role,omitempty"` // PRIMARY or STANDBY
	RedisAvailable        bool   `json:"redisAvailable,omitempty"`
	CurrentLockHolder     string `json:"currentLockHolder,omitempty"`
	LastSuccessfulRefresh string `json:"lastSuccessfulRefresh,omitempty"`
	HasWarning            bool   `json:"hasWarning,omitempty"`
}

// TrafficStatus represents the traffic management status
type TrafficStatus struct {
	Enabled       bool   `json:"enabled"`
	StrategyType  string `json:"strategyType,omitempty"`
	Registered    bool   `json:"registered,omitempty"`
	TargetInfo    string `json:"targetInfo,omitempty"`
	LastOperation string `json:"lastOperation,omitempty"`
	LastError     string `json:"lastError,omitempty"`
	Message       string `json:"message,omitempty"`
}
