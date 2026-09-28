// route.go is the function-runner public-routes model (docs/function-runner
// -plan.md §8, "public routes"): a function's own public route rows —
// hostname + path prefix → this function's live version, or a named alias.
// The runner's public entry (internal/functions/runner/public.go) matches a
// request by Host and the longest whole-segment path prefix, stripping the
// prefix before the function's own endpoint matching — see
// internal/functions/control.Route, the wire shape this mirrors.
package function

import (
	"regexp"
	"strings"
	"time"
)

// HostnamePattern is the route/zone hostname shape: one or more
// dot-separated DNS labels (lowercase letters/digits/hyphens, 1-63 chars
// each, no leading/trailing hyphen). No port (":" is outside the character
// class), no wildcard ("*" likewise).
var HostnamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$`)

// MaxHostnameLength is the DNS name length ceiling (RFC 1035), matching
// functiondomain.MaxZoneLength.
const MaxHostnameLength = 255

// ValidHostname reports whether h is a lowercase DNS name with no port and
// no wildcard, at most MaxHostnameLength bytes.
func ValidHostname(h string) bool {
	return h != "" && len(h) <= MaxHostnameLength && HostnamePattern.MatchString(h)
}

// NormalizeHostname lowercases h and trims a trailing dot — the same
// normalisation internal/functions/runner/public.go's buildRoutes/match
// apply, so a stored route matches exactly what the runner will look up.
func NormalizeHostname(h string) string {
	return strings.ToLower(strings.TrimSuffix(strings.TrimSpace(h), "."))
}

// NormalizePathPrefix normalises p to a "/"-prefixed path with no trailing
// slash, or exactly "/" for the root prefix — matching how
// internal/functions/runner/public.go's buildRoutes treats a stored prefix
// (`"/" + strings.Trim(rt.PathPrefix, "/")`), so the platform stores routes
// in exactly the shape the runner will apply at match time.
func NormalizePathPrefix(p string) string {
	trimmed := strings.Trim(strings.TrimSpace(p), "/")
	if trimmed == "" {
		return "/"
	}
	return "/" + trimmed
}

// Route is one public route: (hostname, pathPrefix) -> this function's live
// version, or a named alias. Table: fng_routes.
type Route struct {
	ID         string
	FunctionID string
	Hostname   string
	PathPrefix string
	// Alias names a non-live alias to route to; nil/"" means live.
	Alias     *string
	CreatedBy *string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// IDStr satisfies usecase.HasID.
func (r Route) IDStr() string { return r.ID }
