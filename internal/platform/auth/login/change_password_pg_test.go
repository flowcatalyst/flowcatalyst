//go:build integration

package login

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// sendEmailCodeState builds an Endpoint wired to the real (testpg) principal
// + MFA repositories, and a principal with the given confirmed methods
// pre-seeded.
func sendEmailCodeState(t *testing.T, email_ string, confirmed ...mfa.MethodType) (*Endpoint, string) {
	t.Helper()
	pool := testpg.Pool(t)
	principals := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)

	userEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principals),
		principalops.CreateCommand{Email: email_, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)

	mfaRepo := mfa.NewRepository(pool)
	for _, mt := range confirmed {
		m := mfa.NewMethod(userEv.UserID, mt)
		now := time.Now().UTC()
		m.ConfirmedAt = &now
		require.NoError(t, mfaRepo.InsertMethod(testpg.AnchorCtx(), m))
	}

	mfaSvc := mfa.NewService(mfaRepo, nil, email.LogService{}, mfa.Config{Issuer: "FlowCatalyst Test", EmailPinLength: 6})
	e := New(Config{
		Principals: principals,
		MFA:        mfaSvc,
	})
	return e, userEv.UserID
}

func doSendEmailCode(t *testing.T, e *Endpoint, principalID string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/auth/change-password/send-email-code", nil)
	req = req.WithContext(auth.WithContext(req.Context(), &auth.AuthContext{PrincipalID: principalID}))
	rec := httptest.NewRecorder()
	e.handleChangePasswordSendEmailCode(rec, req)

	var body map[string]any
	if rec.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	}
	return rec.Code, body
}

// TestHandleChangePasswordSendEmailCode_NoConfirmedFactor is the regression
// guard for the fix: a user with NO confirmed second factor at all must get
// NO_MFA ("two-factor is not enabled"), not NO_EMAIL_2FA — the latter wrongly
// implies email 2FA specifically is the missing piece, when in fact the user
// has no 2FA posture whatsoever.
func TestHandleChangePasswordSendEmailCode_NoConfirmedFactor(t *testing.T) {
	e, principalID := sendEmailCodeState(t, "no-mfa@change-password.test")

	status, body := doSendEmailCode(t, e, principalID)

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "NO_MFA", body["code"])
}

// TestHandleChangePasswordSendEmailCode_TOTPOnlyStaysNoEmail2FA pins the
// preserved behaviour: a user who HAS a confirmed factor, just not email,
// still gets the more specific NO_EMAIL_2FA — only the "no factor at all"
// case changed.
func TestHandleChangePasswordSendEmailCode_TOTPOnlyStaysNoEmail2FA(t *testing.T) {
	e, principalID := sendEmailCodeState(t, "totp-only@change-password.test", mfa.MethodTOTP)

	status, body := doSendEmailCode(t, e, principalID)

	require.Equal(t, http.StatusBadRequest, status)
	require.Equal(t, "NO_EMAIL_2FA", body["code"])
}
