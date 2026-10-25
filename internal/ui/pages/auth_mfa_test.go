package pages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	wa "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/assets"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
)

// fakeMFA is a mfa.Service with the state the pages read: the confirmed
// methods per user, the codes that verify, the pending enrollment.
type fakeMFA struct {
	confirmed map[string][]mfa.MethodType
	totp      string
	emailPin  string
	recovery  map[string]bool
	left      int
	devices   map[string][]mfa.TrustedDevice
	// pendingSecret is what an enrollment hands out; totpConfirm what confirms it.
	pendingURI  string
	totpConfirm string
	// sendErr is what sending a sign-in email code answers (nil: sent).
	sendErr error
	calls   []string
}

const (
	testTOTPURI    = "otpauth://totp/FlowCatalyst:ann@x.test?secret=JBSWY3DPEHPK3PXPSECRETKEY&issuer=FlowCatalyst"
	testTOTPSecret = "JBSWY3DPEHPK3PXPSECRETKEY"
)

func newFakeMFA() *fakeMFA {
	return &fakeMFA{
		confirmed: map[string][]mfa.MethodType{}, totp: "123456", emailPin: "654321",
		recovery: map[string]bool{"AAAAA-BBBBB": true}, pendingURI: testTOTPURI, totpConfirm: "111111",
		devices: map[string][]mfa.TrustedDevice{},
	}
}

func (f *fakeMFA) call(s string) { f.calls = append(f.calls, s) }

func (f *fakeMFA) called(prefix string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func (f *fakeMFA) ConfirmedMethods(_ context.Context, pid string) ([]mfa.MethodType, error) {
	return append([]mfa.MethodType(nil), f.confirmed[pid]...), nil
}

func (f *fakeMFA) has(pid string, t mfa.MethodType) bool { return slices.Contains(f.confirmed[pid], t) }

func (f *fakeMFA) VerifyTOTP(_ context.Context, pid, code string) (bool, error) {
	f.call("verify-totp:" + pid)
	return f.has(pid, mfa.MethodTOTP) && code == f.totp, nil
}

func (f *fakeMFA) VerifyLoginEmailPin(_ context.Context, pid, code string) (bool, error) {
	f.call("verify-email:" + pid)
	return f.has(pid, mfa.MethodEmailPin) && code == f.emailPin, nil
}

func (f *fakeMFA) SendLoginEmailPin(_ context.Context, pid, addr string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.call("send-pin:" + pid + ":" + addr)
	return nil
}

func (f *fakeMFA) VerifyRecoveryCode(_ context.Context, pid, code string) (bool, error) {
	f.call("verify-recovery:" + pid)
	if f.recovery[code] {
		delete(f.recovery, code)
		return true, nil
	}
	return false, nil
}

func (f *fakeMFA) BeginTOTPEnrollment(_ context.Context, pid, _ string) (*mfa.TOTPEnrollment, error) {
	f.call("begin-totp:" + pid)
	if f.has(pid, mfa.MethodTOTP) {
		return nil, mfa.ErrAlreadyEnrolled
	}
	return &mfa.TOTPEnrollment{Secret: testTOTPSecret, URI: f.pendingURI, QR: "data:image/png;base64,PNG"}, nil
}

func (f *fakeMFA) ConfirmTOTPEnrollment(_ context.Context, pid, code string) (bool, error) {
	f.call("confirm-totp:" + pid)
	if code != f.totpConfirm {
		return false, nil
	}
	f.confirmed[pid] = append(f.confirmed[pid], mfa.MethodTOTP)
	return true, nil
}

func (f *fakeMFA) BeginEmailEnrollment(_ context.Context, pid, addr string) error {
	f.call("begin-email:" + pid + ":" + addr)
	return nil
}

func (f *fakeMFA) ConfirmEmailEnrollment(_ context.Context, pid, code string) (bool, error) {
	f.call("confirm-email:" + pid)
	if code != f.emailPin {
		return false, nil
	}
	f.confirmed[pid] = append(f.confirmed[pid], mfa.MethodEmailPin)
	return true, nil
}

func (f *fakeMFA) RemoveMethod(_ context.Context, pid string, t mfa.MethodType) error {
	f.call("remove:" + pid + ":" + string(t))
	f.confirmed[pid] = slices.DeleteFunc(f.confirmed[pid], func(m mfa.MethodType) bool { return m == t })
	return nil
}

func (f *fakeMFA) GenerateRecoveryCodes(_ context.Context, pid string) ([]string, error) {
	f.call("generate:" + pid)
	f.left = 2
	return []string{"NEWCD-00001", "NEWCD-00002"}, nil
}

func (f *fakeMFA) RemainingRecoveryCodes(context.Context, string) (int, error) { return f.left, nil }

func (f *fakeMFA) IssueTrustedDevice(_ context.Context, pid string, _ *string, _ time.Duration) (string, error) {
	f.call("trust:" + pid)
	return "TRUSTED-RAW", nil
}

func (f *fakeMFA) VerifyTrustedDevice(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *fakeMFA) ListTrustedDevices(_ context.Context, pid string) ([]mfa.TrustedDevice, error) {
	return f.devices[pid], nil
}

func (f *fakeMFA) RevokeTrustedDevice(_ context.Context, pid, id string) error {
	f.call("revoke-device:" + pid + ":" + id)
	return nil
}

// fakeTokens accepts (and mints) "pending:<id>" and "enroll:<id>" tokens.
type fakeTokens struct{}

func (fakeTokens) Mint(subject string, purpose mfatoken.Purpose, _ time.Duration) (string, error) {
	return map[mfatoken.Purpose]string{mfatoken.PurposePending: "pending:", mfatoken.PurposeEnroll: "enroll:"}[purpose] + subject, nil
}

func (fakeTokens) Parse(token string, want mfatoken.Purpose) (*mfatoken.Claims, error) {
	prefix := map[mfatoken.Purpose]string{mfatoken.PurposePending: "pending:", mfatoken.PurposeEnroll: "enroll:"}[want]
	id, ok := strings.CutPrefix(token, prefix)
	if !ok || id == "" {
		return nil, errors.New("bad token")
	}
	c := &mfatoken.Claims{}
	c.Subject = id
	return c, nil
}

type fakePrincipals map[string]*principal.Principal

func (f fakePrincipals) FindByID(_ context.Context, id ids.PrincipalID) (*principal.Principal, error) {
	return f[string(id)], nil
}

type fakePolicy struct{ ev twofa.Eval }

func (f fakePolicy) Evaluate(context.Context, string) twofa.Eval { return f.ev }

type fakeSessions struct{ minted []string }

func (f *fakeSessions) MintSessionToken(_ context.Context, id string, _ time.Duration) (string, error) {
	f.minted = append(f.minted, id)
	return "SESSION-JWT-" + id, nil
}

type fakePasskeys struct {
	keys    map[string][]wa.Credential
	revoked []string
}

func (f *fakePasskeys) FindByPrincipal(_ context.Context, pid string) ([]wa.Credential, error) {
	return f.keys[pid], nil
}

func (f *fakePasskeys) Revoke(_ context.Context, id string) error {
	f.revoked = append(f.revoked, id)
	return nil
}

// testTrustedDeviceCookie is the trusted-device cookie over plain HTTP (the
// login flow's name; __Host-fc_td when Secure).
const testTrustedDeviceCookie = "fc_td"

func mfaUser(id, email string) *principal.Principal {
	return &principal.Principal{
		ID: ids.PrincipalID(id), Type: principal.TypeUser, Scope: principal.ScopeAnchor, Name: id, Active: true,
		UserIdentity: principal.NewUserIdentity(email),
	}
}

// required is a domain that compels 2FA with the listed methods.
func mfaRequired(remember bool, methods ...string) twofa.Eval {
	return twofa.Eval{Internal: true, Mapping: &emaildomainmapping.EmailDomainMapping{
		Require2FA: true, Allowed2FAMethods: methods, RememberDeviceEnabled: remember, RememberDeviceDays: 7,
	}}
}

type mfaHarness struct {
	mfa      *fakeMFA
	sessions *fakeSessions
	policy   fakePolicy
	users    fakePrincipals
	keys     *fakePasskeys
	page     *AuthMFA
}

func newMFAHarness() *mfaHarness {
	h := &mfaHarness{
		mfa: newFakeMFA(), sessions: &fakeSessions{}, policy: fakePolicy{ev: twofa.Eval{Internal: true}},
		users: fakePrincipals{"prn_ann": mfaUser("prn_ann", "ann@x.test"), "prn_bob": mfaUser("prn_bob", "bob@x.test")},
		keys:  &fakePasskeys{keys: map[string][]wa.Credential{}},
	}
	h.users["prn_off"] = mfaUser("prn_off", "off@x.test")
	h.users["prn_off"].Active = false
	return h
}

// build wires the page from the current fakes (call after changing them).
func (h *mfaHarness) build() *AuthMFA {
	h.page = &AuthMFA{
		Flow:       &login.TwoFactor{MFA: h.mfa, Tokens: fakeTokens{}, Policy: h.policy, Principals: h.users, Sessions: h.sessions},
		Principals: h.users, Passkeys: h.keys, PasskeyOps: h.keys, Shell: shellForTest(),
	}
	return h.page
}

// mfaMount mounts the pages as ui.Mount does: security headers on all, the
// session guard on the signed-in ones only.
type mfaMount struct{ *AuthMFA }

func (m mfaMount) Mount(r chi.Router) {
	r.Use(guard.SecurityHeaders)
	m.MountPublic(r)
	(&AuthPasskeys{}).Mount(r)
	r.Group(func(r chi.Router) {
		r.Use(guard.RequireSession)
		m.MountSession(r)
	})
}

func (h *mfaHarness) serve(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	if h.page == nil {
		h.build()
	}
	return serveMounted(t, ac, q, mfaMount{h.page})
}

func mfaPost(target string, form url.Values, cookie *http.Cookie) req {
	return req{method: http.MethodPost, target: target, form: form, cookie: cookie, headers: map[string]string{"Sec-Fetch-Site": "same-origin"}}
}

func pendingCk(id string) *http.Cookie {
	return &http.Cookie{Name: authChallengeCookie, Value: "pending:" + id}
}

func enrollCk(id string) *http.Cookie {
	return &http.Cookie{Name: authChallengeCookie, Value: "enroll:" + id}
}

func mfaSession(id string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: id, Scope: auth.ScopeAnchor, Roles: []string{"r"}, Email: id + "@x.test",
		PrincipalType: auth.PrincipalTypeUser,
	}
}

func mfaCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func mfaWantRedirect(t *testing.T, rec *httptest.ResponseRecorder, to string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != to {
		t.Fatalf("want 303 to %q, got %d to %q\n%s", to, rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
}

func mfaWantNoSession(t *testing.T, rec *httptest.ResponseRecorder, h *mfaHarness) {
	t.Helper()
	if c := mfaCookie(rec, "fc_session"); c != nil && c.MaxAge >= 0 && c.Value != "" {
		t.Fatalf("a session cookie was issued: %v", c)
	}
	if len(h.sessions.minted) != 0 {
		t.Fatalf("a session was minted: %v", h.sessions.minted)
	}
}

// ── next ───────────────────────────────────────────────────────────────

func TestAuthNext(t *testing.T) {
	for in, want := range map[string]string{
		"":                           "/ui/",
		"/ui/users":                  "/ui/users",
		"/ui/users?q=a%20b&page=2":   "/ui/users?q=a%20b&page=2",
		"/ui":                        "/ui",
		"https://evil.test/ui/":      "/ui/",
		"//evil.test/ui/":            "/ui/",
		"/\\evil.test":               "/ui/",
		"/dashboard":                 "/ui/",
		"/uix":                       "/ui/",
		"/ui/../auth/login":          "/ui/",
		"/ui/./users":                "/ui/",
		"/ui/%2e%2e/auth":            "/ui/",
		"/ui/users%2f..%2f":          "/ui/",
		"javascript:alert(1)":        "/ui/",
		"/ui/\r\nSet-Cookie: a=b":    "/ui/",
		"/ui/users\tx":               "/ui/",
		"/ui/a b":                    "/ui/",
		"http:/ui/users":             "/ui/",
		"/ui/users#frag":             "/ui/users",
		"/ui//users":                 "/ui//users",
		"/auth/login?redirect=/ui/x": "/ui/",
	} {
		if got := authNext(in); got != want {
			t.Errorf("authNext(%q) = %q, want %q", in, got, want)
		}
	}
}

// ── the challenge ──────────────────────────────────────────────────────

func TestChallengeNeedsAPendingToken(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	for name, c := range map[string]*http.Cookie{
		"no cookie":        nil,
		"enroll token":     enrollCk("prn_ann"),
		"garbage":          {Name: authChallengeCookie, Value: "nope"},
		"unknown user":     pendingCk("prn_zzz"),
		"inactive user":    pendingCk("prn_off"),
		"a session cookie": {Name: "fc_session", Value: "pending:prn_ann"},
	} {
		rec := h.serve(t, nil, req{target: "/ui/auth/2fa", cookie: c})
		mfaWantRedirect(t, rec, guard.LoginPath)
		rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"123456"}}, c))
		mfaWantRedirect(t, rec, guard.LoginPath)
		mfaWantNoSession(t, rec, h)
		if h.mfa.called("verify-totp:") {
			t.Errorf("%s: the code was checked", name)
		}
	}
}

func TestChallengePageOffersTheUsersMethods(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	rec := h.serve(t, nil, req{target: "/ui/auth/2fa?next=%2Fui%2Fusers", cookie: pendingCk("prn_ann")})
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, body, "Two-factor verification", "Enter the 6-digit code from your authenticator app.", `autocomplete="one-time-code"`,
		`name="next" value="/ui/users"`, `name="method" value="TOTP"`, "Use a recovery code", "method=RECOVERY_CODE")
	mustNotContain(t, body, "Use an email code", "Remember this device", assets.URL("fc-webauthn.js"))
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	h.policy.ev = mfaRequired(true, "TOTP", "EMAIL_PIN")
	h.build()
	body = h.serve(t, nil, req{target: "/ui/auth/2fa?method=EMAIL_PIN", cookie: pendingCk("prn_ann")}).Body.String()
	mustContain(t, body, "Email me a code", "Use authenticator app", "Use a recovery code")
	mustNotContain(t, body, "Remember this device") // no code field before the email is sent
	body = h.serve(t, nil, req{target: "/ui/auth/2fa", cookie: pendingCk("prn_ann")}).Body.String()
	mustContain(t, body, "Remember this device for 30 days", "Use an email code")

	// A domain that compels TOTP only: the email factor is not offered even
	// though the user holds it.
	h.policy.ev = mfaRequired(false, "TOTP")
	h.build()
	body = h.serve(t, nil, req{target: "/ui/auth/2fa?method=EMAIL_PIN", cookie: pendingCk("prn_ann")}).Body.String()
	mustNotContain(t, body, "Use an email code", "Email me a code")
}

func TestVerifyCompletesTheSignIn(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {" 123456 "}, "next": {"/ui/users?q=1"}}, pendingCk("prn_ann")))
	mfaWantRedirect(t, rec, "/ui/users?q=1")
	if c := mfaCookie(rec, "fc_session"); c == nil || c.Value != "SESSION-JWT-prn_ann" || !c.HttpOnly {
		t.Fatalf("session cookie = %v", c)
	}
	if c := mfaCookie(rec, authChallengeCookie); c == nil || c.MaxAge >= 0 {
		t.Fatalf("the challenge cookie was not ended: %v", c)
	}
	if mfaCookie(rec, testTrustedDeviceCookie) != nil {
		t.Error("a device was remembered without being asked")
	}

	// The next is checked again at the end.
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"123456"}, "next": {"https://evil.test/"}}, pendingCk("prn_ann")))
	mfaWantRedirect(t, rec, "/ui/")
}

func TestVerifyRefusesAWrongCode(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	for name, form := range map[string]url.Values{
		"wrong code":        {"method": {"TOTP"}, "code": {"000000"}},
		"empty code":        {"method": {"TOTP"}, "code": {""}},
		"a method not held": {"method": {"EMAIL_PIN"}, "code": {"654321"}},
		"unknown recovery":  {"method": {"RECOVERY_CODE"}, "code": {"ZZZZZ-ZZZZZ"}},
	} {
		rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", form, pendingCk("prn_ann")))
		if rec.Code < 400 || rec.Code >= 500 {
			t.Errorf("%s: status %d", name, rec.Code)
		}
		mfaWantNoSession(t, rec, h)
		mustContain(t, rec.Body.String(), "Two-factor verification")
	}
	mustContain(t, h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"000000"}}, pendingCk("prn_ann"))).Body.String(), "Invalid or expired code")
	// A method the page does not know is refused outright.
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"SMS"}, "code": {"1"}}, pendingCk("prn_ann")))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown method: status %d", rec.Code)
	}
	mfaWantNoSession(t, rec, h)
}

// The domain's allowed set is enforced when the code is checked, not only when
// the methods are offered: a factor the domain no longer allows is dead, and
// recovery codes need a usable authenticator app.
func TestVerifyEnforcesTheDomainsMethods(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	h.policy.ev = mfaRequired(false, "TOTP")
	h.build()
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"EMAIL_PIN"}, "code": {"654321"}}, pendingCk("prn_ann")))
	if rec.Code != http.StatusBadRequest || h.mfa.called("verify-email:") {
		t.Fatalf("an email code on a TOTP-only domain: status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mfaWantNoSession(t, rec, h)
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/email", nil, pendingCk("prn_ann")))
	if rec.Code != http.StatusUnprocessableEntity || h.mfa.called("send-pin:") {
		t.Fatalf("an email code sent on a TOTP-only domain: status %d, calls %v", rec.Code, h.mfa.calls)
	}

	// An email-only user (or one whose TOTP the domain no longer allows) has no
	// recovery codes to use, whatever is left in the store.
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	h.policy.ev = mfaRequired(false, "EMAIL_PIN")
	h.build()
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"RECOVERY_CODE"}, "code": {"AAAAA-BBBBB"}}, pendingCk("prn_ann")))
	if rec.Code != http.StatusBadRequest || h.mfa.called("verify-recovery:") {
		t.Fatalf("a recovery code without TOTP: status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mfaWantNoSession(t, rec, h)
	mustNotContain(t, h.serve(t, nil, req{target: "/ui/auth/2fa", cookie: pendingCk("prn_ann")}).Body.String(), "Use a recovery code")
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"RECOVERY_CODE"}, "code": {"AAAAA-BBBBB"}}, pendingCk("prn_ann")))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a recovery code with a TOTP factor the domain does not allow: status %d", rec.Code)
	}
}

// Asking for another email code within the resend interval sends none; the
// code already sent is still the one to enter.
func TestEmailChallengeCooldown(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	h.mfa.sendErr = &mfa.ResendError{Wait: 42 * time.Second}
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/email", nil, pendingCk("prn_ann")))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "42" {
		t.Fatalf("status %d, Retry-After %q", rec.Code, rec.Header().Get("Retry-After"))
	}
	mustContain(t, rec.Body.String(), "A code was sent recently. Wait 42 seconds", `name="code"`)
}

func TestVerifyIsPerUserAndRecoveryCodesBurn(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	// Bob holds no factor: Ann's TOTP code does not sign Bob in.
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"123456"}}, pendingCk("prn_bob")))
	mfaWantNoSession(t, rec, h)
	// A recovery code works once.
	form := url.Values{"method": {"RECOVERY_CODE"}, "code": {"AAAAA-BBBBB"}}
	mfaWantRedirect(t, h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", form, pendingCk("prn_ann"))), "/ui/")
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", form, pendingCk("prn_ann")))
	if rec.Code != http.StatusUnauthorized || len(h.sessions.minted) != 1 {
		t.Errorf("a burnt recovery code: status %d, sessions %v", rec.Code, h.sessions.minted)
	}
}

func TestVerifyRemembersTheDeviceOnlyWhenTheDomainAllows(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	form := url.Values{"method": {"TOTP"}, "code": {"123456"}, "remember": {"1"}}

	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", form, pendingCk("prn_ann")))
	mfaWantRedirect(t, rec, "/ui/")
	if mfaCookie(rec, testTrustedDeviceCookie) != nil || h.mfa.called("trust:") {
		t.Error("the domain does not allow remembering, a device was remembered")
	}

	h.policy.ev = mfaRequired(true, "TOTP")
	h.build()
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", form, pendingCk("prn_ann")))
	c := mfaCookie(rec, testTrustedDeviceCookie)
	if c == nil || c.Value != "TRUSTED-RAW" || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.MaxAge != 7*24*3600 {
		t.Fatalf("trusted device cookie = %+v", c)
	}
	// Not asked: not remembered.
	h.mfa.calls = nil
	h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"TOTP"}, "code": {"123456"}}, pendingCk("prn_ann")))
	if h.mfa.called("trust:") {
		t.Error("remembered without the box")
	}
}

func TestEmailChallenge(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/email", url.Values{"next": {"/ui/users"}}, pendingCk("prn_ann")))
	mustContain(t, rec.Body.String(), "Enter the code we emailed you.", `name="code"`)
	if !h.mfa.called("send-pin:prn_ann:ann@x.test") {
		t.Errorf("calls: %v", h.mfa.calls)
	}
	// Without a pending token, or for a user without the email factor, no code is sent.
	h.mfa.calls = nil
	mfaWantRedirect(t, h.serve(t, nil, mfaPost("/ui/auth/2fa/email", nil, nil)), guard.LoginPath)
	mfaWantRedirect(t, h.serve(t, nil, mfaPost("/ui/auth/2fa/email", nil, enrollCk("prn_ann"))), guard.LoginPath)
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/email", nil, pendingCk("prn_ann")))
	if rec.Code != http.StatusUnprocessableEntity || h.mfa.called("send-pin:") {
		t.Errorf("a user without the email factor: status %d, calls %v", rec.Code, h.mfa.calls)
	}
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/verify", url.Values{"method": {"EMAIL_PIN"}, "code": {"654321"}}, pendingCk("prn_ann")))
	mfaWantRedirect(t, rec, "/ui/")
}

func TestStartHandsOverTheToken(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/start", url.Values{"token": {"pending:prn_ann"}, "next": {"/ui/users"}}, nil))
	mfaWantRedirect(t, rec, "/ui/auth/2fa?next=%2Fui%2Fusers")
	c := mfaCookie(rec, authChallengeCookie)
	if c == nil || c.Value != "pending:prn_ann" || !c.HttpOnly || c.Path != "/ui/auth" || c.MaxAge != 600 {
		t.Fatalf("challenge cookie = %+v", c)
	}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/start", url.Values{"token": {"enroll:prn_bob"}}, nil))
	mfaWantRedirect(t, rec, "/ui/auth/2fa/enroll?next=%2Fui%2F")
	if c := mfaCookie(rec, authChallengeCookie); c == nil || c.MaxAge != 1800 {
		t.Fatalf("enroll cookie = %+v", c)
	}
	// An enroll token of a user who has a factor by now starts nothing: the
	// challenge is their way in.
	mfaWantRedirect(t, h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/start", url.Values{"token": {"enroll:prn_ann"}}, nil)), guard.LoginPath)
	// A token of the other purpose, a bad one, one of an inactive user or of a
	// user without a factor to challenge starts nothing.
	for _, tok := range []string{"enroll:prn_ann", "garbage", "", "pending:prn_off", "pending:prn_nobody", "pending:prn_bob"} {
		rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/start", url.Values{"token": {tok}}, nil))
		mfaWantRedirect(t, rec, guard.LoginPath)
		if c := mfaCookie(rec, authChallengeCookie); c != nil && c.MaxAge >= 0 {
			t.Errorf("token %q set the challenge cookie", tok)
		}
	}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/start", url.Values{"token": {"pending:prn_ann"}}, nil))
	mfaWantRedirect(t, rec, guard.LoginPath)
}

// ── enrollment after the password ──────────────────────────────────────

func TestEnrollOffersOnlyTheDomainsMethods(t *testing.T) {
	h := newMFAHarness()
	h.policy.ev = mfaRequired(false, "TOTP")
	body := h.serve(t, nil, req{target: "/ui/auth/2fa/enroll?next=%2Fui%2Fusers", cookie: enrollCk("prn_ann")}).Body.String()
	mustContain(t, body, "Use an authenticator app", `action="/ui/auth/2fa/enroll/totp"`, `name="next" value="/ui/users"`)
	mustNotContain(t, body, "Use email codes")

	// A pending token is not an enroll token.
	mfaWantRedirect(t, h.serve(t, nil, req{target: "/ui/auth/2fa/enroll", cookie: pendingCk("prn_ann")}), guard.LoginPath)
	mfaWantRedirect(t, h.serve(t, nil, req{target: "/ui/auth/2fa/enroll"}), guard.LoginPath)

	// A forged post for a method the domain does not allow starts nothing.
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/email", nil, enrollCk("prn_ann")))
	if rec.Code != http.StatusForbidden || h.mfa.called("begin-email:") {
		t.Errorf("email enrollment on a TOTP-only domain: status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mustContain(t, rec.Body.String(), "Email codes are not permitted for this domain.")
	h.policy.ev = mfaRequired(false, "EMAIL_PIN")
	h.build()
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp", nil, enrollCk("prn_ann")))
	if rec.Code != http.StatusForbidden || h.mfa.called("begin-totp:") {
		t.Errorf("TOTP enrollment on an email-only domain: status %d, calls %v", rec.Code, h.mfa.calls)
	}
}

var svgTag = regexp.MustCompile(`<svg [^>]*viewBox="0 0 (\d+) \d+"`)

func TestEnrollTOTPShowsTheKeyAndAQRCodeOnce(t *testing.T) {
	h := newMFAHarness()
	h.policy.ev = mfaRequired(false, "TOTP", "EMAIL_PIN")
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp", url.Values{"next": {"/ui/users"}}, enrollCk("prn_ann")))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, body, testTOTPSecret, `href="`+strings.ReplaceAll(testTOTPURI, "&", "&amp;")+`"`, `name="uri"`, `action="/ui/auth/2fa/enroll/totp/confirm"`, `aria-label="Authenticator QR code"`)
	mustNotContain(t, body, "data:image/png")
	if m := svgTag.FindStringSubmatch(body); m == nil {
		t.Error("no QR svg")
	} else if n, _ := strconv.Atoi(m[1]); n < 29 {
		t.Errorf("QR svg of %d modules (version 1 with the quiet zone is 29)", n)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	if strings.Contains(rec.Header().Get("Set-Cookie"), testTOTPSecret) {
		t.Error("the key is in a cookie")
	}
	// The key is on no other page.
	mustNotContain(t, h.serve(t, nil, req{target: "/ui/auth/2fa/enroll", cookie: enrollCk("prn_ann")}).Body.String(), testTOTPSecret)
}

// The enroll token alone is no session: only a factor confirmed in this flow
// ends in one, and only while the account still owes a factor.
func TestEnrollmentIsTheOnlyWayPastTheEnrollToken(t *testing.T) {
	h := newMFAHarness()
	h.policy.ev = mfaRequired(false, "TOTP", "EMAIL_PIN")

	// There is no "done" step to post past the enrolment.
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/done", url.Values{"next": {"/ui/users"}}, enrollCk("prn_ann")))
	if rec.Code < 400 {
		t.Errorf("enroll/done: status %d", rec.Code)
	}
	mfaWantNoSession(t, rec, h)

	// A wrong code: the form again, the QR code still there to retry with.
	h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp", nil, enrollCk("prn_ann")))
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"999999"}, "uri": {testTOTPURI}}, enrollCk("prn_ann")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("wrong code: status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "That code didn&#39;t match", "<svg ", testTOTPSecret)
	mfaWantNoSession(t, rec, h)
	// A forged uri is only drawn, never trusted: not an authenticator URI, no setup.
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"999999"}, "uri": {"javascript:alert(1)"}}, enrollCk("prn_ann")))
	mustNotContain(t, rec.Body.String(), "javascript:", `aria-label="Authenticator QR code"`)
	mfaWantNoSession(t, rec, h)

	// A factor that was not enrolled in this flow: the user (or an admin
	// widening the allowed methods) gave the account one elsewhere. The enroll
	// token now buys nothing: no menu, no setup, no confirm, no session, and
	// the factor is not enrolled over.
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	h.mfa.calls = nil
	for _, q := range []req{
		{target: "/ui/auth/2fa/enroll", cookie: enrollCk("prn_ann")},
		mfaPost("/ui/auth/2fa/enroll/totp", nil, enrollCk("prn_ann")),
		mfaPost("/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"111111"}, "uri": {testTOTPURI}}, enrollCk("prn_ann")),
		mfaPost("/ui/auth/2fa/enroll/email", nil, enrollCk("prn_ann")),
		mfaPost("/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {"654321"}}, enrollCk("prn_ann")),
		mfaPost("/ui/auth/2fa/enroll/done", nil, enrollCk("prn_ann")),
	} {
		rec = h.serve(t, nil, q)
		mfaWantNoSession(t, rec, h)
		if rec.Code == http.StatusOK {
			t.Errorf("%s %s with a factor already set up: status %d", q.method, q.target, rec.Code)
		}
	}
	if h.mfa.called("begin-") || h.mfa.called("confirm-") {
		t.Errorf("the enroll token still enrolled: %v", h.mfa.calls)
	}
	// A factor the domain does not allow does not count: the enrolment is still owed.
	h.policy.ev = mfaRequired(false, "TOTP")
	h.build()

	// The right code: the session, and the recovery codes, once.
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"111111"}, "uri": {testTOTPURI}, "next": {"/ui/users"}}, enrollCk("prn_ann")))
	if rec.Code != http.StatusOK {
		t.Fatalf("right code: status %d\n%s", rec.Code, rec.Body.String())
	}
	mustContain(t, rec.Body.String(), "Recovery codes", "NEWCD-00001", "NEWCD-00002", `href="/ui/users"`)
	mustNotContain(t, rec.Body.String(), testTOTPSecret)
	if c := mfaCookie(rec, "fc_session"); c == nil || c.Value != "SESSION-JWT-prn_ann" {
		t.Fatalf("session cookie = %v", c)
	}
	if c := mfaCookie(rec, authChallengeCookie); c == nil || c.MaxAge >= 0 {
		t.Fatalf("the challenge cookie was not ended: %v", c)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	// The enrolment is over: the same token enrols nothing more.
	h.sessions.minted = nil
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/totp/confirm", url.Values{"code": {"111111"}}, enrollCk("prn_ann")))
	mfaWantRedirect(t, rec, guard.LoginPath)
	mfaWantNoSession(t, rec, h)
}

func TestEnrollEmailCompletesAtOnce(t *testing.T) {
	h := newMFAHarness()
	h.policy.ev = mfaRequired(false, "EMAIL_PIN")
	rec := h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/email", url.Values{"next": {"/ui/users"}}, enrollCk("prn_ann")))
	mustContain(t, rec.Body.String(), "We've emailed you a code.", `action="/ui/auth/2fa/enroll/email/confirm"`)
	if !h.mfa.called("begin-email:prn_ann:ann@x.test") {
		t.Errorf("calls: %v", h.mfa.calls)
	}
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {"000000"}}, enrollCk("prn_ann")))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("wrong code: status %d", rec.Code)
	}
	mfaWantNoSession(t, rec, h)
	// Email is its own recovery channel: no recovery codes, the sign-in completes.
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {"654321"}, "next": {"/ui/users"}}, enrollCk("prn_ann")))
	mfaWantRedirect(t, rec, "/ui/users")
	if h.mfa.called("generate:") {
		t.Error("recovery codes were generated for an email-only user")
	}
	// A pending token confirms nothing.
	rec = h.serve(t, nil, mfaPost("/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {"654321"}}, pendingCk("prn_bob")))
	mfaWantRedirect(t, rec, guard.LoginPath)
	if h.mfa.called("confirm-email:prn_bob") {
		t.Error("a pending token confirmed an enrollment")
	}
}

// ── the security page ──────────────────────────────────────────────────

func TestSecurityPageNeedsASession(t *testing.T) {
	h := newMFAHarness()
	rec := h.serve(t, nil, req{target: "/ui/auth/security"})
	if rec.Code != http.StatusFound || !strings.HasPrefix(rec.Header().Get("Location"), guard.LoginPath) {
		t.Fatalf("status %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	for _, target := range []string{
		"/ui/auth/security/2fa/totp", "/ui/auth/security/2fa/totp/confirm", "/ui/auth/security/2fa/email", "/ui/auth/security/2fa/email/confirm",
		"/ui/auth/security/2fa/TOTP/remove", "/ui/auth/security/2fa/recovery-codes", "/ui/auth/security/2fa/devices/x/revoke", "/ui/auth/security/passkeys/x/remove",
	} {
		rec := h.serve(t, nil, mfaPost(target, url.Values{"code": {"111111"}}, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without a session: status %d", target, rec.Code)
		}
	}
	if len(h.mfa.calls) != 0 || len(h.keys.revoked) != 0 {
		t.Errorf("an unauthenticated post acted: %v %v", h.mfa.calls, h.keys.revoked)
	}
	// A challenge cookie is not a session either.
	rec = h.serve(t, nil, req{target: "/ui/auth/security", cookie: pendingCk("prn_ann")})
	if rec.Code != http.StatusFound {
		t.Errorf("challenge cookie on the security page: status %d", rec.Code)
	}
}

func TestSecurityPageShowsTheUsersOwnState(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	h.mfa.confirmed["prn_bob"] = []mfa.MethodType{mfa.MethodEmailPin}
	h.mfa.left = 7
	label := "Safari on a Mac"
	h.mfa.devices["prn_ann"] = []mfa.TrustedDevice{{ID: "dev_ann", Label: &label, ExpiresAt: time.Now().Add(time.Hour)}}
	h.mfa.devices["prn_bob"] = []mfa.TrustedDevice{{ID: "dev_bob", ExpiresAt: time.Now().Add(time.Hour)}}
	name := "Work laptop"
	h.keys.keys["prn_ann"] = []wa.Credential{{ID: "pk_ann", PrincipalID: "prn_ann", Name: &name, CreatedAt: time.Now()}}
	h.keys.keys["prn_bob"] = []wa.Credential{{ID: "pk_bob", PrincipalID: "prn_bob", CreatedAt: time.Now()}}
	h.policy.ev = mfaRequired(true, "TOTP", "EMAIL_PIN")

	rec := h.serve(t, mfaSession("prn_ann"), req{target: "/ui/auth/security"})
	body := rec.Body.String()
	mustContain(t, body, "<title>Security · FlowCatalyst</title>", "Authenticator app", "Your organisation requires 2FA.", "Add Email code",
		"7 unused codes remaining.", "Regenerate recovery codes", "Safari on a Mac", "Work laptop", "Add a passkey", assets.URL("fc-webauthn.js"),
		"data-fc-passkey-register", `/ui/auth/security/passkeys/pk_ann/remove`, `/ui/auth/security/2fa/devices/dev_ann/revoke`)
	mustNotContain(t, body, "Add Authenticator app", "dev_bob", "pk_bob", "Unnamed passkey")

	body = h.serve(t, mfaSession("prn_bob"), req{target: "/ui/auth/security?added=1"}).Body.String()
	mustContain(t, body, "Email code", "Unnamed passkey", "dev_bob", "Unknown device")
	mustNotContain(t, body, "Recovery codes", "Regenerate recovery codes", "pk_ann", "dev_ann")
}

func TestSelfRemoveKeepsTheLastRequiredFactor(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	h.policy.ev = mfaRequired(false, "TOTP", "EMAIL_PIN")
	rec := h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/TOTP/remove", nil, nil))
	if rec.Code != http.StatusConflict || h.mfa.called("remove:") {
		t.Fatalf("last required factor: status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mustContain(t, rec.Body.String(), "Your organisation requires 2FA — add another method before removing this one.")

	// With a second factor the first can go; the removal acts on the session's user.
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/TOTP/remove", nil, nil))
	if rec.Code != http.StatusOK || !h.mfa.called("remove:prn_ann:TOTP") {
		t.Fatalf("status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mustContain(t, rec.Body.String(), "Authenticator app removed.")

	// A domain that does not compel 2FA lets the last one go.
	h.policy.ev = twofa.Eval{Internal: true}
	h.build()
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/EMAIL_PIN/remove", nil, nil))
	if rec.Code != http.StatusOK || !h.mfa.called("remove:prn_ann:EMAIL_PIN") {
		t.Fatalf("status %d, calls %v", rec.Code, h.mfa.calls)
	}
	// Nothing else is a method; another user's id in the path is no such thing.
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/SMS/remove", nil, nil))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown method: status %d", rec.Code)
	}
}

func TestSelfEnrollRespectsTheDomain(t *testing.T) {
	h := newMFAHarness()
	h.policy.ev = mfaRequired(false, "EMAIL_PIN")
	rec := h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/totp", nil, nil))
	if rec.Code != http.StatusForbidden || h.mfa.called("begin-totp:") {
		t.Errorf("status %d, calls %v", rec.Code, h.mfa.calls)
	}
	mustContain(t, rec.Body.String(), "The authenticator app is not permitted for this domain.")
	h.policy.ev = mfaRequired(false, "TOTP")
	h.build()
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/email", nil, nil))
	if rec.Code != http.StatusForbidden || h.mfa.called("begin-email:") {
		t.Errorf("status %d, calls %v", rec.Code, h.mfa.calls)
	}
	// Already enrolled: the service's refusal is shown.
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/totp", nil, nil))
	if rec.Code != http.StatusConflict {
		t.Errorf("already enrolled: status %d", rec.Code)
	}
}

// The key and the recovery codes are in the response that creates them and
// nowhere else; the confirm step shows the new codes once.
func TestSecretsAreShownOnce(t *testing.T) {
	h := newMFAHarness()
	ann := mfaSession("prn_ann")
	rec := h.serve(t, ann, mfaPost("/ui/auth/security/2fa/totp", nil, nil))
	mustContain(t, rec.Body.String(), testTOTPSecret, "<svg ")
	mustNotContain(t, rec.Body.String(), "data:image/png")
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	mustNotContain(t, h.serve(t, ann, req{target: "/ui/auth/security"}).Body.String(), testTOTPSecret)

	rec = h.serve(t, ann, mfaPost("/ui/auth/security/2fa/totp/confirm", url.Values{"code": {"000000"}, "uri": {testTOTPURI}}, nil))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("wrong code: status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "That code didn&#39;t match")
	rec = h.serve(t, ann, mfaPost("/ui/auth/security/2fa/totp/confirm", url.Values{"code": {"111111"}, "uri": {testTOTPURI}}, nil))
	mustContain(t, rec.Body.String(), "Two-factor method added.", "NEWCD-00001", "NEWCD-00002", "I've saved them")
	mustNotContain(t, rec.Body.String(), testTOTPSecret)
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// Gone from the next view.
	mustNotContain(t, h.serve(t, ann, req{target: "/ui/auth/security"}).Body.String(), "NEWCD-", testTOTPSecret)

	// Regenerating shows the new set once; it needs an authenticator factor.
	h.mfa.calls = nil
	rec = h.serve(t, ann, mfaPost("/ui/auth/security/2fa/recovery-codes", nil, nil))
	mustContain(t, rec.Body.String(), "New recovery codes generated. Save them now.", "NEWCD-00001")
	mustNotContain(t, h.serve(t, ann, req{target: "/ui/auth/security"}).Body.String(), "NEWCD-")
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	h.mfa.calls = nil
	rec = h.serve(t, ann, mfaPost("/ui/auth/security/2fa/recovery-codes", nil, nil))
	if rec.Code != http.StatusBadRequest || h.mfa.called("generate:") {
		t.Errorf("regenerate without TOTP: status %d, calls %v", rec.Code, h.mfa.calls)
	}
}

func TestSelfServiceActsOnlyOnTheSessionsUser(t *testing.T) {
	h := newMFAHarness()
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	h.mfa.confirmed["prn_bob"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}
	name := "Bob's key"
	h.keys.keys["prn_bob"] = []wa.Credential{{ID: "pk_bob", PrincipalID: "prn_bob", Name: &name}}
	h.keys.keys["prn_ann"] = []wa.Credential{{ID: "pk_ann", PrincipalID: "prn_ann"}}

	// A form that names another user changes nothing about who is acted on.
	h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/TOTP/remove", url.Values{"principalId": {"prn_bob"}, "id": {"prn_bob"}}, nil))
	if h.mfa.called("remove:prn_bob") || !h.mfa.called("remove:prn_ann:TOTP") {
		t.Errorf("calls: %v", h.mfa.calls)
	}
	h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/2fa/devices/dev_bob/revoke", url.Values{"principalId": {"prn_bob"}}, nil))
	if !h.mfa.called("revoke-device:prn_ann:dev_bob") || h.mfa.called("revoke-device:prn_bob") {
		t.Errorf("calls: %v", h.mfa.calls)
	}

	// Another user's passkey is not found and is not revoked; one's own is.
	rec := h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/passkeys/pk_bob/remove", nil, nil))
	if rec.Code != http.StatusNotFound || len(h.keys.revoked) != 0 {
		t.Fatalf("another user's passkey: status %d, revoked %v", rec.Code, h.keys.revoked)
	}
	mustNotContain(t, rec.Body.String(), "Bob")
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/passkeys/pk_ann/remove", nil, nil))
	if rec.Code != http.StatusOK || !slices.Equal(h.keys.revoked, []string{"pk_ann"}) {
		t.Fatalf("own passkey: status %d, revoked %v", rec.Code, h.keys.revoked)
	}
	mustContain(t, rec.Body.String(), "Passkey removed.")
	rec = h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/passkeys/nope/remove", nil, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown passkey: status %d", rec.Code)
	}
}

func TestSecurityPageWithoutPasskeys(t *testing.T) {
	h := newMFAHarness()
	h.build().Passkeys, h.page.PasskeyOps = nil, nil
	body := h.serve(t, mfaSession("prn_ann"), req{target: "/ui/auth/security"}).Body.String()
	mustContain(t, body, "Two-Factor Authentication")
	mustNotContain(t, body, "Passkeys", assets.URL("fc-webauthn.js"))
	rec := h.serve(t, mfaSession("prn_ann"), mfaPost("/ui/auth/security/passkeys/x/remove", nil, nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d", rec.Code)
	}
}

// ── passkey sign-in ────────────────────────────────────────────────────

func TestPasskeySignInPage(t *testing.T) {
	h := newMFAHarness()
	rec := h.serve(t, nil, req{target: "/ui/auth/passkey?next=%2Fui%2Fusers&email=ann%40x.test"})
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, body, "Sign in with a passkey", "data-fc-passkey-signin", `data-next="/ui/users"`, `value="ann@x.test"`,
		`<script defer src="`+assets.URL("fc-webauthn.js")+`">`, "data-fc-passkey-error")
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}
	// The next is checked; the field is escaped.
	body = h.serve(t, nil, req{target: "/ui/auth/passkey?next=https%3A%2F%2Fevil.test%2F&email=%22%3E%3Cscript%3E"}).Body.String()
	mustContain(t, body, `data-next="/ui/"`)
	mustNotContain(t, body, "evil.test", `"><script>`)
	// Only the pages that use it load the bundle.
	h.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP}
	mustNotContain(t, h.serve(t, nil, req{target: "/ui/auth/2fa", cookie: pendingCk("prn_ann")}).Body.String(), "fc-webauthn")
}

func TestPasskeySignInPageByPost(t *testing.T) {
	h := newMFAHarness()
	rec := h.serve(t, nil, mfaPost("/ui/auth/passkey", url.Values{"next": {"/ui/users"}, "email": {"ann@x.test"}}, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	mustContain(t, rec.Body.String(), "data-fc-passkey-signin", `data-next="/ui/users"`, `value="ann@x.test"`)
}
