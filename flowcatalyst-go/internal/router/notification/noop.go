package notification

import "log/slog"

// NoOpService is a placeholder notification service that logs notifications instead of sending them.
// In the future, this can be replaced with implementations for email, Slack, PagerDuty, etc.
type NoOpService struct{}

// NewNoOpService creates a new no-op notification service
func NewNoOpService() *NoOpService {
	return &NoOpService{}
}

// NotifyWarning logs the warning
func (s *NoOpService) NotifyWarning(warning *Warning) {
	slog.Info("NOTIFICATION [WARNING]",
		"severity", warning.Severity,
		"category", warning.Category,
		"message", warning.Message,
		"source", warning.Source)
}

// NotifyCriticalError logs the critical error
func (s *NoOpService) NotifyCriticalError(message, source string) {
	slog.Error("NOTIFICATION [CRITICAL]",
		"message", message,
		"source", source)
}

// NotifySystemEvent logs the system event
func (s *NoOpService) NotifySystemEvent(eventType, message string) {
	slog.Info("NOTIFICATION [EVENT]",
		"eventType", eventType,
		"message", message)
}

// IsEnabled returns false as this is a placeholder implementation
func (s *NoOpService) IsEnabled() bool {
	return false
}
