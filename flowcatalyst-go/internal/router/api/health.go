package api

import (
	"encoding/json"
	"net/http"

	"go.flowcatalyst.tech/internal/router/health"
)

// HealthCheckHandler handles infrastructure health check endpoint
// GET /health
type HealthCheckHandler struct {
	infraHealth *health.InfrastructureHealthService
}

// NewHealthCheckHandler creates a new health check handler
func NewHealthCheckHandler(infraHealth *health.InfrastructureHealthService) *HealthCheckHandler {
	return &HealthCheckHandler{
		infraHealth: infraHealth,
	}
}

// ServeHTTP handles the health check request
func (h *HealthCheckHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	result := h.infraHealth.CheckHealth()

	w.Header().Set("Content-Type", "application/json")
	if result.Healthy {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(result)
}

// KubernetesHealthHandler handles Kubernetes-style health probes
// GET /health/live - Liveness probe
// GET /health/ready - Readiness probe
// GET /health/startup - Startup probe
type KubernetesHealthHandler struct {
	infraHealth  *health.InfrastructureHealthService
	brokerHealth *health.BrokerHealthService
}

// NewKubernetesHealthHandler creates a new Kubernetes health handler
func NewKubernetesHealthHandler(
	infraHealth *health.InfrastructureHealthService,
	brokerHealth *health.BrokerHealthService,
) *KubernetesHealthHandler {
	return &KubernetesHealthHandler{
		infraHealth:  infraHealth,
		brokerHealth: brokerHealth,
	}
}

// Liveness handles the liveness probe
// Returns 200 if the application is alive (not deadlocked, able to process requests)
func (h *KubernetesHealthHandler) Liveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// If we can respond, we're alive
	// Liveness probes should NOT check external dependencies
	// They should only verify the application itself is not deadlocked
	status := health.NewHealthyStatus("ALIVE")

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(status)
}

// Readiness handles the readiness probe
// Returns 200 if the application is ready to serve traffic
func (h *KubernetesHealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var issues []string

	// Check 1: Infrastructure health (QueueManager initialized, pools operational)
	if h.infraHealth != nil {
		infraHealth := h.infraHealth.CheckHealth()
		if !infraHealth.Healthy && infraHealth.Issues != nil {
			issues = append(issues, infraHealth.Issues...)
		}
	}

	// Check 2: Broker connectivity (critical external dependency)
	if h.brokerHealth != nil {
		brokerIssues := h.brokerHealth.CheckBrokerConnectivity()
		issues = append(issues, brokerIssues...)
	}

	// Determine readiness
	ready := len(issues) == 0

	var status *health.ReadinessStatus
	if ready {
		status = health.NewHealthyStatus("READY")
	} else {
		status = health.NewUnhealthyStatus("NOT_READY", issues)
	}

	w.Header().Set("Content-Type", "application/json")
	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

// Startup handles the startup probe
// Similar to readiness but with more lenient timeout/failure thresholds
func (h *KubernetesHealthHandler) Startup(w http.ResponseWriter, r *http.Request) {
	// For now, startup is the same as readiness
	h.Readiness(w, r)
}

// RegisterRoutes registers all health check routes on a mux
func (h *KubernetesHealthHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health/live", h.Liveness)
	mux.HandleFunc("/health/ready", h.Readiness)
	mux.HandleFunc("/health/startup", h.Startup)
}
