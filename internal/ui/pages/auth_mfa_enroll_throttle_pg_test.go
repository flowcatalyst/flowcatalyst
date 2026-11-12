//go:build integration

package pages

import (
	"net/http"
	"net/url"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/email"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// L3: enrolling an email factor through an enroll token cannot be used to
// brute-force the PIN. Asking for the code again inside the resend interval
// sends nothing (so the attempt count is not reset); wrong codes feed the
// sign-in backoff and, while it holds, even the right code is refused. A user
// who types the right code within limits still enrols.
func TestEmailEnrolmentIsThrottledAgainstPostgres(t *testing.T) {
	r := newPGRig(t, "pgl3")
	mailer := &pgMailer{}
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	r.page.Flow.MFA = mfa.NewService(mfa.NewRepository(testpg.Pool(t)), enc, mailer, mfa.DefaultConfig())
	pinRe := regexp.MustCompile(`\d{6}`)

	// Ann: begin sends a code; a second begin inside the interval does not.
	annTok := r.token(r.ann, mfatoken.PurposeEnroll)
	rec := r.do(http.MethodPost, "/ui/auth/2fa/enroll/email", url.Values{}, "", annTok)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	annPin := pinRe.FindString(mailer.last.HTMLBody)
	require.NotEmpty(t, annPin)
	mailer.last = email.Message{}
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/email", url.Values{}, "", annTok)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, rec.Body.String())
	assert.Empty(t, mailer.last.To, "a second enrolment code was emailed within the resend interval")

	// Wrong codes trip the backoff (rig: one free failure, then a 60s delay).
	confirm := func(code string) *http.Response {
		rec := r.do(http.MethodPost, "/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {code}}, "", annTok)
		return rec.Result()
	}
	assert.Equal(t, http.StatusUnprocessableEntity, confirm("000000").StatusCode)
	assert.Equal(t, http.StatusUnprocessableEntity, confirm("000001").StatusCode)
	assert.Equal(t, http.StatusTooManyRequests, confirm("000002").StatusCode, "wrong enrolment codes did not trip the backoff")

	// While it holds, the right code is refused and no session is issued.
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {annPin}}, "", annTok)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code)
	assert.Nil(t, mfaCookie(rec, "fc_session"), "a session was issued while the backoff held")

	// Bob (another account, same address): begin, one typo, then the right code enrols.
	bobTok := r.token(r.bob, mfatoken.PurposeEnroll)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/email", url.Values{}, "", bobTok)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	bobPin := pinRe.FindString(mailer.last.HTMLBody)
	require.NotEmpty(t, bobPin)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {"000000"}}, "", bobTok)
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	rec = r.do(http.MethodPost, "/ui/auth/2fa/enroll/email/confirm", url.Values{"code": {bobPin}}, "", bobTok)
	mfaWantRedirect(t, rec, "/ui/")
	assert.NotNil(t, mfaCookie(rec, "fc_session"))
}
