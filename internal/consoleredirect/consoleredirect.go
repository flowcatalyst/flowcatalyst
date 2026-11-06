// Package consoleredirect makes the server-rendered console (/ui) the
// default: it wraps the Vue SPA's fallback handler and answers each SPA page
// URL with a 302 to the equivalent /ui page. The SPA stays in the binary;
// FC_CONSOLE=spa turns the redirects off (docs/environment-variables.md).
//
// The map is the table in routes.go. Rules:
//   - only GET and HEAD are redirected; other methods, /ui itself, API paths
//     and the SPA's static files (frontend.ServesAsIs) pass through;
//   - the Location is always a path starting "/ui/", built from the table
//     plus path segments copied (still escaped) from the request, never from
//     a query value; the original query is appended unchanged except for the
//     sign-in "redirect" parameter, which is translated to its /ui page (or
//     dropped);
//   - the response is no-store so switching the setting back is immediate.
package consoleredirect

import (
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
)

// EnvVar is the setting: "ui" (default) redirects, "spa" serves the SPA.
const EnvVar = "FC_CONSOLE"

const (
	root = "/ui/"
	// maxPath bounds the request path considered; longer is not a page.
	maxPath = 2048
	// maxSegment bounds one copied segment.
	maxSegment = 256
)

// Enabled reads FC_CONSOLE. Anything but "spa" (any case) redirects; an
// unrecognised value is logged and treated as "ui".
func Enabled() bool {
	v := strings.ToLower(strings.TrimSpace(envutil.Or(EnvVar, "ui")))
	switch v {
	case "spa":
		return false
	case "ui", "":
		return true
	}
	slog.Warn("unknown "+EnvVar+" value; using ui", "value", v)
	return true
}

// Wrap redirects SPA page URLs to /ui when FC_CONSOLE allows, else returns
// spa unchanged. isStatic reports paths spa answers from files
// (frontend.ServesAsIs).
func Wrap(spa http.Handler, isStatic func(string) bool) http.Handler {
	if !Enabled() {
		slog.Info("console: serving the Vue SPA (" + EnvVar + "=spa)")
		return spa
	}
	slog.Info("console: SPA page URLs redirect to /ui (" + EnvVar + "=spa turns this off)")
	return New(spa, isStatic)
}

// New is Wrap without the environment check.
func New(spa http.Handler, isStatic func(string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			spa.ServeHTTP(w, r)
			return
		}
		p := r.URL.Path
		if p == "/ui" || strings.HasPrefix(p, "/ui/") || (isStatic != nil && isStatic(p)) {
			spa.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Location", Target(r.URL.EscapedPath(), r.URL.RawQuery))
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusFound)
	})
}

// Target maps an escaped SPA request path and raw query to the /ui Location.
// A path that is no SPA page maps to /ui/ (the SPA's own catch-all sent it to
// the dashboard).
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

// redirectValue translates a sign-in return path: an SPA page maps through
// the table (its own query kept), a /ui path stays for /ui to validate, and
// anything else is refused.
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
