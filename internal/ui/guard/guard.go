// Package guard holds the request guards of the server-rendered UI (/ui):
// cross-site request forgery protection, the session requirement with
// HTML-vs-Datastar negotiation, the role-less profile gate and the security
// headers. The design (why cookie sessions are safe here) is in
// docs/ui/README.md, "CSRF".
package guard

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
)

// Denier renders a refusal (an HTML page for navigations, a bare status for
// Datastar requests). The UI's shell provides it so this package does not
// depend on the templates.
type Denier func(w http.ResponseWriter, r *http.Request, status int, message string)

// PlainDenier is a minimal Denier (text/plain), for tests and as a fallback.
func PlainDenier(w http.ResponseWriter, _ *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = fmt.Fprintln(w, message)
}

// ErrNoProvenance: an unsafe request carried neither Sec-Fetch-Site nor
// Origin. Every browser sends at least one of them on a POST (form or
// fetch), so such a request is not from a browser and has no business
// presenting a session cookie to the UI; scripts use /api with a bearer token.
var ErrNoProvenance = errors.New("cross-origin protection: unsafe request without Sec-Fetch-Site or Origin")

// CSRF rejects cross-site state-changing requests (anything but GET, HEAD and
// OPTIONS) before they reach a handler:
//
//  1. Sec-Fetch-Site must be "same-origin" (or "none": user-initiated, e.g. a
//     bookmark) when the browser sends it (all current browsers do);
//  2. otherwise the Origin header must match the request's Host (or one of
//     trustedOrigins, e.g. the public base URL behind a proxy that rewrites Host);
//  3. a request with neither header is refused (ErrNoProvenance); this is
//     stricter than net/http.CrossOriginProtection, which lets it through as
//     "not a browser".
//
// Steps 1 and 2 are net/http.CrossOriginProtection. It complements the
// SameSite=Lax session cookie (which already keeps the cookie off cross-site
// POSTs and fetches) and covers what Lax does not: same-site sibling
// subdomains and browsers that ignore SameSite. GET handlers must not change
// state; the UI's routing convention enforces that (mutations are POST/PUT/
// PATCH/DELETE only).
func CSRF(trustedOrigins []string, deny Denier) (func(http.Handler) http.Handler, error) {
	cop := http.NewCrossOriginProtection()
	for _, o := range trustedOrigins {
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("guard: trusted origin %q is not an http(s) origin", o)
		}
		if err := cop.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
			return nil, fmt.Errorf("guard: trusted origin %q: %w", o, err)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := cop.Check(r)
			if err == nil && !safeMethod(r.Method) &&
				r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Origin") == "" {
				err = ErrNoProvenance
			}
			if err != nil {
				slog.WarnContext(r.Context(), "ui: cross-origin request refused",
					"method", r.Method, "path", r.URL.Path, "origin", r.Header.Get("Origin"),
					"secFetchSite", r.Header.Get("Sec-Fetch-Site"), "err", err)
				deny(w, r, http.StatusForbidden, "This request was blocked because it did not come from this site.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// LoginPath is where an unauthenticated navigation is sent.
const LoginPath = "/auth/login"

// IsHTMLNavigation reports whether r is a top-level browser navigation that
// can follow a redirect to a login page: GET/HEAD, not a Datastar action,
// and accepting HTML (the TypeScript SDK's requireSession uses the same rule).
func IsHTMLNavigation(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	if ds.IsDatastar(r) {
		return false
	}
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// LoginRedirect is the login URL that returns to r's path afterwards.
func LoginRedirect(r *http.Request) string {
	return LoginPath + "?redirect=" + url.QueryEscape(r.URL.RequestURI())
}

// RequireSession lets authenticated requests through. Without a session, a
// browser navigation is redirected (302) to the login page; everything else
// (Datastar actions, fetches, form posts) gets 401, which the shell turns
// into a navigation to the login page (data-on:datastar-fetch on <body>).
func RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth.FromContext(r.Context()) != nil {
			next.ServeHTTP(w, r)
			return
		}
		if IsHTMLNavigation(r) {
			http.Redirect(w, r, LoginRedirect(r), http.StatusFound) //nolint:gosec // G710: the target is the constant LoginPath; the request URI is only its query-escaped redirect parameter
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = fmt.Fprintln(w, "Not signed in.")
	})
}

// ProfileOnly refuses a signed-in USER who holds no platform role (the same
// rule as the API's ProfileOnlyWithoutRole: such a user may only reach their
// own profile, which lives in the SPA).
func ProfileOnly(deny Denier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if auth.FromContext(r.Context()).IsRoleless() {
				deny(w, r, http.StatusForbidden, "Your account has no platform access. Only your profile is available.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ContentSecurityPolicy for UI pages. Datastar evaluates attribute
// expressions with the Function constructor, so 'unsafe-eval' is required;
// no inline <script> and no inline event handlers are allowed (the ds
// package's Redirect/ReplaceURL avoid scripts for that reason). Inline
// style attributes are used by the kit (display:none before Datastar loads,
// the select checkmark mask), hence 'unsafe-inline' for styles only.
const ContentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; font-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; " +
	"form-action 'self'; frame-ancestors 'none'"

// SecurityHeaders sets the UI's response headers: the CSP, no framing, no
// sniffing, no referrer leaks across origins, and no caching of pages that
// carry user data (assets are served outside this middleware).
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", ContentSecurityPolicy)
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
