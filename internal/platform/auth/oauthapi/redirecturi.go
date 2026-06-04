package oauthapi

import (
	"net/url"
	"strings"
)

// MatchRedirectURI reports whether uri is permitted by any registered
// redirect-URI pattern. It is the single matcher for BOTH the OAuth
// /oauth/authorize redirect_uri and the OIDC post_logout_redirect_uri, so the
// two can never drift. It is URL-component-aware and strict:
//
//   - An exact string match always passes.
//   - Otherwise both sides are parsed; the scheme must be identical, the
//     incoming uri must carry NO userinfo (blocks "https://victim@evil.com"
//     host-confusion), and host + port are compared structurally.
//   - The host may use a wildcard confined to the leftmost DNS label — full
//     ("*.inhanceapps.com") or partial ("qa-*.inhanceapps.com") — over a
//     concrete base domain of >=2 labels. One pattern therefore covers every
//     tenant subdomain. The wildcard can never cross a dot, so
//     "*.inhanceapps.com" matches "app.inhanceapps.com" but NOT
//     "inhanceapps.com", "a.b.inhanceapps.com", "inhanceapps.com.evil.com", a
//     bare "https://*", or "https://*.com".
//   - Path: a pattern with no path (or "/") matches any path on the host; a
//     pattern ending in "/*" matches that path prefix; otherwise the path must
//     match exactly. The incoming uri's query/fragment are ignored (we append
//     our own state on redirect).
func MatchRedirectURI(uri string, registered []string) bool {
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

// hostMatchesPattern supports an exact host or a wildcard confined to the
// leftmost DNS label — full ("*.x.com") or partial ("qa-*.x.com", "*-qa.x.com").
// The wildcard can never cross a dot: each host is split at its first dot into
// (label, base); the base must match exactly and be a concrete >=2-label domain,
// and only the leftmost label is glob-matched.
func hostMatchesPattern(host, pattern string) bool {
	if !strings.Contains(pattern, "*") {
		return host == pattern
	}
	pLabel, pBase, ok := strings.Cut(pattern, ".")
	if !ok {
		return false // bare "*" — no base domain
	}
	if strings.Contains(pBase, "*") {
		return false // wildcards only permitted in the leftmost label
	}
	if strings.Count(pBase, ".") < 1 {
		return false // base must have >=2 labels (rejects "*.com", "qa-*.com")
	}
	uLabel, uBase, ok := strings.Cut(host, ".")
	if !ok || uBase != pBase {
		return false
	}
	return labelMatches(uLabel, pLabel)
}

// labelMatches reports whether a single dot-free DNS label matches a pattern
// that may contain '*' wildcards. Literal segments must appear in order and
// each '*' must consume at least one character (so "qa-*" matches "qa-foo" but
// not the bare "qa-"). The label must be non-empty.
func labelMatches(label, pattern string) bool {
	if label == "" {
		return false
	}
	if !strings.Contains(pattern, "*") {
		return label == pattern
	}
	segs := strings.Split(pattern, "*")
	rest, ok := strings.CutPrefix(label, segs[0])
	if !ok {
		return false
	}
	for i := 1; i < len(segs); i++ {
		seg := segs[i]
		if i == len(segs)-1 { // final literal segment (may be empty)
			if seg == "" {
				return rest != "" // trailing '*' must consume >=1 char
			}
			// '*' before the final literal must consume >=1 char.
			return strings.HasSuffix(rest, seg) && len(rest) > len(seg)
		}
		idx := strings.Index(rest, seg)
		if idx < 1 { // '*' before this literal must consume >=1 char
			return false
		}
		rest = rest[idx+len(seg):]
	}
	return true
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
