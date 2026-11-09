package shell

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// logoHost is a host (and port) that may be written into a CSP source: letters,
// digits, dots and hyphens, an optional port. Anything else (a ';', a quote,
// a space) could add a directive, so such a logo is not allowed by origin.
var logoHost = regexp.MustCompile(`^[A-Za-z0-9.-]{1,253}(:[0-9]{1,5})?$`)

// logoOrigin is the https origin of a remote logo URL ("https://cdn.example"),
// "" for a same-site path, a data URL or anything that is not plain https.
func logoOrigin(raw string) string {
	if !strings.HasPrefix(strings.ToLower(raw), "https://") {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || !logoHost.MatchString(u.Host) {
		return ""
	}
	return "https://" + strings.ToLower(u.Host)
}

// allowLogoOrigin widens the page policy's img-src (guard.SecurityHeaders:
// img-src 'self' data:) by the remote logo's origin, and nothing else, so the
// sidebar can load a logo the theme serves from its own host, as the SPA's
// sidebar does. A page without a remote logo keeps the policy as it is.
func allowLogoOrigin(w http.ResponseWriter, logoURL string) {
	origin := logoOrigin(logoURL)
	if origin == "" {
		return
	}
	h := w.Header()
	if csp := h.Get("Content-Security-Policy"); csp != "" {
		h.Set("Content-Security-Policy", strings.Replace(csp, "img-src 'self' data:", "img-src 'self' data: "+origin, 1))
	}
}
