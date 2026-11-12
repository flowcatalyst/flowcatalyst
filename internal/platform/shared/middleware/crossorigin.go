package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// cookieSessionKey marks a request whose AuthContext the Authenticator built
// from the fc_session cookie (not from an Authorization header).
type cookieSessionKey struct{}

// AuthenticatedBySessionCookie reports whether the request's AuthContext came
// from the fc_session cookie. False for bearer-authenticated requests, for the
// dev test headers, and for a request whose cookie was stale or invalid (it
// authenticated nothing).
func AuthenticatedBySessionCookie(ctx context.Context) bool {
	v, _ := ctx.Value(cookieSessionKey{}).(bool)
	return v
}

// WithSessionCookieAuth records on ctx that its AuthContext came from the
// fc_session cookie. The Authenticator calls it; another authenticator that
// accepts the cookie, or a test standing in for one, does the same.
func WithSessionCookieAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, cookieSessionKey{}, true)
}

// bearerAuthKey marks a request whose AuthContext the Authenticator built from
// an Authorization: Bearer token.
type bearerAuthKey struct{}

// AuthenticatedByBearer reports whether the request's AuthContext came from an
// Authorization: Bearer API token. False for the cookie, for the dev test
// headers and for a context a test injected directly.
func AuthenticatedByBearer(ctx context.Context) bool {
	v, _ := ctx.Value(bearerAuthKey{}).(bool)
	return v
}

// WithBearerAuth records on ctx that its AuthContext came from an Authorization:
// Bearer token. The Authenticator calls it.
func WithBearerAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, bearerAuthKey{}, true)
}

// CodeSessionRequired is the wire code of the refusal an API token gets on a
// route that manages the principal's own sign-in credentials.
const CodeSessionRequired = "SESSION_REQUIRED"

// RefuseBearer returns the 403 SESSION_REQUIRED error when ctx's AuthContext came from
// an Authorization: Bearer token, nil otherwise. The rule is "refuse when
// Bearer-authenticated", not "require the cookie": dev test headers and contexts a
// test injects directly stay admitted. Routes that mint a session-grade token or
// change how the principal signs in call it first.
func RefuseBearer(ctx context.Context) error {
	if !AuthenticatedByBearer(ctx) {
		return nil
	}
	return usecase.Authorization(CodeSessionRequired, "This action needs a signed-in browser session; an API token cannot perform it")
}

// sessionInfoKey carries what the Authenticator knows about the fc_session
// cookie that established the request: when it was issued, and how recent a
// sign-in the platform currently demands for changing sign-in methods.
type sessionInfoKey struct{}

type sessionInfo struct {
	issuedAt     time.Time
	stepUpMaxAge time.Duration
}

// WithSessionInfo records the cookie session's issued-at and the configured
// step-up window on ctx. The Authenticator calls it for cookie-authenticated requests.
func WithSessionInfo(ctx context.Context, issuedAt time.Time, stepUpMaxAge time.Duration) context.Context {
	return context.WithValue(ctx, sessionInfoKey{}, sessionInfo{issuedAt: issuedAt, stepUpMaxAge: stepUpMaxAge})
}

// SessionIssuedAt returns the issued-at of the fc_session cookie that
// authenticated the request. ok is false for any other kind of request.
func SessionIssuedAt(ctx context.Context) (time.Time, bool) {
	si, ok := ctx.Value(sessionInfoKey{}).(sessionInfo)
	return si.issuedAt, ok
}

// CodeReauthRequired is the wire code of the refusal an old browser session gets
// on a route that changes how the principal signs in.
const CodeReauthRequired = "REAUTH_REQUIRED"

// RequireRecentSignIn returns the 403 REAUTH_REQUIRED error when ctx's AuthContext
// came from an fc_session cookie issued longer ago than the step-up window
// (FC_AUTH_STEPUP_MAX_AGE_SECS), nil otherwise. A window of 0 disables the check.
// Like RefuseBearer it keys on how the request was authenticated, so Bearer, dev
// test headers and contexts a test injects directly are not affected. Routes that
// add or remove a sign-in method call it right after RefuseBearer: a session
// captured earlier must not be enough to bind a new passkey or factor.
func RequireRecentSignIn(ctx context.Context) error {
	if !AuthenticatedBySessionCookie(ctx) {
		return nil
	}
	si, ok := ctx.Value(sessionInfoKey{}).(sessionInfo)
	if !ok || si.stepUpMaxAge <= 0 {
		return nil
	}
	if time.Since(si.issuedAt) <= si.stepUpMaxAge {
		return nil
	}
	return usecase.Authorization(CodeReauthRequired, "Sign in again to change your sign-in methods")
}

// Wire codes of the session guard's refusals.
const (
	CodeCrossOriginRequest   = "CROSS_ORIGIN_REQUEST"
	CodeUnsupportedMediaType = "UNSUPPORTED_MEDIA_TYPE"
)

// errNoProvenance: an unsafe cookie-authenticated request carried neither
// Sec-Fetch-Site nor Origin. Every browser sends at least one of them on a
// POST, PUT, PATCH or DELETE, so the request did not come from a browser and
// has no business presenting the session cookie; scripts use a bearer token.
var errNoProvenance = errors.New("unsafe request without Sec-Fetch-Site or Origin")

// SessionCSRF guards the requests the browser's session cookie authenticates.
// It runs after Authenticator and applies only when
// AuthenticatedBySessionCookie is true and the method is unsafe (anything but
// GET, HEAD, OPTIONS):
//
//  1. Provenance (net/http.CrossOriginProtection): Sec-Fetch-Site must be
//     "same-origin" or "none" when present; without it, the Origin header
//     must name the request's Host or one of trustedOrigins (the public base
//     URL, for a proxy that rewrites Host). A request with neither header is
//     refused too, which CrossOriginProtection alone would let through. 403
//     CROSS_ORIGIN_REQUEST.
//  2. JSON only: a request with a body must declare application/json (or a
//     +json type). text/plain, form and multipart bodies (what a page on
//     another origin can send without a CORS preflight) and a body without a
//     Content-Type are refused. 415 UNSUPPORTED_MEDIA_TYPE.
//
// SameSite=Lax already keeps the cookie off cross-site POSTs; this covers
// what Lax does not: sibling subdomains of the same registrable domain
// (same-site, so the cookie is sent) and browsers that ignore SameSite.
//
// Requests authenticated by a bearer token (SDKs, service accounts, the
// router) are not affected: a page on another origin cannot attach an
// Authorization header without a preflight the platform never grants. The
// OAuth/OIDC endpoints and the dispatch callbacks are mounted outside the
// authenticated group and do not pass through this guard.
func SessionCSRF(trustedOrigins []string) (func(http.Handler) http.Handler, error) {
	check, err := SessionCSRFChecker(trustedOrigins)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if safeMethod(r.Method) || !AuthenticatedBySessionCookie(r.Context()) {
				next.ServeHTTP(w, r)
				return
			}
			if ref := check(r); ref != nil {
				writeEnvelope(w, ref.Status, ref.Code, ref.Message)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// CSRFRefusal is a request the session guard refused: the status, the wire code
// (CodeCrossOriginRequest or CodeUnsupportedMediaType) and the message.
type CSRFRefusal struct {
	Status        int
	Code, Message string
}

// SessionCSRFChecker is the session guard's rule (see [SessionCSRF]) for a caller that
// is not a chi middleware, such as the push gateway's authenticator. The function it
// returns is for a request the fc_session cookie authenticates; it answers nil for a safe
// method or an acceptable request, else the refusal. SessionCSRF applies it.
func SessionCSRFChecker(trustedOrigins []string) (func(*http.Request) *CSRFRefusal, error) {
	cop, err := newCrossOriginProtection(trustedOrigins)
	if err != nil {
		return nil, err
	}
	return func(r *http.Request) *CSRFRefusal {
		if safeMethod(r.Method) {
			return nil
		}
		err := cop.Check(r)
		if err == nil && r.Header.Get("Sec-Fetch-Site") == "" && r.Header.Get("Origin") == "" {
			err = errNoProvenance
		}
		if err != nil {
			slog.WarnContext(r.Context(), "cookie-session request refused: not from this origin",
				"method", r.Method, "path", r.URL.Path, "origin", r.Header.Get("Origin"),
				"secFetchSite", r.Header.Get("Sec-Fetch-Site"), "err", err)
			return &CSRFRefusal{
				http.StatusForbidden, CodeCrossOriginRequest,
				"This request was refused because it did not come from this site. " +
					"Requests authenticated by the session cookie must be sent by the platform's own pages; " +
					"API clients authenticate with a bearer token.",
			}
		}
		if hasBody(r) && !isJSONMediaType(r.Header.Get("Content-Type")) {
			return &CSRFRefusal{
				http.StatusUnsupportedMediaType, CodeUnsupportedMediaType,
				"Requests authenticated by the session cookie must send a JSON body (Content-Type: application/json).",
			}
		}
		return nil
	}, nil
}

// CredentialCSRF guards the public routes that sign a browser in or set its
// credentials (password login, the 2FA steps that finish it, passkey sign-in,
// the password-reset and set-password flow, the portal password login): login
// CSRF, where a page on another site makes the victim's browser sign in to the
// attacker's account. It applies to every unsafe request, authenticated or
// not:
//
//  1. Provenance (net/http.CrossOriginProtection, unchanged): a request a
//     browser marks as cross-origin (Sec-Fetch-Site other than same-origin or
//     none, or else an Origin naming neither the request's Host nor a trusted
//     origin) is refused, 403 CROSS_ORIGIN_REQUEST. A request with neither
//     header passes: it did not come from a browser, and login CSRF needs the
//     victim's browser (every browser sends Origin on a cross-origin POST).
//     This is where it differs from SessionCSRF, which refuses that case
//     because there the request presents the browser's session cookie.
//  2. JSON only, for every caller: a request with a body must declare
//     application/json (or a +json type), else 415 UNSUPPORTED_MEDIA_TYPE.
//     A cross-origin page can send only text/plain, form and multipart bodies
//     (or none) without a CORS preflight, and the platform answers none.
func CredentialCSRF(trustedOrigins []string) (func(http.Handler) http.Handler, error) {
	cop, err := newCrossOriginProtection(trustedOrigins)
	if err != nil {
		return nil, err
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if safeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			if err := cop.Check(r); err != nil {
				slog.WarnContext(r.Context(), "sign-in request refused: not from this origin",
					"method", r.Method, "path", r.URL.Path, "origin", r.Header.Get("Origin"),
					"secFetchSite", r.Header.Get("Sec-Fetch-Site"), "err", err)
				writeEnvelope(w, http.StatusForbidden, CodeCrossOriginRequest,
					"This request was refused because it did not come from this site. "+
						"Sign-in and credential requests must be sent by the platform's own pages.")
				return
			}
			if hasBody(r) && !isJSONMediaType(r.Header.Get("Content-Type")) {
				writeEnvelope(w, http.StatusUnsupportedMediaType, CodeUnsupportedMediaType,
					"This endpoint accepts only a JSON body (Content-Type: application/json).")
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// newCrossOriginProtection builds the provenance check both guards use, with
// trustedOrigins (the public base URL) accepted as Origin for a proxy that
// rewrites Host.
func newCrossOriginProtection(trustedOrigins []string) (*http.CrossOriginProtection, error) {
	cop := http.NewCrossOriginProtection()
	for _, o := range trustedOrigins {
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("middleware: trusted origin %q is not an http(s) origin", o)
		}
		if err := cop.AddTrustedOrigin(u.Scheme + "://" + u.Host); err != nil {
			return nil, fmt.Errorf("middleware: trusted origin %q: %w", o, err)
		}
	}
	return cop, nil
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

// hasBody reports whether the request carries (or may carry) a body: a
// non-zero Content-Length, or an unknown one (-1, chunked).
func hasBody(r *http.Request) bool {
	return r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0
}

// isJSONMediaType accepts application/json and any application/*+json.
func isJSONMediaType(ct string) bool {
	if ct == "" {
		return false
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	return mt == "application/json" || (strings.HasPrefix(mt, "application/") && strings.HasSuffix(mt, "+json"))
}

func writeEnvelope(w http.ResponseWriter, status int, code, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(httperror.Envelope{Code: code, Message: msg})
}
