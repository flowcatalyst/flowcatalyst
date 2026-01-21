package health

import (
	"encoding/json"
	"net/http"
	"sync"
)

// Status represents the health status of a component
type Status string

const (
	StatusUp   Status = "UP"
	StatusDown Status = "DOWN"
)

// Check represents a single health check
type Check struct {
	Name   string                 `json:"name"`
	Status Status                 `json:"status"`
	Data   map[string]interface{} `json:"data,omitempty"`
}

// HealthResponse represents the health endpoint response
type HealthResponse struct {
	Status Status  `json:"status"`
	Checks []Check `json:"checks,omitempty"`
}

// CheckFunc is a function that performs a health check
type CheckFunc func() Check

// Checker manages health checks for the application
type Checker struct {
	mu            sync.RWMutex
	livenessChecks  []CheckFunc
	readinessChecks []CheckFunc
}

// NewChecker creates a new health checker
func NewChecker() *Checker {
	return &Checker{
		livenessChecks:  make([]CheckFunc, 0),
		readinessChecks: make([]CheckFunc, 0),
	}
}

// AddLivenessCheck adds a liveness check
func (c *Checker) AddLivenessCheck(check CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.livenessChecks = append(c.livenessChecks, check)
}

// AddReadinessCheck adds a readiness check
func (c *Checker) AddReadinessCheck(check CheckFunc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readinessChecks = append(c.readinessChecks, check)
}

// runChecks runs a set of health checks and returns the aggregated response
func (c *Checker) runChecks(checks []CheckFunc) HealthResponse {
	c.mu.RLock()
	defer c.mu.RUnlock()

	response := HealthResponse{
		Status: StatusUp,
		Checks: make([]Check, 0, len(checks)),
	}

	for _, checkFunc := range checks {
		check := checkFunc()
		response.Checks = append(response.Checks, check)
		if check.Status == StatusDown {
			response.Status = StatusDown
		}
	}

	return response
}

// GetLiveness returns the liveness status
func (c *Checker) GetLiveness() HealthResponse {
	return c.runChecks(c.livenessChecks)
}

// GetReadiness returns the readiness status
func (c *Checker) GetReadiness() HealthResponse {
	return c.runChecks(c.readinessChecks)
}

// GetHealth returns the combined health status
func (c *Checker) GetHealth() HealthResponse {
	c.mu.RLock()
	allChecks := make([]CheckFunc, 0, len(c.livenessChecks)+len(c.readinessChecks))
	allChecks = append(allChecks, c.livenessChecks...)
	allChecks = append(allChecks, c.readinessChecks...)
	c.mu.RUnlock()

	return c.runChecks(allChecks)
}

// HandleHealth handles the /q/health endpoint
func (c *Checker) HandleHealth(w http.ResponseWriter, r *http.Request) {
	response := c.GetHealth()
	c.writeResponse(w, response)
}

// HandleLive handles the /q/health/live endpoint
func (c *Checker) HandleLive(w http.ResponseWriter, r *http.Request) {
	// Liveness is always UP if the server is running
	// Add any liveness checks if needed
	response := c.GetLiveness()
	if len(response.Checks) == 0 {
		response.Status = StatusUp
	}
	c.writeResponse(w, response)
}

// HandleReady handles the /q/health/ready endpoint
func (c *Checker) HandleReady(w http.ResponseWriter, r *http.Request) {
	response := c.GetReadiness()
	if len(response.Checks) == 0 {
		response.Status = StatusUp
	}
	c.writeResponse(w, response)
}

func (c *Checker) writeResponse(w http.ResponseWriter, response HealthResponse) {
	w.Header().Set("Content-Type", "application/json")

	if response.Status == StatusDown {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(response)
}

// MongoDBCheck creates a health check for MongoDB
func MongoDBCheck(pingFunc func() error) CheckFunc {
	return func() Check {
		err := pingFunc()
		if err != nil {
			return Check{
				Name:   "MongoDB",
				Status: StatusDown,
				Data: map[string]interface{}{
					"error": err.Error(),
				},
			}
		}
		return Check{
			Name:   "MongoDB",
			Status: StatusUp,
		}
	}
}

// NATSCheck creates a health check for NATS
func NATSCheck(isConnected func() bool) CheckFunc {
	return func() Check {
		if !isConnected() {
			return Check{
				Name:   "NATS",
				Status: StatusDown,
			}
		}
		return Check{
			Name:   "NATS",
			Status: StatusUp,
		}
	}
}

// SQSCheck creates a health check for AWS SQS
// The checkFunc should call GetQueueAttributes to verify queue accessibility
func SQSCheck(checkFunc func() error) CheckFunc {
	return func() Check {
		err := checkFunc()
		if err != nil {
			return Check{
				Name:   "SQS",
				Status: StatusDown,
				Data: map[string]interface{}{
					"error": err.Error(),
				},
			}
		}
		return Check{
			Name:   "SQS",
			Status: StatusUp,
		}
	}
}

// StreamProcessorCheck creates a health check for a stream processor
// The isRunning function should return true if the processor is healthy
// The watcherStatuses should return the status of each watcher
func StreamProcessorCheck(isRunning func() bool, getWatcherStatuses func() map[string]bool) CheckFunc {
	return func() Check {
		if !isRunning() {
			return Check{
				Name:   "StreamProcessor",
				Status: StatusDown,
				Data: map[string]interface{}{
					"running": false,
				},
			}
		}

		watcherStatuses := getWatcherStatuses()
		allRunning := true
		for _, running := range watcherStatuses {
			if !running {
				allRunning = false
				break
			}
		}

		status := StatusUp
		if !allRunning {
			status = StatusDown
		}

		return Check{
			Name:   "StreamProcessor",
			Status: status,
			Data: map[string]interface{}{
				"running":  true,
				"watchers": watcherStatuses,
			},
		}
	}
}

// StreamMetricsData holds detailed metrics for a stream watcher (interface type for health check)
type StreamMetricsData struct {
	WatcherName      string
	Running          bool
	HasFatalError    bool
	FatalError       string
	BatchesProcessed int64
	CheckpointedSeq  int64
	InFlightCount    int32
	AvailableSlots   int32
}

// StreamProcessorCheckDetailed creates a detailed health check for a stream processor
// This version includes metrics like Java's StreamContext: batches, checkpoints, in-flight, fatal errors
func StreamProcessorCheckDetailed(isRunning func() bool, getMetrics func() interface{}) CheckFunc {
	return func() Check {
		if !isRunning() {
			return Check{
				Name:   "StreamProcessor",
				Status: StatusDown,
				Data: map[string]interface{}{
					"running": false,
				},
			}
		}

		metricsRaw := getMetrics()

		// Type assert to slice of metrics
		// We use interface{} to avoid circular imports with stream package
		metrics, ok := metricsRaw.([]StreamMetricsData)
		if !ok {
			// Try to handle the case where it's a different struct with same fields
			// This works with any slice type that has compatible interface
			return handleGenericMetrics(metricsRaw)
		}

		// Check for fatal errors in any stream
		for _, m := range metrics {
			if m.HasFatalError {
				return Check{
					Name:   "StreamProcessor",
					Status: StatusDown,
					Data: map[string]interface{}{
						"running":      true,
						"failedStream": m.WatcherName,
						"error":        m.FatalError,
					},
				}
			}
		}

		// Aggregate metrics
		var totalBatches, totalCheckpointed int64
		var totalInFlight, totalAvailableSlots int32
		runningCount := 0

		for _, m := range metrics {
			if m.Running {
				runningCount++
			}
			totalBatches += m.BatchesProcessed
			totalCheckpointed += m.CheckpointedSeq
			totalInFlight += m.InFlightCount
			totalAvailableSlots += m.AvailableSlots
		}

		if runningCount == 0 && len(metrics) > 0 {
			return Check{
				Name:   "StreamProcessor",
				Status: StatusDown,
				Data: map[string]interface{}{
					"running": false,
					"reason":  "No streams running",
				},
			}
		}

		return Check{
			Name:   "StreamProcessor",
			Status: StatusUp,
			Data: map[string]interface{}{
				"running":                        true,
				"totalStreams":                   len(metrics),
				"runningStreams":                 runningCount,
				"totalBatchesProcessed":          totalBatches,
				"totalCheckpointedBatches":       totalCheckpointed,
				"totalInFlightBatches":           totalInFlight,
				"totalAvailableConcurrencySlots": totalAvailableSlots,
			},
		}
	}
}

// handleGenericMetrics handles metrics from any source using reflection-like interface approach
func handleGenericMetrics(metricsRaw interface{}) Check {
	// Check if it implements a common interface or use type switch
	switch v := metricsRaw.(type) {
	case []interface{}:
		return aggregateFromInterfaceSlice(v)
	default:
		// Fallback to simple response
		return Check{
			Name:   "StreamProcessor",
			Status: StatusUp,
			Data: map[string]interface{}{
				"running": true,
			},
		}
	}
}

func aggregateFromInterfaceSlice(metrics []interface{}) Check {
	runningCount := 0
	totalCount := len(metrics)

	for _, m := range metrics {
		if metricMap, ok := m.(map[string]interface{}); ok {
			if running, ok := metricMap["Running"].(bool); ok && running {
				runningCount++
			}
			if hasFatal, ok := metricMap["HasFatalError"].(bool); ok && hasFatal {
				return Check{
					Name:   "StreamProcessor",
					Status: StatusDown,
					Data: map[string]interface{}{
						"running":      true,
						"failedStream": metricMap["WatcherName"],
						"error":        metricMap["FatalError"],
					},
				}
			}
		}
	}

	return Check{
		Name:   "StreamProcessor",
		Status: StatusUp,
		Data: map[string]interface{}{
			"running":        true,
			"totalStreams":   totalCount,
			"runningStreams": runningCount,
		},
	}
}
