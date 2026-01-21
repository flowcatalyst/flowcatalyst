package notification

import (
	"time"
)

// Warning represents a system warning
type Warning struct {
	ID           string    `json:"id"`
	Category     string    `json:"category"`
	Severity     string    `json:"severity"`
	Message      string    `json:"message"`
	Timestamp    time.Time `json:"timestamp"`
	Source       string    `json:"source"`
	Acknowledged bool      `json:"acknowledged"`
}

// Service defines the notification service interface
type Service interface {
	// NotifyWarning sends a notification for a warning
	NotifyWarning(warning *Warning)

	// NotifyCriticalError sends a notification for a critical error
	NotifyCriticalError(message, source string)

	// NotifySystemEvent sends a notification for a system event
	NotifySystemEvent(eventType, message string)

	// IsEnabled checks if notifications are enabled
	IsEnabled() bool
}

// Severity levels in order of importance
var SeverityOrder = []string{"INFO", "WARNING", "ERROR", "CRITICAL"}

// GetSeverityIndex returns the index of a severity level
func GetSeverityIndex(severity string) int {
	for i, s := range SeverityOrder {
		if s == severity {
			return i
		}
	}
	return 0
}

// MeetsMinSeverity checks if severity meets minimum threshold
func MeetsMinSeverity(severity, minSeverity string) bool {
	return GetSeverityIndex(severity) >= GetSeverityIndex(minSeverity)
}
