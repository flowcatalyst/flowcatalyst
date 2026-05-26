package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
	routerapi "github.com/flowcatalyst/flowcatalyst-go/internal/router/api"
)

func setup(t *testing.T) (*chi.Mux, *router.WarningService, *router.HealthService) {
	t.Helper()
	ws := router.NewWarningService(router.WarningServiceConfig{})
	hs := router.NewHealthService(router.DefaultHealthServiceConfig(), ws)
	r := chi.NewRouter()
	routerapi.RegisterRoutes(r, routerapi.Deps{Warnings: ws, Health: hs})
	return r, ws, hs
}

func fire(t *testing.T, r http.Handler, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader *strings.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	var req *http.Request
	if reader != nil {
		req = httptest.NewRequest(method, path, reader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode json: %v (body=%q)", err, w.Body.String())
	}
	return out
}

func decodeArray(t *testing.T, w *httptest.ResponseRecorder) []any {
	t.Helper()
	var out []any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode array: %v (body=%q)", err, w.Body.String())
	}
	return out
}

// ── Health probes ────────────────────────────────────────────────────────

func TestHealthLive(t *testing.T) {
	r, _, _ := setup(t)
	w := fire(t, r, "GET", "/health/live", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	if body["status"] != "LIVE" {
		t.Errorf("body.status: got %v want LIVE", body["status"])
	}
}

func TestHealthReady_HealthyReturns200(t *testing.T) {
	r, _, hs := setup(t)
	hs.SetConsumerRunning("c1", true)
	hs.RecordConsumerPoll("c1")
	w := fire(t, r, "GET", "/health/ready", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if decodeJSON(t, w)["status"] != "READY" {
		t.Error("expected status=READY")
	}
}

func TestHealthReady_DegradedReturns503(t *testing.T) {
	r, ws, hs := setup(t)
	hs.SetConsumerRunning("c1", true)
	hs.RecordConsumerPoll("c1")
	// Critical warning forces Degraded status.
	ws.Add(router.WarningCategoryConnection, router.WarningCritical, "uh oh", "test")
	w := fire(t, r, "GET", "/health/ready", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status: got %d want 503", w.Code)
	}
	if decodeJSON(t, w)["status"] != "NOT_READY" {
		t.Error("expected status=NOT_READY")
	}
}

func TestHealth_LegacyEndpoint(t *testing.T) {
	r, _, _ := setup(t)
	w := fire(t, r, "GET", "/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	// Snake-case per Rust convention.
	if _, ok := body["active_warnings"]; !ok {
		t.Errorf("expected active_warnings field (snake_case for parity)")
	}
}

// ── Monitoring reads ─────────────────────────────────────────────────────

func TestMonitoring_ShapeMatchesRust(t *testing.T) {
	r, _, _ := setup(t)
	w := fire(t, r, "GET", "/monitoring", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	// Top-level keys MUST match Rust's MonitoringResponse exactly
	// (snake_case from default serde) so the dashboard isn't surprised.
	for _, key := range []string{"status", "version", "health_report", "pool_stats", "active_warnings", "critical_warnings"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level key %q (parity-critical)", key)
		}
	}
	report, _ := body["health_report"].(map[string]any)
	for _, key := range []string{"status", "pools_healthy", "pools_unhealthy", "consumers_healthy", "consumers_unhealthy", "active_warnings", "critical_warnings", "issues"} {
		if _, ok := report[key]; !ok {
			t.Errorf("missing health_report.%s (parity-critical)", key)
		}
	}
}

func TestMonitoring_HealthDashboardCamelCase(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningCritical, "boom", "test")
	w := fire(t, r, "GET", "/monitoring/health", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	// Dashboard response: top-level snake_case (status/timestamp/uptimeMillis is actually camelCase
	// per the Rust rename), details object uses camelCase per-field.
	for _, key := range []string{"status", "timestamp", "uptimeMillis", "details"} {
		if _, ok := body[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}
	details, _ := body["details"].(map[string]any)
	for _, key := range []string{"totalQueues", "healthyQueues", "totalPools", "healthyPools", "activeWarnings", "criticalWarnings", "circuitBreakersOpen", "degradationReason"} {
		if _, ok := details[key]; !ok {
			t.Errorf("missing details.%s (parity-critical camelCase)", key)
		}
	}
	if details["degradationReason"] == nil {
		t.Errorf("expected degradationReason to be set when warnings present")
	}
}

func TestMonitoring_PoolsEmptyByDefault(t *testing.T) {
	r, _, _ := setup(t)
	w := fire(t, r, "GET", "/monitoring/pools", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	out := decodeArray(t, w)
	if len(out) != 0 {
		t.Errorf("pools: got %d want 0 (no PoolStatsProvider wired)", len(out))
	}
}

func TestMonitoring_WarningsShape(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "test1", "src")
	ws.Add(router.WarningCategoryConnection, router.WarningError, "test2", "src")
	w := fire(t, r, "GET", "/monitoring/warnings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	out := decodeArray(t, w)
	if len(out) != 2 {
		t.Errorf("warnings: got %d want 2", len(out))
	}
	first, _ := out[0].(map[string]any)
	// Snake-case per Rust Warning default serde.
	for _, key := range []string{"id", "category", "severity", "message", "source", "created_at", "acknowledged"} {
		if _, ok := first[key]; !ok {
			t.Errorf("missing warning.%s (parity-critical snake_case)", key)
		}
	}
}

// ── Warnings management ──────────────────────────────────────────────────

func TestListWarnings_FilterBySeverity(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w", "s")
	ws.Add(router.WarningCategoryConnection, router.WarningCritical, "c", "s")

	w := fire(t, r, "GET", "/warnings?severity=CRITICAL", "")
	out := decodeArray(t, w)
	if len(out) != 1 {
		t.Fatalf("filter=CRITICAL: got %d want 1", len(out))
	}
	got, _ := out[0].(map[string]any)
	if got["severity"] != "CRITICAL" {
		t.Errorf("expected severity=CRITICAL, got %v", got["severity"])
	}
}

func TestListWarnings_SeverityAcceptsWarnAlias(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w", "s")

	// "WARN" and "WARNING" must both map to the WARNING severity (Rust
	// list_warnings accepts both spellings).
	for _, q := range []string{"WARN", "WARNING"} {
		w := fire(t, r, "GET", "/warnings?severity="+q, "")
		out := decodeArray(t, w)
		if len(out) != 1 {
			t.Errorf("severity=%s: got %d want 1", q, len(out))
		}
	}
}

func TestListWarnings_FilterByAcknowledged(t *testing.T) {
	r, ws, _ := setup(t)
	id := ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w1", "s")
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w2", "s")
	ws.Acknowledge(id)

	w := fire(t, r, "GET", "/warnings?acknowledged=false", "")
	out := decodeArray(t, w)
	if len(out) != 1 {
		t.Errorf("acknowledged=false: got %d want 1", len(out))
	}
}

func TestAcknowledge_Success(t *testing.T) {
	r, ws, _ := setup(t)
	id := ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w", "s")
	w := fire(t, r, "POST", "/warnings/"+id+"/acknowledge", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	if body["acknowledged"] != true {
		t.Errorf("body.acknowledged: got %v want true", body["acknowledged"])
	}
	if ws.UnacknowledgedCount() != 0 {
		t.Errorf("unack count: got %d want 0", ws.UnacknowledgedCount())
	}
}

func TestAcknowledge_NotFound(t *testing.T) {
	r, _, _ := setup(t)
	w := fire(t, r, "POST", "/warnings/does-not-exist/acknowledge", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", w.Code)
	}
	body := decodeJSON(t, w)
	if body["error"] != "Warning not found" {
		t.Errorf("body.error: got %v want 'Warning not found'", body["error"])
	}
}

func TestAcknowledgeAll(t *testing.T) {
	r, ws, _ := setup(t)
	for i := 0; i < 3; i++ {
		ws.Add(router.WarningCategoryConnection, router.WarningWarning, "x", "s")
	}
	w := fire(t, r, "POST", "/warnings/acknowledge-all", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	body := decodeJSON(t, w)
	if got := body["acknowledged"]; got != float64(3) {
		t.Errorf("body.acknowledged: got %v want 3", got)
	}
	if ws.UnacknowledgedCount() != 0 {
		t.Errorf("unack count after ack-all: got %d want 0", ws.UnacknowledgedCount())
	}
}

func TestCritical(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "w", "s")
	ws.Add(router.WarningCategoryConnection, router.WarningCritical, "c", "s")
	w := fire(t, r, "GET", "/warnings/critical", "")
	out := decodeArray(t, w)
	if len(out) != 1 {
		t.Errorf("critical: got %d want 1", len(out))
	}
}

func TestClearAll(t *testing.T) {
	r, ws, _ := setup(t)
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "x", "s")
	ws.Add(router.WarningCategoryConnection, router.WarningWarning, "y", "s")
	w := fire(t, r, "DELETE", "/warnings", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200", w.Code)
	}
	if ws.Count() != 0 {
		t.Errorf("count after clear: got %d want 0", ws.Count())
	}
}
