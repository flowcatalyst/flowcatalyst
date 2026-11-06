package pages

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	wa "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/uiassets"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// fakeAuthEndpoints stands in for the platform's /auth/* endpoints the page
// calls in-process: it answers from the table and records what it was sent.
type fakeAuthEndpoints struct {
	mu    sync.Mutex
	me    map[string]any
	calls []string
	// bodies are the JSON bodies posted, by path.
	bodies  map[string]map[string]any
	answers map[string]fakeAnswer
	cookies string
}

type fakeAnswer struct {
	status int
	body   any
}

func newFakeAuthEndpoints() *fakeAuthEndpoints {
	return &fakeAuthEndpoints{
		me:      map[string]any{"principalId": "prn_ann", "name": "Ann Lee", "email": "ann@x.test", "roles": []string{"platform:admin", "app:viewer"}, "ssoManaged": false},
		bodies:  map[string]map[string]any{},
		answers: map[string]fakeAnswer{},
	}
}

func (f *fakeAuthEndpoints) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.cookies = r.Header.Get("Cookie")
	if r.Body != nil {
		var b map[string]any
		if json.NewDecoder(r.Body).Decode(&b) == nil {
			f.bodies[r.URL.Path] = b
		}
	}
	a, ok := f.answers[r.Method+" "+r.URL.Path]
	if !ok {
		switch r.URL.Path {
		case "/auth/me":
			a = fakeAnswer{200, f.me}
		case "/auth/change-password":
			a = fakeAnswer{200, map[string]any{"message": "Your password has been changed."}}
		case "/auth/change-password/send-email-code":
			a = fakeAnswer{200, map[string]any{"message": "A code has been sent to your email."}}
		case "/auth/login-history":
			a = fakeAnswer{200, map[string]any{"attempts": []any{}}}
		default:
			a = fakeAnswer{404, map[string]any{"message": "no such endpoint"}}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(a.status)
	_ = json.NewEncoder(w).Encode(a.body)
}

func (f *fakeAuthEndpoints) called(call string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Contains(f.calls, call)
}

type profileHarness struct {
	up    *fakeAuthEndpoints
	mfa   *mfaHarness
	dev   *fakeDevUsers
	creds *fakeDevUserOps
}

func newProfileHarness() *profileHarness {
	return &profileHarness{
		up: newFakeAuthEndpoints(), mfa: newMFAHarness(),
		dev:   &fakeDevUsers{ps: []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", nil, false, "")}},
		creds: &fakeDevUserOps{secret: testSecret},
	}
}

type profileMount struct{ *Profile }

func (m profileMount) Mount(r chi.Router) {
	r.Use(guard.SecurityHeaders)
	m.Profile.Mount(r)
}

func (h *profileHarness) serve(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	page := &Profile{Upstream: h.up, Principals: h.dev, Credentials: h.creds, Security: h.mfa.build(), Shell: shellForTest()}
	return serveMounted(t, ac, q, profileMount{page})
}

func profileSession() *auth.AuthContext { return mfaSession("prn_ann") }

func profileDevSession() *auth.AuthContext {
	ac := mfaSession("prn_ann")
	ac.Permissions = []string{"platform:developer:api-credential:manage"}
	return ac
}

func profilePost(target string, form url.Values) req {
	return mfaPost(target, form, nil)
}

func passwordForm(cur, nw, conf string) url.Values {
	return url.Values{"currentPassword": {cur}, "newPassword": {nw}, "confirmPassword": {conf}}
}

func TestProfileShowsTheUser(t *testing.T) {
	h := newProfileHarness()
	h.mfa.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	name := "Work laptop"
	h.mfa.keys.keys["prn_ann"] = []wa.Credential{{ID: "pk_ann", PrincipalID: "prn_ann", Name: &name, CreatedAt: time.Now()}}
	rec := h.serve(t, profileSession(), req{target: "/ui/profile"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	body := rec.Body.String()
	mustContain(t, body, "<title>Profile · FlowCatalyst</title>", "User Information", "Account Settings", "Ann Lee", "ann@x.test",
		"platform:admin", "app:viewer", ">AL<", "Work laptop", "/ui/auth/security/passkeys/pk_ann/remove", "Change Password", `action="/ui/profile/password"`,
		"Passkeys", "Two-Factor Authentication", "Authenticator app", "Security", "Sign-in Activity", "/ui/profile?activity=1",
		uiassets.URL("fc-webauthn.js"), `data-done="/ui/profile?added=1"`)
	// Display name and email are read-only, as in the SPA.
	mustContain(t, body, `id="profile-name"`, `id="profile-email"`)
	mustNotContain(t, body, "Developer API Access", "Your password is managed", testSecret)
	if !h.up.called("GET /auth/me") {
		t.Errorf("the profile was not read from /auth/me: %v", h.up.calls)
	}
}

func TestProfileForwardsTheSessionCookie(t *testing.T) {
	h := newProfileHarness()
	page := &Profile{Upstream: h.up, Principals: h.dev, Credentials: h.creds, Security: h.mfa.build(), Shell: shellForTest()}
	r := httptest.NewRequest(http.MethodGet, "/ui/profile", nil)
	r.AddCookie(&http.Cookie{Name: "fc_session", Value: "JWT"})
	r = r.WithContext(auth.WithContext(r.Context(), profileSession()))
	rec := httptest.NewRecorder()
	mux := chi.NewRouter()
	mux.Route("/ui", page.Mount)
	mux.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK || !strings.Contains(h.up.cookies, "fc_session=JWT") {
		t.Fatalf("status %d, upstream cookies %q", rec.Code, h.up.cookies)
	}
}

func TestProfileSSOUserCannotChangePassword(t *testing.T) {
	h := newProfileHarness()
	h.up.me["ssoManaged"] = true
	body := h.serve(t, profileSession(), req{target: "/ui/profile"}).Body.String()
	mustContain(t, body, "Your password is managed by your organisation")
	mustNotContain(t, body, "Change Password", `action="/ui/profile/password"`)

	// The endpoint refuses too: a posted form reaches no upstream change.
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")))
	if rec.Code != http.StatusUnprocessableEntity || h.up.called("POST /auth/change-password") {
		t.Fatalf("status %d, calls %v", rec.Code, h.up.calls)
	}
}

func TestProfilePageWithoutPasskeyStoreOmitsPasskeys(t *testing.T) {
	h := newProfileHarness()
	page := &Profile{Upstream: h.up, Principals: h.dev, Credentials: h.creds, Shell: shellForTest()}
	body := serveMounted(t, profileSession(), req{target: "/ui/profile"}, profileMount{page}).Body.String()
	mustContain(t, body, "Account Settings", "Sign-in Activity")
	mustNotContain(t, body, "Two-Factor Authentication", "Add a passkey")
}

func TestProfileDeveloperCredentialVisibility(t *testing.T) {
	h := newProfileHarness()
	h.dev.ps = []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", []string{developerRole}, false, "")}

	// Without the permission the section is absent, whatever the record says.
	mustNotContain(t, h.serve(t, profileSession(), req{target: "/ui/profile"}).Body.String(), "Developer API Access")

	// With it and no secret yet: set, no revoke or rotate.
	body := h.serve(t, profileDevSession(), req{target: "https://fc.test/ui/profile"}).Body.String()
	mustContain(t, body, "Developer API Access", "No credential set", "Set Secret", "client_id=prn_ann",
		"https://fc.test/oauth/token", "client_credentials", "Never")
	mustNotContain(t, body, "Rotate Secret", "confirm-dev-revoke", "Credential set<")

	// With a secret: revoke and rotate, each behind a confirmation.
	h.dev.ps = []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", []string{developerRole}, true, "")}
	body = h.serve(t, profileDevSession(), req{target: "/ui/profile"}).Body.String()
	mustContain(t, body, "Credential set", "Rotate Secret", "Revoke", "confirm-dev-revoke", "confirm-dev-rotate",
		`action="/ui/profile/developer-credential/revoke"`, `action="/ui/profile/developer-credential"`)
	mustNotContain(t, body, "Set Secret", "No credential set")
}

func TestProfileSetCredentialShowsTheSecretOnce(t *testing.T) {
	h := newProfileHarness()
	h.dev.ps = []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", []string{developerRole}, false, "")}
	// A form cannot name another user: the id comes from the session.
	rec := h.serve(t, profileDevSession(), profilePost("/ui/profile/developer-credential", url.Values{"id": {"prn_bob"}}))
	if rec.Code != http.StatusOK || len(h.creds.calls) != 1 || h.creds.calls[0] != "set:prn_ann" {
		t.Fatalf("status %d, calls %v", rec.Code, h.creds.calls)
	}
	mustContain(t, rec.Body.String(), testSecret, "Developer Client Secret", "Copy this secret now", "Developer client secret updated.", "client_id=prn_ann")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control %q", cc)
	}
	// Seen again, the secret is gone.
	mustNotContain(t, h.serve(t, profileDevSession(), req{target: "/ui/profile"}).Body.String(), testSecret)
}

func TestProfileRevokeCredential(t *testing.T) {
	h := newProfileHarness()
	h.dev.ps = []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", []string{developerRole}, true, "")}
	rec := h.serve(t, profileDevSession(), profilePost("/ui/profile/developer-credential/revoke", url.Values{"id": {"prn_bob"}}))
	mfaWantRedirect(t, rec, "/ui/profile")
	if len(h.creds.calls) != 1 || h.creds.calls[0] != "revoke:prn_ann" {
		t.Fatalf("calls %v", h.creds.calls)
	}
	if c := mfaCookie(rec, "fc_ui_flash"); c == nil || c.Value != flashDevRevoked {
		t.Errorf("flash cookie %v", c)
	}
}

func TestProfileDeveloperCredentialNeedsThePermission(t *testing.T) {
	h := newProfileHarness()
	h.dev.ps = []principal.Principal{devUser("prn_ann", "Ann Lee", "ann@x.test", nil, false, "")}
	for _, target := range []string{"/ui/profile/developer-credential", "/ui/profile/developer-credential/revoke"} {
		rec := h.serve(t, profileSession(), profilePost(target, nil))
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s: status %d", target, rec.Code)
		}
	}
	if len(h.creds.calls) != 0 {
		t.Fatalf("an operation ran: %v", h.creds.calls)
	}
}

func TestProfileChangePasswordChecksBeforeAskingThePlatform(t *testing.T) {
	h := newProfileHarness()
	for name, tc := range map[string]struct {
		form url.Values
		want string
	}{
		"policy":   {passwordForm("old-pass-word", "short", "short"), "at least"},
		"mismatch": {passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz?"), "New password and confirmation do not match."},
		"identity": {passwordForm("old-pass-word", "ann@x.test", "ann@x.test"), ""},
	} {
		rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", tc.form))
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", name, rec.Code)
			continue
		}
		body := rec.Body.String()
		// The dialog comes back open, with the message.
		mustContain(t, body, `data-init="el.removeAttribute(&#39;open&#39;); el.showModal()"`, `role="alert"`, tc.want)
		mustNotContain(t, body, "N3w-Passw0rd-xyz")
	}
	if h.up.called("POST /auth/change-password") {
		t.Fatalf("a refused form reached the platform: %v", h.up.calls)
	}
}

func TestProfileChangePasswordSuccess(t *testing.T) {
	h := newProfileHarness()
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")))
	mfaWantRedirect(t, rec, "/ui/profile")
	if c := mfaCookie(rec, "fc_ui_flash"); c == nil || c.Value != flashProfilePassword {
		t.Errorf("flash cookie %v", c)
	}
	got := h.up.bodies["/auth/change-password"]
	if got["currentPassword"] != "old-pass-word" || got["newPassword"] != "N3w-Passw0rd-xyz!" {
		t.Errorf("posted %v", got)
	}
	if _, has := got["code"]; has {
		t.Errorf("a code was sent without the second-factor step: %v", got)
	}
}

func TestProfileChangePasswordRefusals(t *testing.T) {
	h := newProfileHarness()
	h.up.answers["POST /auth/change-password"] = fakeAnswer{401, map[string]any{"code": "INVALID_CURRENT_PASSWORD", "message": "Your current password is incorrect."}}
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", passwordForm("wrong-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "Your current password is incorrect.", "showModal()")
	mustNotContain(t, rec.Body.String(), "wrong-pass-word", "N3w-Passw0rd-xyz")
}

func TestProfileChangePasswordSecondFactorStep(t *testing.T) {
	h := newProfileHarness()
	h.up.answers["POST /auth/change-password"] = fakeAnswer{400, map[string]any{
		"code": "MFA_REQUIRED", "message": "Enter a code from your second factor to change your password.", "methods": []string{"TOTP", "EMAIL_PIN", "SMS"},
	}}
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")))
	body := rec.Body.String()
	mustContain(t, body, "Enter a code from your second factor", "Two-factor code", `name="mfa" value="1"`, `name="methods" value="TOTP,EMAIL_PIN"`,
		"Verify &amp; Change", "Send email code", "Enter the code from your authenticator app, or send an email code below.",
		`name="newPassword" value="N3w-Passw0rd-xyz!"`)
	// A TOTP factor means no email is sent unasked.
	if h.up.called("POST /auth/change-password/send-email-code") {
		t.Error("an email code was sent although the user has an authenticator app")
	}

	// The code is required once asked for, then goes with the change.
	form := passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")
	form.Set("mfa", "1")
	form.Set("methods", "TOTP")
	rec = h.serve(t, profileSession(), profilePost("/ui/profile/password", form))
	mustContain(t, rec.Body.String(), "Enter your two-factor code.")
	form.Set("code", " 123456 ")
	h.up.answers["POST /auth/change-password"] = fakeAnswer{200, map[string]any{"message": "ok"}}
	rec = h.serve(t, profileSession(), profilePost("/ui/profile/password", form))
	mfaWantRedirect(t, rec, "/ui/profile")
	if h.up.bodies["/auth/change-password"]["code"] != "123456" {
		t.Errorf("posted %v", h.up.bodies["/auth/change-password"])
	}
}

func TestProfileEmailOnlyFactorGetsItsCodeAtOnce(t *testing.T) {
	h := newProfileHarness()
	h.up.answers["POST /auth/change-password"] = fakeAnswer{400, map[string]any{"code": "MFA_REQUIRED", "message": "Enter a code.", "methods": []string{"EMAIL_PIN"}}}
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password", passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")))
	if !h.up.called("POST /auth/change-password/send-email-code") {
		t.Fatalf("no email code sent: %v", h.up.calls)
	}
	mustContain(t, rec.Body.String(), "A code has been sent to your email.", "We sent a code to your email.", "Send email code")
}

func TestProfileSendEmailCode(t *testing.T) {
	h := newProfileHarness()
	form := passwordForm("old-pass-word", "N3w-Passw0rd-xyz!", "N3w-Passw0rd-xyz!")
	form.Set("mfa", "1")
	form.Set("methods", "TOTP,EMAIL_PIN")
	rec := h.serve(t, profileSession(), profilePost("/ui/profile/password/email-code", form))
	if rec.Code != http.StatusOK || !h.up.called("POST /auth/change-password/send-email-code") {
		t.Fatalf("status %d, calls %v", rec.Code, h.up.calls)
	}
	mustContain(t, rec.Body.String(), "A code has been sent to your email.", "Two-factor code", "showModal()")
	if h.up.called("POST /auth/change-password") {
		t.Error("sending a code changed the password")
	}

	h.up.answers["POST /auth/change-password/send-email-code"] = fakeAnswer{400, map[string]any{"code": "NO_EMAIL_2FA", "message": "email codes are not enabled for your account"}}
	rec = h.serve(t, profileSession(), profilePost("/ui/profile/password/email-code", form))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("refused send: status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "Could not send a code.")
}

func TestProfileSignInActivity(t *testing.T) {
	h := newProfileHarness()
	at := time.Date(2026, 11, 1, 9, 30, 0, 0, time.UTC)
	h.up.answers["GET /auth/login-history"] = fakeAnswer{200, map[string]any{"attempts": []any{
		map[string]any{"outcome": "SUCCESS", "ipAddress": "10.1.2.3", "attemptedAt": at, "userAgent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605 Chrome/120.0 Safari/537"},
		map[string]any{"outcome": "FAILURE", "attemptedAt": at},
	}}}

	// Closed by default: nothing is read.
	body := h.serve(t, profileSession(), req{target: "/ui/profile"}).Body.String()
	mustNotContain(t, body, "Recent sign-in", "10.1.2.3")
	if h.up.called("GET /auth/login-history") {
		t.Error("the history was read without being asked for")
	}

	body = h.serve(t, profileSession(), req{target: "/ui/profile?activity=1"}).Body.String()
	mustContain(t, body, "Sessions last up to 24 hours", "Success", "Failed", "Chrome on macOS", "Unknown device", "10.1.2.3", "Hide")

	h.up.answers["GET /auth/login-history"] = fakeAnswer{200, map[string]any{"attempts": []any{}}}
	mustContain(t, h.serve(t, profileSession(), req{target: "/ui/profile?activity=1"}).Body.String(), "No recent sign-in activity.")

	h.up.answers["GET /auth/login-history"] = fakeAnswer{500, map[string]any{"message": "boom"}}
	mustContain(t, h.serve(t, profileSession(), req{target: "/ui/profile?activity=1"}).Body.String(), "Could not load sign-in activity.")
}

func TestProfileFailsWhenTheSessionIsGone(t *testing.T) {
	h := newProfileHarness()
	h.up.answers["GET /auth/me"] = fakeAnswer{401, map[string]any{"message": "Not authenticated"}}
	rec := h.serve(t, profileSession(), req{target: "/ui/profile"})
	if rec.Code == http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestDeviceLabel(t *testing.T) {
	for ua, want := range map[string]string{
		"":                                    "Unknown device",
		"Mozilla/5.0 (Windows NT 10) Edg/120": "Edge on Windows",
		"Mozilla/5.0 (X11; Linux) Firefox/12": "Firefox on Linux",
		"Mozilla/5.0 (iPhone) Safari/604":     "Safari on iOS",
		"curl/8":                              "Browser",
	} {
		if got := deviceLabel(ua); got != want {
			t.Errorf("deviceLabel(%q) = %q, want %q", ua, got, want)
		}
	}
}

func TestProfileCredentialRefusalIsABanner(t *testing.T) {
	h := newProfileHarness()
	h.creds.err = usecase.BusinessRule("NOT_DEVELOPER", "Principal does not hold the developer role")
	rec := h.serve(t, profileDevSession(), profilePost("/ui/profile/developer-credential", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "Principal does not hold the developer role", "Developer API Access")
	mustNotContain(t, rec.Body.String(), testSecret)
}
