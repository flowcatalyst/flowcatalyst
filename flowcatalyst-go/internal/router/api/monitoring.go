package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"go.flowcatalyst.tech/internal/router/health"
)

// MonitoringHandler handles monitoring and metrics endpoints for dashboard
// All endpoints are at /monitoring/*
type MonitoringHandler struct {
	healthStatus           *health.HealthStatusService
	poolMetrics            health.PoolMetricsProvider
	queueMetrics           health.QueueStatsGetter
	warningService         health.WarningGetter
	warningSeverityGetter  WarningSeverityGetter
	circuitBreakers        health.CircuitBreakerGetter
	inFlightGetter         InFlightMessagesGetter
	standbyService         StandbyStatusGetter
	trafficService         TrafficStatusGetter
	warningMutator         WarningMutator
	circuitBrMutator       CircuitBreakerMutator
}

// InFlightMessagesGetter provides in-flight message info
type InFlightMessagesGetter interface {
	GetInFlightMessages(limit int, messageID string) []*health.InFlightMessage
}

// StandbyStatusGetter provides standby status info
type StandbyStatusGetter interface {
	IsEnabled() bool
	GetStatus() *health.StandbyStatus
}

// TrafficStatusGetter provides traffic management status
type TrafficStatusGetter interface {
	IsEnabled() bool
	GetStatus() *health.TrafficStatus
}

// WarningMutator provides warning mutations
type WarningMutator interface {
	AcknowledgeWarning(id string) bool
	ClearAllWarnings()
	ClearOldWarnings(hours int)
}

// WarningSeverityGetter provides warnings filtered by severity
type WarningSeverityGetter interface {
	GetWarningsBySeverity(severity string) []*health.Warning
}

// CircuitBreakerMutator provides circuit breaker mutations
type CircuitBreakerMutator interface {
	GetCircuitBreakerState(name string) string
	ResetCircuitBreaker(name string) bool
	ResetAllCircuitBreakers()
}

// NewMonitoringHandler creates a new monitoring handler
func NewMonitoringHandler(
	healthStatus *health.HealthStatusService,
	poolMetrics health.PoolMetricsProvider,
) *MonitoringHandler {
	return &MonitoringHandler{
		healthStatus: healthStatus,
		poolMetrics:  poolMetrics,
	}
}

// SetQueueMetrics sets the queue metrics provider
func (h *MonitoringHandler) SetQueueMetrics(qm health.QueueStatsGetter) {
	h.queueMetrics = qm
}

// SetWarningService sets the warning service
func (h *MonitoringHandler) SetWarningService(ws health.WarningGetter, wm WarningMutator) {
	h.warningService = ws
	h.warningMutator = wm
	// Also set severity getter if the service implements it
	if sg, ok := ws.(WarningSeverityGetter); ok {
		h.warningSeverityGetter = sg
	}
}

// SetCircuitBreakerService sets the circuit breaker service
func (h *MonitoringHandler) SetCircuitBreakerService(cb health.CircuitBreakerGetter, cbm CircuitBreakerMutator) {
	h.circuitBreakers = cb
	h.circuitBrMutator = cbm
}

// SetInFlightGetter sets the in-flight messages provider
func (h *MonitoringHandler) SetInFlightGetter(ifg InFlightMessagesGetter) {
	h.inFlightGetter = ifg
}

// SetStandbyService sets the standby service
func (h *MonitoringHandler) SetStandbyService(ss StandbyStatusGetter) {
	h.standbyService = ss
}

// SetTrafficService sets the traffic management service
func (h *MonitoringHandler) SetTrafficService(ts TrafficStatusGetter) {
	h.trafficService = ts
}

// GetHealthStatus handles GET /monitoring/health
func (h *MonitoringHandler) GetHealthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := h.healthStatus.GetHealthStatus()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// GetQueueStats handles GET /monitoring/queue-stats
func (h *MonitoringHandler) GetQueueStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats map[string]*health.QueueStats
	if h.queueMetrics != nil {
		stats = h.queueMetrics.GetAllQueueStats()
	} else {
		stats = make(map[string]*health.QueueStats)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetPoolStats handles GET /monitoring/pool-stats
func (h *MonitoringHandler) GetPoolStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats map[string]*health.PoolStats
	if h.poolMetrics != nil {
		stats = h.poolMetrics.GetAllPoolStats()
	} else {
		stats = make(map[string]*health.PoolStats)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetAllWarnings handles GET /monitoring/warnings
func (h *MonitoringHandler) GetAllWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var warnings []*health.Warning
	if h.warningService != nil {
		warnings = h.warningService.GetAllWarnings()
	} else {
		warnings = []*health.Warning{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(warnings)
}

// GetUnacknowledgedWarnings handles GET /monitoring/warnings/unacknowledged
func (h *MonitoringHandler) GetUnacknowledgedWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var warnings []*health.Warning
	if h.warningService != nil {
		warnings = h.warningService.GetUnacknowledgedWarnings()
	} else {
		warnings = []*health.Warning{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(warnings)
}

// GetWarningsBySeverity handles GET /monitoring/warnings/severity/{severity}
func (h *MonitoringHandler) GetWarningsBySeverity(w http.ResponseWriter, r *http.Request, severity string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var warnings []*health.Warning
	if h.warningSeverityGetter != nil {
		warnings = h.warningSeverityGetter.GetWarningsBySeverity(severity)
	} else {
		warnings = []*health.Warning{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(warnings)
}

// AcknowledgeWarning handles POST /monitoring/warnings/{warningId}/acknowledge
func (h *MonitoringHandler) AcknowledgeWarning(w http.ResponseWriter, r *http.Request, warningID string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.warningMutator != nil && h.warningMutator.AcknowledgeWarning(warningID) {
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	} else {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "Warning not found"})
	}
}

// ClearAllWarnings handles DELETE /monitoring/warnings
func (h *MonitoringHandler) ClearAllWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.warningMutator != nil {
		h.warningMutator.ClearAllWarnings()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// ClearOldWarnings handles DELETE /monitoring/warnings/old?hours=24
func (h *MonitoringHandler) ClearOldWarnings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hours := 24
	if hoursParam := r.URL.Query().Get("hours"); hoursParam != "" {
		if parsed, err := strconv.Atoi(hoursParam); err == nil {
			hours = parsed
		}
	}

	if h.warningMutator != nil {
		h.warningMutator.ClearOldWarnings(hours)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GetCircuitBreakerStats handles GET /monitoring/circuit-breakers
func (h *MonitoringHandler) GetCircuitBreakerStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var stats map[string]*health.CircuitBreakerStats
	if h.circuitBreakers != nil {
		stats = h.circuitBreakers.GetAllCircuitBreakerStats()
	} else {
		stats = make(map[string]*health.CircuitBreakerStats)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetCircuitBreakerState handles GET /monitoring/circuit-breakers/{name}/state
func (h *MonitoringHandler) GetCircuitBreakerState(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	state := "UNKNOWN"
	if h.circuitBrMutator != nil {
		state = h.circuitBrMutator.GetCircuitBreakerState(name)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"name": name, "state": state})
}

// ResetCircuitBreaker handles POST /monitoring/circuit-breakers/{name}/reset
func (h *MonitoringHandler) ResetCircuitBreaker(w http.ResponseWriter, r *http.Request, name string) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.circuitBrMutator != nil && h.circuitBrMutator.ResetCircuitBreaker(name) {
		json.NewEncoder(w).Encode(map[string]string{"status": "success"})
	} else {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": "Failed to reset circuit breaker"})
	}
}

// ResetAllCircuitBreakers handles POST /monitoring/circuit-breakers/reset-all
func (h *MonitoringHandler) ResetAllCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h.circuitBrMutator != nil {
		h.circuitBrMutator.ResetAllCircuitBreakers()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

// GetInFlightMessages handles GET /monitoring/in-flight-messages?limit=100&messageId=xxx
func (h *MonitoringHandler) GetInFlightMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 100
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		if parsed, err := strconv.Atoi(limitParam); err == nil {
			limit = parsed
		}
	}
	messageID := r.URL.Query().Get("messageId")

	var messages []*health.InFlightMessage
	if h.inFlightGetter != nil {
		messages = h.inFlightGetter.GetInFlightMessages(limit, messageID)
	} else {
		messages = []*health.InFlightMessage{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(messages)
}

// GetStandbyStatus handles GET /monitoring/standby-status
func (h *MonitoringHandler) GetStandbyStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.standbyService == nil || !h.standbyService.IsEnabled() {
		json.NewEncoder(w).Encode(map[string]bool{"standbyEnabled": false})
		return
	}

	status := h.standbyService.GetStatus()
	json.NewEncoder(w).Encode(status)
}

// GetTrafficStatus handles GET /monitoring/traffic-status
func (h *MonitoringHandler) GetTrafficStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if h.trafficService == nil || !h.trafficService.IsEnabled() {
		json.NewEncoder(w).Encode(health.TrafficStatus{
			Enabled: false,
			Message: "Traffic management not available",
		})
		return
	}

	status := h.trafficService.GetStatus()
	json.NewEncoder(w).Encode(status)
}

// GetDashboard handles GET /monitoring/dashboard
// Returns the monitoring dashboard HTML page
func (h *MonitoringHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(dashboardHTML))
}

// RegisterRoutes registers all monitoring routes on a mux
// Note: This uses a simple pattern matching since Go 1.22+ supports path params
func (h *MonitoringHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/monitoring/health", h.GetHealthStatus)
	mux.HandleFunc("/monitoring/queue-stats", h.GetQueueStats)
	mux.HandleFunc("/monitoring/pool-stats", h.GetPoolStats)
	mux.HandleFunc("/monitoring/warnings", h.handleWarnings)
	mux.HandleFunc("/monitoring/warnings/unacknowledged", h.GetUnacknowledgedWarnings)
	mux.HandleFunc("/monitoring/warnings/old", h.ClearOldWarnings)
	mux.HandleFunc("/monitoring/warnings/severity/", h.handleWarningSeverity)
	mux.HandleFunc("/monitoring/circuit-breakers", h.handleCircuitBreakers)
	mux.HandleFunc("/monitoring/circuit-breakers/reset-all", h.ResetAllCircuitBreakers)
	mux.HandleFunc("/monitoring/in-flight-messages", h.GetInFlightMessages)
	mux.HandleFunc("/monitoring/standby-status", h.GetStandbyStatus)
	mux.HandleFunc("/monitoring/traffic-status", h.GetTrafficStatus)
	mux.HandleFunc("/monitoring/dashboard", h.GetDashboard)
}

// handleWarnings handles GET/DELETE for /monitoring/warnings
func (h *MonitoringHandler) handleWarnings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.GetAllWarnings(w, r)
	case http.MethodDelete:
		h.ClearAllWarnings(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCircuitBreakers handles GET for /monitoring/circuit-breakers
func (h *MonitoringHandler) handleCircuitBreakers(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.GetCircuitBreakerStats(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleWarningSeverity handles GET /monitoring/warnings/severity/{severity}
func (h *MonitoringHandler) handleWarningSeverity(w http.ResponseWriter, r *http.Request) {
	// Extract severity from path: /monitoring/warnings/severity/{severity}
	path := r.URL.Path
	prefix := "/monitoring/warnings/severity/"
	if len(path) <= len(prefix) {
		http.Error(w, "Severity parameter required", http.StatusBadRequest)
		return
	}
	severity := path[len(prefix):]
	h.GetWarningsBySeverity(w, r, severity)
}
