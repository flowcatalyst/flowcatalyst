//go:build integration

package oauthapi

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// stateForLoginAttemptIPUA builds a *State wired to the real (testpg)
// principal and login-attempt repositories plus a self-contained encryption
// service, so client_credentials runs its real secret-verification path and
// recordAttempt writes a real row this test can read back.
func stateForLoginAttemptIPUA(t *testing.T) *State {
	t.Helper()
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := encryption.New(key)
	require.NoError(t, err)
	return &State{
		Principals:    principal.NewRepository(testpg.Pool(t)),
		Auth:          testAuthService(t),
		Encryption:    enc,
		LoginAttempts: loginattempt.NewRepository(testpg.Pool(t)),
	}
}

// newActivePrincipal creates an active USER principal (client_credentials
// doesn't restrict principal type) with a unique email, returning its id.
func newActivePrincipal(t *testing.T, email string) string {
	t.Helper()
	uow := testpg.NewUoW(t)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, principalops.CreateUser(principal.NewRepository(testpg.Pool(t))),
		principalops.CreateCommand{Email: email, Scope: "ANCHOR"}, testpg.TestEC())
	require.NoError(t, err)
	return ev.UserID
}

// confidentialClient builds an active confidential OAuth client with the
// given secret, linked to principalID.
func confidentialClient(t *testing.T, s *State, clientID, principalID, secret string) *auth.OAuthClient {
	t.Helper()
	c := &auth.OAuthClient{
		ClientID:    clientID,
		Active:      true,
		ClientType:  auth.OAuthClientConfidential,
		PrincipalID: &principalID,
	}
	c.SetSecretRef(encrypted(t, s, secret))
	return c
}

// doTokenRequestWithHeaders is doTokenRequest plus caller-supplied headers,
// needed here to drive X-Forwarded-For / User-Agent.
func doTokenRequestWithHeaders(t *testing.T, s *State, form url.Values, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/oauth/token", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	s.Token(rr, req)
	return rr
}

// TestTokenRecordsIPAndUserAgent_Success (T1): a successful client_credentials
// grant stores the login-attempt row with the rightmost X-Forwarded-For hop
// and the request's User-Agent. Mutant: a call site that passes nil for
// either field would leave the stored columns empty, so asserting the
// non-empty, exact values (not merely that a row exists) pins this.
func TestTokenRecordsIPAndUserAgent_Success(t *testing.T) {
	s := stateForLoginAttemptIPUA(t)
	principalID := newActivePrincipal(t, "iprua-success@token.test")
	clientID := "oac_iprua_success"
	c := confidentialClient(t, s, clientID, principalID, "s3cr3t")
	s.OAuthClients = fakeClientFinder{client: c}

	rr := doTokenRequestWithHeaders(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {"s3cr3t"},
	}, map[string]string{
		"X-Forwarded-For": "203.0.113.9, 198.51.100.7",
		"User-Agent":      "fc-test/1.0",
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	attempts, err := s.LoginAttempts.FindRecentByIdentifier(t.Context(), clientID, 1)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	a := attempts[0]
	require.Equal(t, loginattempt.OutcomeSuccess, a.Outcome)
	require.NotNil(t, a.IPAddress)
	require.Equal(t, "198.51.100.7", *a.IPAddress, "must be the rightmost X-Forwarded-For hop")
	require.NotNil(t, a.UserAgent)
	require.Equal(t, "fc-test/1.0", *a.UserAgent)
}

// TestTokenRecordsIPAndUserAgent_Failure (T2): an invalid client secret still
// records IP/UA on the failure row. Mutant: wiring recordAttempt's IP/UA
// derivation only into the success call site (mintClientCredentialsToken)
// while leaving the failure call sites (handleClientCredentialsGrant) on the
// old signature would pass this test's sibling but fail this one.
func TestTokenRecordsIPAndUserAgent_Failure(t *testing.T) {
	s := stateForLoginAttemptIPUA(t)
	principalID := newActivePrincipal(t, "iprua-failure@token.test")
	clientID := "oac_iprua_failure"
	c := confidentialClient(t, s, clientID, principalID, "s3cr3t")
	s.OAuthClients = fakeClientFinder{client: c}

	rr := doTokenRequestWithHeaders(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {"WRONG-SECRET"},
	}, map[string]string{
		"X-Forwarded-For": "203.0.113.9, 198.51.100.7",
		"User-Agent":      "fc-test/1.0",
	})
	require.Equal(t, http.StatusUnauthorized, rr.Code, rr.Body.String())

	attempts, err := s.LoginAttempts.FindRecentByIdentifier(t.Context(), clientID, 1)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	a := attempts[0]
	require.Equal(t, loginattempt.OutcomeFailure, a.Outcome)
	require.NotNil(t, a.IPAddress)
	require.Equal(t, "198.51.100.7", *a.IPAddress)
	require.NotNil(t, a.UserAgent)
	require.Equal(t, "fc-test/1.0", *a.UserAgent)
}

// TestTokenRecordsIP_NoForwardedFor (T3): with no X-Forwarded-For header the
// stored IP is the remote address without its port (httptest's fixed
// "192.0.2.1:1234"), never empty. Mutant: deriving IP from the header alone
// (skipping the RemoteAddr fallback) would store nil here.
func TestTokenRecordsIP_NoForwardedFor(t *testing.T) {
	s := stateForLoginAttemptIPUA(t)
	principalID := newActivePrincipal(t, "iprua-noxff@token.test")
	clientID := "oac_iprua_noxff"
	c := confidentialClient(t, s, clientID, principalID, "s3cr3t")
	s.OAuthClients = fakeClientFinder{client: c}

	rr := doTokenRequestWithHeaders(t, s, url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {"s3cr3t"},
	}, map[string]string{
		"User-Agent": "fc-test/1.0",
	})
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	attempts, err := s.LoginAttempts.FindRecentByIdentifier(t.Context(), clientID, 1)
	require.NoError(t, err)
	require.Len(t, attempts, 1)
	a := attempts[0]
	require.NotNil(t, a.IPAddress)
	require.NotEqual(t, "", *a.IPAddress)
	require.Equal(t, "192.0.2.1", *a.IPAddress, "must fall back to the remote address without its port")
}
