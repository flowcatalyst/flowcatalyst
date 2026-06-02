package api

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed dashboard.html
var dashboardHTML string

// handleDashboardHTML serves the embedded dashboard, with the mount
// prefix injected so the page works both standalone and when nested
// under a parent router (e.g. fc-dev nesting fc-router under /q/router).
//
// Mirrors crates/fc-router/src/api/mod.rs::dashboard_html_handler. The
// injected `window.__API_BASE__` is consumed by `fetchWithAuth` in
// dashboard.html to prepend onto every `/monitoring/...` request.
//
// Mounted at both `/monitoring/dashboard` and `/dashboard.html`. The
// prefix is recovered by stripping whichever suffix matches the
// request path.
func handleDashboardHTML() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		prefix := ""
		switch {
		case strings.HasSuffix(path, "/monitoring/dashboard"):
			prefix = strings.TrimSuffix(path, "/monitoring/dashboard")
		case strings.HasSuffix(path, "/dashboard.html"):
			prefix = strings.TrimSuffix(path, "/dashboard.html")
		}
		body := strings.ReplaceAll(dashboardHTML, "__FC_API_BASE__", prefix)

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body)) //nolint:gosec // G705: static dashboard HTML with the matched router mount-path prefix substituted, not free user input
	}
}
