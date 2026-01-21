package health

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// QueueType represents the type of message queue
type QueueType string

const (
	QueueTypeSQS      QueueType = "SQS"
	QueueTypeNATS     QueueType = "NATS"
	QueueTypeActiveMQ QueueType = "ACTIVEMQ"
	QueueTypeEmbedded QueueType = "EMBEDDED"
)

// BrokerConnectivityChecker provides broker-specific connectivity checks
type BrokerConnectivityChecker interface {
	// CheckConnectivity checks if the broker is accessible
	CheckConnectivity(ctx context.Context) error
	// CheckQueueAccessible checks if a specific queue is accessible
	CheckQueueAccessible(ctx context.Context, queueName string) error
}

// BrokerHealthService checks broker (SQS/NATS/ActiveMQ) connectivity and health.
// Provides explicit health checks for external messaging dependencies.
type BrokerHealthService struct {
	mu sync.RWMutex

	enabled      bool
	queueType    QueueType
	checker      BrokerConnectivityChecker
	lastCheck    time.Time
	lastResult   bool
	lastIssues   []string

	// Metrics
	connectionAttempts  int64
	connectionSuccesses int64
	connectionFailures  int64
	brokerAvailable     atomic.Int32
}

// NewBrokerHealthService creates a new broker health service
func NewBrokerHealthService(enabled bool, queueType QueueType, checker BrokerConnectivityChecker) *BrokerHealthService {
	svc := &BrokerHealthService{
		enabled:   enabled,
		queueType: queueType,
		checker:   checker,
	}
	svc.brokerAvailable.Store(0)
	return svc
}

// CheckBrokerConnectivity checks broker connectivity based on configured queue type.
// This is a quick connectivity check, not a full queue validation.
// Returns a list of issues found, empty if healthy.
func (s *BrokerHealthService) CheckBrokerConnectivity() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.enabled {
		slog.Debug("Message router disabled, skipping broker connectivity check")
		return []string{}
	}

	atomic.AddInt64(&s.connectionAttempts, 1)
	s.lastCheck = time.Now()

	var issues []string

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var connected bool

	switch s.queueType {
	case QueueTypeEmbedded:
		// Embedded queue is always available
		connected = true

	default:
		if s.checker != nil {
			err := s.checker.CheckConnectivity(ctx)
			if err != nil {
				slog.Error("Broker connectivity check failed", "error", err, "queueType", string(s.queueType))
				issues = append(issues, fmt.Sprintf("%s broker connectivity check failed: %v", s.queueType, err))
				connected = false
			} else {
				connected = true
			}
		} else {
			slog.Warn("No broker connectivity checker configured", "queueType", string(s.queueType))
			issues = append(issues, fmt.Sprintf("%s broker checker not configured", s.queueType))
			connected = false
		}
	}

	if connected {
		atomic.AddInt64(&s.connectionSuccesses, 1)
		s.brokerAvailable.Store(1)
		slog.Debug("Broker connectivity check passed", "queueType", string(s.queueType))
	} else {
		atomic.AddInt64(&s.connectionFailures, 1)
		s.brokerAvailable.Store(0)
		if len(issues) == 0 {
			issues = append(issues, fmt.Sprintf("%s broker is not accessible", s.queueType))
		}
	}

	s.lastResult = connected
	s.lastIssues = issues
	return issues
}

// CheckQueueAccessible checks if a specific queue is accessible
func (s *BrokerHealthService) CheckQueueAccessible(queueName string) []string {
	if !s.enabled || s.checker == nil {
		return []string{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := s.checker.CheckQueueAccessible(ctx, queueName)
	if err != nil {
		return []string{fmt.Sprintf("Cannot access queue [%s]: %v", queueName, err)}
	}

	return []string{}
}

// GetBrokerType returns the current broker type
func (s *BrokerHealthService) GetBrokerType() QueueType {
	return s.queueType
}

// IsAvailable returns whether the broker is currently available
func (s *BrokerHealthService) IsAvailable() bool {
	return s.brokerAvailable.Load() == 1
}

// GetMetrics returns broker health metrics
func (s *BrokerHealthService) GetMetrics() (attempts, successes, failures int64) {
	return atomic.LoadInt64(&s.connectionAttempts),
		atomic.LoadInt64(&s.connectionSuccesses),
		atomic.LoadInt64(&s.connectionFailures)
}

// GetLastCheck returns the last check time and result
func (s *BrokerHealthService) GetLastCheck() (time.Time, bool, []string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastCheck, s.lastResult, s.lastIssues
}

// SetChecker updates the broker connectivity checker
func (s *BrokerHealthService) SetChecker(checker BrokerConnectivityChecker) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.checker = checker
}
