// Package consoleredirect is the platform HTTP server's fallback handler: it
// redirects legacy root and SPA browser URLs (/, /dashboard, /dispatch-jobs,
// /auth/login?..., etc.) to their server-rendered /ui equivalents while
// preserving JSON 404 envelopes on unknown API paths, plain-text 404 responses
// on missing static assets and unmatched /ui paths, and 405 on non-GET/HEAD
// requests.
//
// The route map lives in routes.go. Rules:
//   - paths under API prefixes (/api, /bff, /oauth, /.well-known, /control, /q)
//     answer 404 JSON ({error: "NOT_FOUND", message: "No route for ..."});
//   - methods other than GET and HEAD answer 405 (Allow: GET, HEAD);
//   - /ui, /ui/*, /assets, /assets/*, and static-asset extensions answer 404;
//   - all other GET and HEAD paths answer 302 to a Location starting "/ui/",
//     built from the route table plus escaped path segments (never from a query
//     value), with sign-in "redirect" query parameters translated to /ui (or
//     dropped) and Cache-Control: no-store.
package consoleredirect

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
	"strings"
)

const (
	root = "/ui/"
	// maxPath bounds the request path considered; longer is not a page.
	maxPath = 2048
	// maxSegment bounds one copied segment.
	maxSegment = 256

	contentSecurityPolicy = "frame-ancestors 'none'; object-src 'none'; base-uri 'self'; form-action 'self'"
)

// apiPrefixes are the first path segments owned by platform APIs rather than
// browser page routes.
var apiPrefixes = []string{"api", "bff", "oauth", ".well-known", "control", "q"}

// IsAPIPath reports whether p (a URL path) lies under one of the platform API
// prefixes, matched by whole first segment (/apiary is not /api).
func IsAPIPath(p string) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(p, "/"), "/")
	return slices.Contains(apiPrefixes, first)
}

// assetExtensions are the file types a missing path should 404 for rather than
// redirect to /ui/. An explicit allow-list rather than "has a dot": route
// parameters (roles, domains, identifiers) may contain dots.
var assetExtensions = map[string]bool{
	".js": true, ".mjs": true, ".cjs": true, ".css": true, ".map": true,
	".json": true, ".webmanifest": true, ".txt": true, ".xml": true, ".wasm": true,
	".html": true, ".htm": true,
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".svg": true, ".ico": true,
	".webp": true, ".avif": true, ".bmp": true,
	".woff": true, ".woff2": true, ".ttf": true, ".otf": true, ".eot": true,
}

func isAssetPath(p string) bool {
	reqPath := strings.TrimPrefix(path.Clean("/"+p), "/")
	if reqPath == "" || reqPath == "index.html" {
		return false
	}
	return reqPath == "assets" || strings.HasPrefix(reqPath, "assets/") || assetExtensions[strings.ToLower(path.Ext(reqPath))]
}

func setSecurityHeaders(h http.Header) {
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "same-origin")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", contentSecurityPolicy)
}

// Handler returns the standalone fallback http.Handler for platform servers.
func Handler() http.Handler {
	return http.HandlerFunc(serveFallback)
}

func serveFallback(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())

	if IsAPIPath(r.URL.Path) {
		apiNotFound(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		plainStatus(w, r, http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	if p == "/ui" || strings.HasPrefix(p, "/ui/") || isAssetPath(p) {
		plainStatus(w, r, http.StatusNotFound)
		return
	}
	w.Header().Set("Location", Target(r.URL.EscapedPath(), r.URL.RawQuery))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusFound)
}

func plainStatus(w http.ResponseWriter, r *http.Request, status int) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = io.WriteString(w, http.StatusText(status)+"\n")
	}
}

func apiNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNotFound)
	if r.Method != http.MethodHead {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":   "NOT_FOUND",
			"message": "No route for " + r.Method + " " + r.URL.Path,
		})
	}
}

// Target maps an escaped legacy SPA request path and raw query to the /ui
// Location. A path that matches no specific route maps to /ui/ (matching the
// former SPA catch-all to the landing page).
func Target(escapedPath, rawQuery string) string {
	ui, ok := mapPath(escapedPath)
	if !ok {
		return root
	}
	if strings.HasPrefix(ui, "/ui/auth/") && rawQuery != "" {
		rawQuery = translateRedirect(rawQuery)
	}
	if rawQuery == "" {
		return ui
	}
	return ui + "?" + rawQuery
}

// mapPath applies the route table to an escaped path.
func mapPath(p string) (string, bool) {
	if len(p) > maxPath || p == "" || p[0] != '/' {
		return "", false
	}
	if p != "/" {
		p = strings.TrimSuffix(p, "/")
	}
	var segs []string
	if p != "/" {
		segs = strings.Split(p[1:], "/")
	}
	if slices.Contains(segs, "") {
		return "", false
	}
	for _, rt := range routes {
		if params, ok := rt.match(segs); ok {
			return rt.build(params), true
		}
	}
	return "", false
}

// translateRedirect rewrites the sign-in "redirect" parameter in a raw query
// to its /ui page, or drops it when it names no page. Every other parameter
// is left byte for byte.
func translateRedirect(rawQuery string) string {
	parts := strings.Split(rawQuery, "&")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		k, v, _ := strings.Cut(part, "=")
		if key, err := url.QueryUnescape(k); err != nil || key != "redirect" {
			out = append(out, part)
			continue
		}
		val, err := url.QueryUnescape(v)
		if err != nil {
			continue
		}
		if t := redirectValue(val); t != "" {
			out = append(out, "redirect="+url.QueryEscape(t))
		}
	}
	return strings.Join(out, "&")
}

// redirectValue translates a sign-in return path: a legacy SPA page maps
// through the table (its own query kept), a /ui path stays for /ui to
// validate, and anything else is refused.
func redirectValue(val string) string {
	if val == "" || val[0] != '/' || strings.HasPrefix(val, "//") || len(val) > maxPath {
		return ""
	}
	for i := 0; i < len(val); i++ {
		if val[i] < 0x20 || val[i] == 0x7f || val[i] == '\\' {
			return ""
		}
	}
	u, err := url.Parse(val)
	if err != nil || u.Scheme != "" || u.Host != "" {
		return ""
	}
	if u.Path == "/ui" || strings.HasPrefix(u.Path, "/ui/") {
		return val
	}
	ui, ok := mapPath(u.EscapedPath())
	if !ok {
		return ""
	}
	if u.RawQuery != "" {
		ui += "?" + u.RawQuery
	}
	return ui
}
