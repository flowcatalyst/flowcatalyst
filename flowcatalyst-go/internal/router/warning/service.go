package warning

import (
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Service manages system warnings
type Service interface {
	// AddWarning adds a new warning
	AddWarning(category, severity, message, source string)

	// GetAllWarnings returns all warnings
	GetAllWarnings() []Warning

	// GetWarningsBySeverity returns warnings filtered by severity
	GetWarningsBySeverity(severity string) []Warning

	// GetUnacknowledgedWarnings returns unacknowledged warnings
	GetUnacknowledgedWarnings() []Warning

	// AcknowledgeWarning acknowledges a warning by ID
	AcknowledgeWarning(warningID string) bool

	// ClearAllWarnings removes all warnings
	ClearAllWarnings()

	// ClearOldWarnings removes warnings older than specified hours
	ClearOldWarnings(hoursOld int)
}

// InMemoryService stores warnings in memory
type InMemoryService struct {
	mu          sync.RWMutex
	warnings    map[string]*Warning
	maxWarnings int
}

// NewInMemoryService creates a new in-memory warning service
func NewInMemoryService() *InMemoryService {
	return &InMemoryService{
		warnings:    make(map[string]*Warning),
		maxWarnings: 1000,
	}
}

// NewInMemoryServiceWithLimit creates a new in-memory warning service with custom limit
func NewInMemoryServiceWithLimit(maxWarnings int) *InMemoryService {
	return &InMemoryService{
		warnings:    make(map[string]*Warning),
		maxWarnings: maxWarnings,
	}
}

// AddWarning adds a new warning
func (s *InMemoryService) AddWarning(category, severity, message, source string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Limit warning storage by removing oldest if at capacity
	if len(s.warnings) >= s.maxWarnings {
		s.removeOldest()
	}

	warningID := uuid.New().String()
	warning := &Warning{
		ID:           warningID,
		Category:     category,
		Severity:     severity,
		Message:      message,
		Timestamp:    time.Now(),
		Source:       source,
		Acknowledged: false,
	}

	s.warnings[warningID] = warning

	slog.Info("Warning added",
		"severity", severity,
		"category", category,
		"source", source,
		"message", message)
}

// removeOldest removes the oldest warning (must be called with lock held)
func (s *InMemoryService) removeOldest() {
	var oldestID string
	var oldestTime time.Time

	for id, w := range s.warnings {
		if oldestID == "" || w.Timestamp.Before(oldestTime) {
			oldestID = id
			oldestTime = w.Timestamp
		}
	}

	if oldestID != "" {
		delete(s.warnings, oldestID)
	}
}

// GetAllWarnings returns all warnings sorted by timestamp (newest first)
func (s *InMemoryService) GetAllWarnings() []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.sortedWarnings(nil)
}

// GetWarningsBySeverity returns warnings filtered by severity
func (s *InMemoryService) GetWarningsBySeverity(severity string) []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filter := func(w *Warning) bool {
		return strings.EqualFold(w.Severity, severity)
	}
	return s.sortedWarnings(filter)
}

// GetUnacknowledgedWarnings returns unacknowledged warnings
func (s *InMemoryService) GetUnacknowledgedWarnings() []Warning {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filter := func(w *Warning) bool {
		return !w.Acknowledged
	}
	return s.sortedWarnings(filter)
}

// sortedWarnings returns warnings sorted by timestamp (newest first) with optional filter
func (s *InMemoryService) sortedWarnings(filter func(*Warning) bool) []Warning {
	result := make([]Warning, 0, len(s.warnings))

	for _, w := range s.warnings {
		if filter == nil || filter(w) {
			result = append(result, *w)
		}
	}

	// Sort by timestamp descending (newest first)
	sort.Slice(result, func(i, j int) bool {
		return result[i].Timestamp.After(result[j].Timestamp)
	})

	return result
}

// AcknowledgeWarning acknowledges a warning by ID
func (s *InMemoryService) AcknowledgeWarning(warningID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	warning, exists := s.warnings[warningID]
	if !exists {
		return false
	}

	warning.Acknowledged = true
	slog.Info("Warning acknowledged", "warningId", warningID)
	return true
}

// ClearAllWarnings removes all warnings
func (s *InMemoryService) ClearAllWarnings() {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := len(s.warnings)
	s.warnings = make(map[string]*Warning)
	slog.Info("Cleared all warnings", "count", count)
}

// ClearOldWarnings removes warnings older than specified hours
func (s *InMemoryService) ClearOldWarnings(hoursOld int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	threshold := time.Now().Add(-time.Duration(hoursOld) * time.Hour)
	var toRemove []string

	for id, w := range s.warnings {
		if w.Timestamp.Before(threshold) {
			toRemove = append(toRemove, id)
		}
	}

	for _, id := range toRemove {
		delete(s.warnings, id)
	}

	slog.Info("Cleared old warnings", "count", len(toRemove), "hoursOld", hoursOld)
}

// Count returns the current number of warnings
func (s *InMemoryService) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.warnings)
}
