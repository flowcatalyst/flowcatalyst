package login

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// The /auth/2fa/* JSON endpoints over the shared flow with fakes of the
// services: the refusals the server-rendered pages share (their own tests in
// internal/ui/pages drive the same flow through the pages).

type flowFakeMFA struct {
	confirmed map[string][]mfa.MethodType
	sendErr   error
	calls     []string
}

func (f *flowFakeMFA) call(s string) { f.calls = append(f.calls, s) }
func (f *flowFakeMFA) called(prefix string) bool {
	return slices.ContainsFunc(f.calls, func(c string) bool { return strings.HasPrefix(c, prefix) })
}

func (f *flowFakeMFA) ConfirmedMethods(_ context.Context, pid string) ([]mfa.MethodType, error) {
	return slices.Clone(f.confirmed[pid]), nil
}

func (f *flowFakeMFA) VerifyTOTP(_ context.Context, _, code string) (bool, error) {
	f.call("verify-totp")
	return code == "123456", nil
}

func (f *flowFakeMFA) VerifyLoginEmailPin(_ context.Context, _, code string) (bool, error) {
	f.call("verify-email")
	return code == "654321", nil
}

func (f *flowFakeMFA) SendLoginEmailPin(context.Context, string, string) error {
	if f.sendErr != nil {
		return f.sendErr
	}
	f.call("send-pin")
	return nil
}

func (f *flowFakeMFA) VerifyRecoveryCode(context.Context, string, string) (bool, error) {
	f.call("verify-recovery")
	return true, nil
}

func (f *flowFakeMFA) BeginTOTPEnrollment(context.Context, string, string) (*mfa.TOTPEnrollment, error) {
	f.call("begin-totp")
	return &mfa.TOTPEnrollment{Secret: "S", URI: "otpauth://totp/x?secret=S"}, nil
}

func (f *flowFakeMFA) ConfirmTOTPEnrollment(_ context.Context, pid, code string) (bool, error) {
	f.call("confirm-totp")
	if code != "111111" {
		return false, nil
	}
	f.confirmed[pid] = append(f.confirmed[pid], mfa.MethodTOTP)
	return true, nil
}

func (f *flowFakeMFA) BeginEmailEnrollment(context.Context, string, string) error {
	f.call("begin-email")
	return nil
}

func (f *flowFakeMFA) ConfirmEmailEnrollment(context.Context, string, string) (bool, error) {
	f.call("confirm-email")
	return true, nil
}
func (f *flowFakeMFA) RemoveMethod(context.Context, string, mfa.MethodType) error { return nil }
func (f *flowFakeMFA) GenerateRecoveryCodes(context.Context, string) ([]string, error) {
	return []string{"A"}, nil
}
func (f *flowFakeMFA) RemainingRecoveryCodes(context.Context, string) (int, error) { return 0, nil }
func (f *flowFakeMFA) IssueTrustedDevice(context.Context, string, *string, time.Duration) (string, error) {
	return "T", nil
}

func (f *flowFakeMFA) VerifyTrustedDevice(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *flowFakeMFA) ListTrustedDevices(context.Context, string) ([]mfa.TrustedDevice, error) {
	return nil, nil
}
func (f *flowFakeMFA) RevokeTrustedDevice(context.Context, string, string) error { return nil }

type flowPolicy struct{ ev twofa.Eval }

func (p flowPolicy) Evaluate(context.Context, string) twofa.Eval { return p.ev }

type flowPrincipals map[string]*principal.Principal

func (f flowPrincipals) FindByID(_ context.Context, id ids.PrincipalID) (*principal.Principal, error) {
	return f[string(id)], nil
}

type flowSessions struct{ minted int }

func (s *flowSessions) MintSessionToken(context.Context, string, time.Duration) (string, error) {
	s.minted++
	return "JWT", nil
}

type flowRig struct {
	mfa      *flowFakeMFA
	sessions *flowSessions
	issuer   *mfatoken.Issuer
	router   chi.Router
}

func newFlowRig(t *testing.T, allowed ...string) *flowRig {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	rig := &flowRig{mfa: &flowFakeMFA{confirmed: map[string][]mfa.MethodType{}}, sessions: &flowSessions{}, issuer: mfatoken.NewIssuer(key, "iss")}
	e := &Endpoint{cfg: Config{MFATokens: rig.issuer}, tf: &TwoFactor{
		MFA: rig.mfa, Tokens: rig.issuer, Sessions: rig.sessions,
		Policy: flowPolicy{ev: twofa.Eval{Internal: true, Mapping: &emaildomainmapping.EmailDomainMapping{Require2FA: true, Allowed2FAMethods: allowed}}},
		Principals: flowPrincipals{"prn_ann": {
			ID: "prn_ann", Type: principal.TypeUser, Active: true, UserIdentity: principal.NewUserIdentity("ann@x.test"),
		}},
	}}
	rig.router = chi.NewRouter()
	e.RegisterTwoFactorRoutes(rig.router)
	return rig
}

func (rig *flowRig) token(t *testing.T, purpose mfatoken.Purpose) string {
	t.Helper()
	tok, err := rig.issuer.Mint("prn_ann", purpose, time.Minute)
	require.NoError(t, err)
	return tok
}

func (rig *flowRig) post(path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	rig.router.ServeHTTP(rec, req)
	return rec
}

func noSessionCookie(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "fc_session" && c.Value != "" {
			t.Fatalf("a session cookie was set: %v", c)
		}
	}
}

// An enroll token turns into a session only through a factor confirmed in
// that flow, while the account owes one: a factor set up anywhere else ends it.
func TestEnrollTokenIsDeadOnceAFactorExists(t *testing.T) {
	rig := newFlowRig(t, "TOTP", "EMAIL_PIN")
	enroll := rig.token(t, mfatoken.PurposeEnroll)
	rig.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin} // enrolled elsewhere
	for _, path := range []string{"/auth/2fa/enroll/totp/begin", "/auth/2fa/enroll/totp/confirm", "/auth/2fa/enroll/email/begin", "/auth/2fa/enroll/email/confirm"} {
		rec := rig.post(path, `{"enrollToken":"`+enroll+`","code":"111111"}`)
		assert.Equal(t, http.StatusConflict, rec.Code, path)
		assert.Contains(t, rec.Body.String(), "ENROLLMENT_NOT_REQUIRED", path)
		noSessionCookie(t, rec)
	}
	assert.Empty(t, rig.mfa.calls, "the enroll token still reached the service")
	assert.Zero(t, rig.sessions.minted)

	// A factor the domain does not allow does not count: enrolment is owed.
	rig = newFlowRig(t, "TOTP")
	enroll = rig.token(t, mfatoken.PurposeEnroll)
	rig.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	rec := rig.post("/auth/2fa/enroll/totp/begin", `{"enrollToken":"`+enroll+`"}`)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	// Email cannot be enrolled on a TOTP-only domain.
	rec = rig.post("/auth/2fa/enroll/email/begin", `{"enrollToken":"`+enroll+`"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	// A pending token is not an enroll token.
	rec = rig.post("/auth/2fa/enroll/totp/begin", `{"enrollToken":"`+rig.token(t, mfatoken.PurposePending)+`"}`)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// The domain's allowed set holds when the code is checked and when the email
// code is sent, and recovery codes need a usable authenticator app.
func TestVerifyEnforcesTheAllowedMethods(t *testing.T) {
	rig := newFlowRig(t, "TOTP")
	pending := rig.token(t, mfatoken.PurposePending)
	rig.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodTOTP, mfa.MethodEmailPin}

	rec := rig.post("/auth/2fa/verify", `{"mfaToken":"`+pending+`","method":"EMAIL_PIN","code":"654321"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "METHOD_NOT_ALLOWED")
	noSessionCookie(t, rec)
	rec = rig.post("/auth/2fa/challenge/email", `{"mfaToken":"`+pending+`"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, rig.mfa.called("verify-email") || rig.mfa.called("send-pin"), "%v", rig.mfa.calls)

	rec = rig.post("/auth/2fa/verify", `{"mfaToken":"`+pending+`","method":"SMS","code":"1"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "INVALID_METHOD")

	// Email-only on an email-only domain: no recovery codes.
	rig = newFlowRig(t, "EMAIL_PIN")
	pending = rig.token(t, mfatoken.PurposePending)
	rig.mfa.confirmed["prn_ann"] = []mfa.MethodType{mfa.MethodEmailPin}
	rec = rig.post("/auth/2fa/verify", `{"mfaToken":"`+pending+`","method":"RECOVERY_CODE","code":"AAAAA-BBBBB"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.False(t, rig.mfa.called("verify-recovery"))
	noSessionCookie(t, rec)

	// The resend interval answers 429 with Retry-After.
	rig.mfa.sendErr = &mfa.ResendError{Wait: 30 * time.Second}
	rec = rig.post("/auth/2fa/challenge/email", `{"mfaToken":"`+pending+`"}`)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Equal(t, "30", rec.Header().Get("Retry-After"))
	assert.Contains(t, rec.Body.String(), "CODE_RECENTLY_SENT")
}
