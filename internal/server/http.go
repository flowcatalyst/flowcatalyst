package server

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// swaggerUIHTML is a minimal Swagger UI page (served at /swagger-ui) that
// loads the spec from /q/openapi, mirroring the Rust docs surface.
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

// Version is the build version reported by /health. Overridable at build
// time via -ldflags "-X .../internal/server.Version=<v>". Mirrors Rust's
// env!("CARGO_PKG_VERSION").
var Version = "dev"

// healthHandler is the package-level /health stub used by Run when no
// platform-mounted handler beats it. fc-server / fc-dev get the same
// shape so monitoring tooling can scrape both equivalently. Matches the
// Rust health_handler shape: {"status":"UP","version":...}.
func healthHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "UP", "version": Version})
}

// metricsRouter builds the /metrics + /ready + /health surface bound to
// the metrics port. Detailed router/pool Prometheus series live under
// the router prefix on the API port via routerapi.PrometheusHandler —
// this router stays a small "is the binary up" target until we add
// platform-level Prometheus exporters.
func metricsRouter(cfg EnvCfg) http.Handler {
	r := chi.NewRouter()
	r.Get("/health", healthHandler)
	r.Get("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ready",
			"platform":  cfg.PlatformEnabled,
			"router":    cfg.RouterEnabled,
			"scheduler": cfg.SchedulerEnabled,
			"stream":    cfg.StreamEnabled,
			"outbox":    cfg.OutboxEnabled,
			"mcp":       cfg.MCPEnabled,
		})
	})
	r.Get("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		// Platform-level Prometheus exporters are still on the to-do
		// list; router-level metrics live at <prefix>/metrics under the
		// API port.
		_, _ = w.Write([]byte("# fc-server metrics placeholder\n"))
	})
	return r
}
