package pages

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// The /ui security page changes sign-in methods too, so it asks for the same recent
// sign-in the /auth/* endpoints do: an old browser session gets the server's message
// and a "Sign in again" link back to the page, and nothing is changed.

// cookieSessionMount marks the signed-in context as the Authenticator does for an
// fc_session cookie issued age ago, under a step-up window of window.
type cookieSessionMount struct {
	inner       mounter
	age, window time.Duration
}

func (m cookieSessionMount) Mount(r chi.Router) {
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := platformmw.WithSessionCookieAuth(req.Context())
			ctx = platformmw.WithSessionInfo(ctx, time.Now().Add(-m.age), m.window)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	m.inner.Mount(r)
}

func TestSecurityPageWritesNeedARecentSignIn(t *testing.T) {
	actions := []struct{ name, target string }{
		{"add authenticator app", "/ui/auth/security/2fa/totp"},
		{"add email code", "/ui/auth/security/2fa/email"},
		{"remove a method", "/ui/auth/security/2fa/TOTP/remove"},
		{"regenerate recovery codes", "/ui/auth/security/2fa/recovery-codes"},
		{"revoke a device", "/ui/auth/security/2fa/devices/abc/revoke"},
		{"remove a passkey", "/ui/auth/security/passkeys/pk1/remove"},
	}
	wantLink := `href="/ui/auth/login?redirect=%2Fui%2Fauth%2Fsecurity"`

	for _, a := range actions {
		t.Run(a.name+": old session refused", func(t *testing.T) {
			h := newMFAHarness()
			h.build()
			rec := serveMounted(t, mfaSession("prn_ann"), mfaPost(a.target, url.Values{}, nil),
				cookieSessionMount{inner: mfaMount{h.page}, age: 20 * time.Minute, window: 15 * time.Minute})
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status %d, want 403: %s", rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if !strings.Contains(body, "Sign in again to change your sign-in methods") {
				t.Errorf("the server's message is not on the page")
			}
			if !strings.Contains(body, wantLink) || !strings.Contains(body, ">Sign in again</a>") {
				t.Errorf("no \"Sign in again\" link back to the security page")
			}
			if len(h.mfa.calls) != 0 {
				t.Errorf("the refused action reached the two-factor service: %v", h.mfa.calls)
			}
		})
		t.Run(a.name+": recent session, zero window and injected context proceed", func(t *testing.T) {
			for name, m := range map[string]func(*mfaHarness) mounter{
				"recent": func(h *mfaHarness) mounter {
					return cookieSessionMount{inner: mfaMount{h.page}, age: time.Minute, window: 15 * time.Minute}
				},
				"window 0": func(h *mfaHarness) mounter {
					return cookieSessionMount{inner: mfaMount{h.page}, age: 100 * time.Hour}
				},
				"injected": func(h *mfaHarness) mounter { return mfaMount{h.page} },
			} {
				h := newMFAHarness()
				h.build()
				rec := serveMounted(t, mfaSession("prn_ann"), mfaPost(a.target, url.Values{}, nil), m(h))
				if rec.Code == http.StatusForbidden || strings.Contains(rec.Body.String(), "Sign in again") {
					t.Errorf("%s: refused: %d %s", name, rec.Code, rec.Body.String())
				}
			}
		})
	}
}

// A successful password change replaces this browser's session (the old one ended
// with the old password); the page must pass that Set-Cookie on or the user is
// signed out by their own change.
func TestProfilePasswordChangePassesOnTheReplacementSession(t *testing.T) {
	h := newProfileHarness()
	inner := h.up
	h.up = nil // the wrapper below is the upstream
	page := &Profile{
		Upstream: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/auth/change-password" {
				w.Header().Add("Set-Cookie", "fc_session=fresh-session-token; Path=/; HttpOnly")
			}
			inner.ServeHTTP(w, r)
		}),
		Principals: h.dev, Credentials: h.creds, Security: h.mfa.build(), Shell: shellForTest(),
	}
	rec := serveMounted(t, profileSession(),
		profilePost("/ui/profile/password", passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")),
		profileMount{page})
	mfaWantRedirect(t, rec, "/ui/profile")
	c := mfaCookie(rec, "fc_session")
	if c == nil || c.Value != "fresh-session-token" {
		t.Fatalf("the replacement session cookie was not passed on: %v", rec.Result().Header.Values("Set-Cookie"))
	}
}
