package warning

import "time"

// Severity levels for warnings
const (
	SeverityCritical = "CRITICAL"
	SeverityError    = "ERROR"
	SeverityWarning  = "WARNING"
	SeverityInfo     = "INFO"
)

// Common warning categories
const (
	CategoryQueueBacklog   = "QUEUE_BACKLOG"
	CategoryQueueGrowing   = "QUEUE_GROWING"
	CategoryMediation      = "MEDIATION"
	CategoryConfiguration  = "CONFIGURATION"
	CategoryPoolLimit      = "POOL_LIMIT"
	CategoryCircuitBreaker = "CIRCUIT_BREAKER"
	CategoryHealth         = "HEALTH"
	CategoryLeader         = "LEADER_ELECTION"
)

// Warning represents a system warning or error notification
type Warning struct {
	// ID is the unique warning identifier (UUID)
	ID string `json:"id"`

	// Category is the warning category (e.g., QUEUE_BACKLOG, MEDIATION)
	Category string `json:"category"`

	// Severity is the severity level (CRITICAL, ERROR, WARNING, INFO)
	Severity string `json:"severity"`

	// Message describes the issue
	Message string `json:"message"`

	// Timestamp is when the warning was created
	Timestamp time.Time `json:"timestamp"`

	// Source is the component that generated the warning
	Source string `json:"source"`

	// Acknowledged indicates if the warning has been acknowledged
	Acknowledged bool `json:"acknowledged"`
}
