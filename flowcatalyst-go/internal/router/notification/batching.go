package notification

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BatchingConfig holds batching configuration
type BatchingConfig struct {
	MinSeverity string
	BatchWindow time.Duration
}

// DefaultBatchingConfig returns default batching configuration
func DefaultBatchingConfig() *BatchingConfig {
	return &BatchingConfig{
		MinSeverity: "WARNING",
		BatchWindow: 5 * time.Minute,
	}
}

// BatchingService collects warnings over a configurable interval
// and sends a single summary notification to all registered delegates.
// Only sends notifications for warnings at or above the configured minimum severity.
type BatchingService struct {
	mu sync.Mutex

	delegates      []Service
	config         *BatchingConfig
	warningBatch   []*Warning
	categoryCount  map[string]int
	batchStartTime time.Time
}

// NewBatchingService creates a new batching notification service
func NewBatchingService(delegates []Service, config *BatchingConfig) *BatchingService {
	if config == nil {
		config = DefaultBatchingConfig()
	}

	slog.Info("BatchingNotificationService initialized",
		"delegates", len(delegates),
		"minSeverity", config.MinSeverity)

	return &BatchingService{
		delegates:      delegates,
		config:         config,
		warningBatch:   make([]*Warning, 0),
		categoryCount:  make(map[string]int),
		batchStartTime: time.Now(),
	}
}

// NotifyWarning adds a warning to the batch
func (s *BatchingService) NotifyWarning(warning *Warning) {
	if !MeetsMinSeverity(warning.Severity, s.config.MinSeverity) {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.warningBatch = append(s.warningBatch, warning)
	s.categoryCount[warning.Category]++
}

// NotifyCriticalError adds a critical error to the batch
func (s *BatchingService) NotifyCriticalError(message, source string) {
	warning := &Warning{
		ID:        uuid.New().String(),
		Category:  "CRITICAL_ERROR",
		Severity:  "CRITICAL",
		Message:   message,
		Timestamp: time.Now(),
		Source:    source,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.warningBatch = append(s.warningBatch, warning)
	s.categoryCount["CRITICAL_ERROR"]++
}

// NotifySystemEvent adds a system event to the batch if it meets severity
func (s *BatchingService) NotifySystemEvent(eventType, message string) {
	if !MeetsMinSeverity("INFO", s.config.MinSeverity) {
		return
	}

	category := "SYSTEM_EVENT_" + eventType
	warning := &Warning{
		ID:        uuid.New().String(),
		Category:  category,
		Severity:  "INFO",
		Message:   message,
		Timestamp: time.Now(),
		Source:    "System",
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.warningBatch = append(s.warningBatch, warning)
	s.categoryCount[category]++
}

// IsEnabled returns true if any delegate is enabled
func (s *BatchingService) IsEnabled() bool {
	for _, delegate := range s.delegates {
		if delegate.IsEnabled() {
			return true
		}
	}
	return len(s.delegates) > 0
}

// SendBatch sends batched notifications. Called by scheduler.
func (s *BatchingService) SendBatch() {
	s.mu.Lock()
	if len(s.warningBatch) == 0 {
		s.mu.Unlock()
		slog.Debug("No warnings to send in this batch period")
		return
	}

	// Copy batch data
	warnings := make([]*Warning, len(s.warningBatch))
	copy(warnings, s.warningBatch)
	batchEndTime := time.Now()
	batchStartTime := s.batchStartTime

	// Clear batch
	s.warningBatch = make([]*Warning, 0)
	s.categoryCount = make(map[string]int)
	s.batchStartTime = time.Now()
	s.mu.Unlock()

	slog.Info("Sending batched notification",
		"count", len(warnings),
		"startTime", batchStartTime,
		"endTime", batchEndTime)

	// Group warnings by severity
	warningsBySeverity := make(map[string][]*Warning)
	for _, w := range warnings {
		warningsBySeverity[w.Severity] = append(warningsBySeverity[w.Severity], w)
	}

	// Send summary to all delegates
	for _, delegate := range s.delegates {
		if err := s.sendSummaryToDelegate(delegate, warnings, warningsBySeverity, batchStartTime, batchEndTime); err != nil {
			slog.Error("Failed to send notification via delegate", "error", err)
		}
	}
}

// sendSummaryToDelegate sends summary notification to a single delegate
func (s *BatchingService) sendSummaryToDelegate(
	delegate Service,
	allWarnings []*Warning,
	warningsBySeverity map[string][]*Warning,
	startTime, endTime time.Time,
) error {
	// Build summary message
	var summary strings.Builder
	summary.WriteString(fmt.Sprintf("FlowCatalyst Warning Summary (%s to %s)\n\n",
		startTime.Format(time.RFC3339), endTime.Format(time.RFC3339)))

	// Add warnings by severity (in order: CRITICAL, ERROR, WARNING, INFO)
	for i := len(SeverityOrder) - 1; i >= 0; i-- {
		severity := SeverityOrder[i]
		warningsForSeverity := warningsBySeverity[severity]
		if len(warningsForSeverity) == 0 {
			continue
		}

		summary.WriteString(fmt.Sprintf("%s Issues (%d):\n", severity, len(warningsForSeverity)))

		// Group by category and show counts
		byCategory := make(map[string][]*Warning)
		for _, w := range warningsForSeverity {
			byCategory[w.Category] = append(byCategory[w.Category], w)
		}

		for category, categoryWarnings := range byCategory {
			if len(categoryWarnings) == 1 {
				summary.WriteString(fmt.Sprintf("  - %s: %s\n", category, categoryWarnings[0].Message))
			} else {
				summary.WriteString(fmt.Sprintf("  - %s: %d occurrences\n", category, len(categoryWarnings)))
				summary.WriteString(fmt.Sprintf("    Example: %s\n", categoryWarnings[0].Message))
			}
		}
		summary.WriteString("\n")
	}

	summary.WriteString(fmt.Sprintf("Total Warnings: %d\n", len(allWarnings)))

	// Create synthetic warning with summary
	summaryWarning := &Warning{
		ID:        uuid.New().String(),
		Category:  "BATCH_SUMMARY",
		Severity:  getHighestSeverity(warningsBySeverity),
		Message:   summary.String(),
		Timestamp: time.Now(),
		Source:    "BatchingNotificationService",
	}

	// Send to delegate
	delegate.NotifyWarning(summaryWarning)
	return nil
}

// getHighestSeverity returns the highest severity from the map
func getHighestSeverity(warningsBySeverity map[string][]*Warning) string {
	for i := len(SeverityOrder) - 1; i >= 0; i-- {
		if len(warningsBySeverity[SeverityOrder[i]]) > 0 {
			return SeverityOrder[i]
		}
	}
	return "INFO"
}
