//go:build integration

package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/pushgw"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The session cookie has two names: __Host-fc_session (HTTPS) and the old
// fc_session. These tests drive the whole router with real cookies under both
// names and both process settings. The setting is process-wide state, so none of
// them runs in parallel and each restores it.

func hostCookieSecure(t *testing.T, secure bool) {
	t.Helper()
	platformmw.SetSessionCookieSecure(secure)
	t.Cleanup(func() { platformmw.SetSessionCookieSecure(false) })
}

type namedCookie struct{ name, value string }

func (x *saHarness) meAs(cookies ...namedCookie) (status int, principalID string) {
	x.t.Helper()
	r := httptest.NewRequest(http.MethodGet, "http://platform.example.com/auth/me", nil)
	for _, c := range cookies {
		r.AddCookie(&http.Cookie{Name: c.name, Value: c.value})
	}
	r.Header.Set("Sec-Fetch-Site", "same-origin")
	w := httptest.NewRecorder()
	x.h.ServeHTTP(w, r)
	var body struct {
		PrincipalID string `json:"principalId"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body.PrincipalID
}

// The four rows of the acceptance rule, through the real Authenticator, plus the
// new name.
func TestHostCookieAcceptanceRule(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	marked := x.mintNow(pid)                                  // minted after the upgrade: carries the marker
	unmarked := x.cookieAt(pid, time.Now().Add(-time.Minute)) // a session from before the upgrade: no marker
	old, host := platformmw.SessionCookieName, platformmw.HostSessionCookieName

	hostCookieSecure(t, true)
	code, _ := x.meAs(namedCookie{old, marked})
	require.Equal(t, http.StatusUnauthorized, code, "secure + old name + marker: a planted cookie must not sign in")
	code, got := x.meAs(namedCookie{old, unmarked})
	require.Equal(t, http.StatusOK, code, "secure + old name + no marker: a pre-upgrade session keeps working")
	require.Equal(t, pid, got)
	code, got = x.meAs(namedCookie{host, marked})
	require.Equal(t, http.StatusOK, code, "secure + __Host- name")
	require.Equal(t, pid, got)

	hostCookieSecure(t, false)
	code, _ = x.meAs(namedCookie{old, marked})
	require.Equal(t, http.StatusOK, code, "not secure + old name + marker (plain-HTTP development)")
	code, _ = x.meAs(namedCookie{old, unmarked})
	require.Equal(t, http.StatusOK, code, "not secure + old name + no marker")
}

// Both names present: the __Host- one wins, even when the old one holds a valid
// token for another principal (a planted cookie cannot displace the real one).
func TestHostCookieWinsOverOldName(t *testing.T) {
	x := newSAHarness(t, 0)
	victim, _ := x.newUser()
	attacker, _ := x.newUser()
	require.NotEqual(t, victim, attacker)
	for _, secure := range []bool{true, false} {
		hostCookieSecure(t, secure)
		code, got := x.meAs(
			namedCookie{platformmw.SessionCookieName, x.cookieAt(attacker, time.Now().Add(-time.Minute))},
			namedCookie{platformmw.HostSessionCookieName, x.mintNow(victim)},
		)
		require.Equal(t, http.StatusOK, code, "secure=%v", secure)
		require.Equal(t, victim, got, "secure=%v: the old-name cookie displaced the __Host- one", secure)
	}
}

// AuthenticateRequest (the push gateway's reader) applies the same rule.
func TestHostCookieRuleInAuthenticateRequest(t *testing.T) {
	x := newSAHarness(t, 0)
	pid, _ := x.newUser()
	marked := x.mintNow(pid)
	unmarked := x.cookieAt(pid, time.Now().Add(-time.Minute))
	resolve := func(c namedCookie) (bool, error) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: c.name, Value: c.value})
		_, present, err := platformmw.AuthenticateRequest(context.Background(), x.svcs.authProvider, r)
		return present, err
	}

	hostCookieSecure(t, true)
	present, err := resolve(namedCookie{platformmw.SessionCookieName, marked})
	require.NoError(t, err)
	require.False(t, present, "secure + old name + marker")
	present, err = resolve(namedCookie{platformmw.SessionCookieName, unmarked})
	require.NoError(t, err)
	require.True(t, present, "secure + old name + no marker")
	present, err = resolve(namedCookie{platformmw.HostSessionCookieName, marked})
	require.NoError(t, err)
	require.True(t, present, "secure + __Host- name")

	hostCookieSecure(t, false)
	present, err = resolve(namedCookie{platformmw.SessionCookieName, marked})
	require.NoError(t, err)
	require.True(t, present, "not secure + old name + marker")
}

// /oauth/authorize is the reader that turns a session into an authorization code.
func TestAuthorizeAppliesHostCookieRule(t *testing.T) {
	x := newSAHarness(t, 0)
	pool := testpg.Pool(t)
	require.NoError(t, seed.NewSeeder(pool).SeedFcdevCLIClient(context.Background()))
	pid, _ := x.newUser()
	q := url.Values{
		"response_type": {"code"}, "client_id": {platformauth.FcdevCLIClientID},
		"redirect_uri": {"http://127.0.0.1:53682/callback"}, "state": {"st4te"},
		"code_challenge": {strings.Repeat("a", 43)}, "code_challenge_method": {"S256"},
	}
	issuesCode := func(c namedCookie) bool {
		r := httptest.NewRequest(http.MethodGet, "http://platform.example.com/oauth/authorize?"+q.Encode(), nil)
		r.AddCookie(&http.Cookie{Name: c.name, Value: c.value})
		w := httptest.NewRecorder()
		x.h.ServeHTTP(w, r)
		loc := w.Header().Get("Location")
		return strings.HasPrefix(loc, "http://127.0.0.1:53682/callback") && strings.Contains(loc, "code=")
	}
	marked := x.mintNow(pid)
	unmarked := x.cookieAt(pid, time.Now().Add(-time.Minute))
	old, host := platformmw.SessionCookieName, platformmw.HostSessionCookieName

	hostCookieSecure(t, true)
	require.False(t, issuesCode(namedCookie{old, marked}), "secure + old name + marker: a planted cookie was given a code")
	require.True(t, issuesCode(namedCookie{old, unmarked}), "secure + old name + no marker")
	require.True(t, issuesCode(namedCookie{host, marked}), "secure + __Host- name")

	hostCookieSecure(t, false)
	require.True(t, issuesCode(namedCookie{old, marked}), "not secure + old name + marker")
}

// Sign-out through the push hook reads whichever name is present, and announces
// the signed-out principal.
func TestPushSignOutReadsEitherName(t *testing.T) {
	x := newSAHarness(t, 0)
	pool := testpg.Pool(t)
	pid, _ := x.newUser()
	hostCookieSecure(t, true)
	hook := pushSignOut(x.svcs.authProvider, pool)

	conn, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	defer conn.Release()
	_, err = conn.Exec(context.Background(), `LISTEN `+pushgw.ChannelPrincipalEnd)
	require.NoError(t, err)

	for _, name := range []string{platformmw.SessionCookieName, platformmw.HostSessionCookieName} {
		r := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
		r.AddCookie(&http.Cookie{Name: name, Value: x.mintNow(pid)})
		hook(r)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		n, err := conn.Conn().WaitForNotification(ctx)
		cancel()
		require.NoError(t, err, "sign-out with the %s cookie announced nothing", name)
		require.Equal(t, pid, n.Payload, name)
	}
}
