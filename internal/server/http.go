package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	schedulerpkg "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
)

// swaggerUIHTML is a minimal Swagger UI page (served at /swagger-ui) that
// loads the spec from /q/openapi (the legacy docs path).
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>FlowCatalyst API</title>
<link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist/swagger-ui.css"/>
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js" crossorigin></script>
<script>window.onload=function(){window.ui=SwaggerUIBundle({url:'/q/openapi',dom_id:'#swagger-ui'});};</script>
</body>
</html>`

// mountFallback makes fallback the router's NotFound handler.
//
// SPA history-mode routes (e.g. GET /auth/login, which an /oauth/authorize
// redirect lands on) can collide with an API path registered for another
// method (POST /auth/login). chi answers the method mismatch with 405 instead
// of falling through to NotFound, so the SPA would never render. A GET
// method-mismatch is therefore served by the fallback, unless isAPIPath says
// the path is an API's (a GET to a POST-only API route stays 405); every other
// method keeps the 405.
func mountFallback(r chi.Router, fallback http.Handler, isAPIPath func(string) bool) {
	r.NotFound(fallback.ServeHTTP)
	r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodGet && (isAPIPath == nil || !isAPIPath(req.URL.Path)) {
			fallback.ServeHTTP(w, req)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
}

// Version is the build version reported by /health. Overridable at build
// time via -ldflags "-X .../internal/server.Version=<v>".
var Version = "dev"

// healthHandler is the package-level /health stub used by Run when no
// platform-mounted handler beats it. fc-server / fcdev get the same
// shape so monitoring tooling can scrape both equivalently. The wire
// shape is {"status":"UP","version":...}.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP", "version": Version})
}

// metricsRouter builds the /metrics + /ready + /health surface bound to
// the metrics port. /metrics carries the process's Go runtime and OS
// metrics; detailed router/pool Prometheus series live under the router
// prefix on the API port via routerapi.PrometheusHandler.
//
// It serves NO application endpoint. The router-config document briefly lived
// here, unauthenticated, reached through a network alias; it is now an
// ordinary authenticated route on the API listener, so this listener is back
// to "is the binary up".
func metricsRouter(cfg EnvCfg) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":        "ready",
			"platform":      cfg.PlatformEnabled,
			"router":        cfg.RouterEnabled,
			"scheduler":     cfg.SchedulerEnabled,
			"scheduled_job": cfg.ScheduledJobEnabled,
			"stream":        cfg.StreamEnabled,
			"outbox":        cfg.OutboxEnabled,
			"mcp":           cfg.MCPEnabled,
		})
	})
	// The process's Go runtime and OS metrics (goroutines, GC, heap,
	// scheduler latency, CPU, RSS, open fds), for every subsystem this
	// process runs. Router pool/queue metrics live at <prefix>/metrics on
	// the API port.
	r.Handle("/metrics", runtimeMetricsHandler(cfg.SchedulerEnabled))
	return r
}

// runtimeMetricsHandler serves the Go runtime and process collectors from a
// registry of their own, plus the dispatch scheduler's series when this process
// runs the scheduler.
func runtimeMetricsHandler(scheduler bool) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	// The dispatch-job lifecycle's counters: every process that can write a
	// dispatch job (ingest, the delivery callback, fan-out, recovery) owns them,
	// scheduler or not.
	gatherers := prometheus.Gatherers{reg, dispatchjob.MetricsRegistry}
	if scheduler {
		gatherers = append(gatherers, schedulerpkg.MetricsRegistry)
	}
	return promhttp.HandlerFor(gatherers, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
}
