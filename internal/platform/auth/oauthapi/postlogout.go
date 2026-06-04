package oauthapi

import (
	"net/url"
	"strings"
)

// MatchPostLogoutRedirectURI reports whether uri is permitted by any registered
// post_logout_redirect_uri pattern. It is deliberately stricter and more
// URL-aware than the legacy redirect_uri matcher (matchesRedirectURI), which is
// kept as-is for /oauth/authorize parity:
//
//   - An exact string match always passes.
//   - Otherwise both sides are parsed; the scheme must be identical, the
//     incoming uri must carry NO userinfo (blocks "https://victim@evil.com"
//     host-confusion), and host + port are compared structurally.
//   - The host may use a single leftmost "*." wildcard standing for exactly one
//     non-empty DNS label over a concrete base domain of >=2 labels. So
//     "https://*.inhanceapps.com" matches "https://app.inhanceapps.com" but NOT
//     "https://inhanceapps.com", "https://a.b.inhanceapps.com",
//     "https://inhanceapps.com.evil.com", a bare "https://*", or "https://*.com".
//   - Path: a pattern with no path (or "/") matches any path on the host; a
//     pattern ending in "/*" matches that path prefix; otherwise the path must
//     match exactly. The incoming uri's query/fragment are ignored (we append
//     our own state on redirect).
func MatchPostLogoutRedirectURI(uri string, registered []string) bool {
	for _, r := range registered {
		if r == uri {
			return true
		}
	}
	u, err := url.Parse(uri)
	if err != nil || u.User != nil || u.Hostname() == "" || u.Scheme == "" {
		return false
	}
	for _, pattern := range registered {
		if !strings.Contains(pattern, "*") {
			continue // non-wildcard patterns only match via the exact pass above
		}
		if matchOnePostLogout(u, pattern) {
			return true
		}
	}
	return false
}

func matchOnePostLogout(u *url.URL, pattern string) bool {
	p, err := url.Parse(pattern)
	if err != nil || p.Hostname() == "" {
		return false
	}
	if !strings.EqualFold(u.Scheme, p.Scheme) {
		return false
	}
	if u.Port() != p.Port() {
		return false
	}
	if !hostMatchesPattern(strings.ToLower(u.Hostname()), strings.ToLower(p.Hostname())) {
		return false
	}
	return pathMatchesPattern(u.EscapedPath(), p.EscapedPath())
}

// hostMatchesPattern supports an exact host or a single leftmost "*." label.
func hostMatchesPattern(host, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return host == pattern
	}
	// Only a leftmost "*." wildcard is permitted.
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}
	base := pattern[len("*."):]
	// base must be a concrete domain (no further wildcard) with >=2 labels,
	// so "*.inhanceapps.com" is allowed but "*.com" / "*" are not.
	if base == "" || strings.Contains(base, "*") || strings.Count(base, ".") < 1 {
		return false
	}
	suffix := "." + base
	if !strings.HasSuffix(host, suffix) {
		return false
	}
	label := host[:len(host)-len(suffix)]
	// exactly one non-empty label in front of the base domain.
	return label != "" && !strings.Contains(label, ".")
}

func pathMatchesPattern(uriPath, patternPath string) bool {
	if patternPath == "" || patternPath == "/" {
		return true // host-only pattern → any path on the host
	}
	if strings.HasSuffix(patternPath, "/*") {
		prefix := strings.TrimSuffix(patternPath, "*") // retain trailing slash
		return strings.HasPrefix(uriPath, prefix)
	}
	return uriPath == patternPath
}
