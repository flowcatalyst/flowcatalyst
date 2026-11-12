//go:build integration

package oauthapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// A developer's client_credentials token (client_id = the user's own id) for a user
// whose email domain is SSO-managed also needs a recent interactive sign-in
// (FC_DEVELOPER_CREDENTIAL_SSO_MAX_AGE_DAYS); with the setting off, a stale one is
// logged once per principal per process.

const day = 24 * time.Hour

func ssoDevEncryption(t *testing.T) *encryption.Service {
	t.Helper()
	k := make([]byte, 32)
	_, err := rand.Read(k)
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", base64.StdEncoding.EncodeToString(k))
	t.Setenv("FLOWCATALYST_APP_KEY_PREVIOUS", "")
	enc, err := encryption.FromEnv()
	require.NoError(t, err)
	require.NotNil(t, enc)
	return enc
}

func ssoDevState(t *testing.T, enc *encryption.Service, maxAgeDays int) *State {
	t.Helper()
	pool := testpg.Pool(t)
	s := testStateForDeveloperGrant(t, enc)
	s.Mappings = emaildomainmapping.NewRepository(pool)
	s.IdentityProviders = identityprovider.NewRepository(pool)
	s.LoginAttempts = loginattempt.NewRepository(pool)
	s.DeveloperCredentialSSOMaxAge = time.Duration(maxAgeDays) * day
	return s
}

// ssoDeveloper creates a developer user whose domain IS mapped to an OIDC provider,
// and records a successful interactive sign-in lastLogin ago (zero: never).
func ssoDeveloper(t *testing.T, tag string, sso bool, lastLogin time.Duration) (pid, secret string) {
	t.Helper()
	domain := tag + ".sso-dev.test"
	pid, secret = seedDeveloperGrantFixture(t, "dev@"+domain)
	pool := testpg.Pool(t)
	ctx := context.Background()
	if sso {
		idpID := "idp_" + string(ids.NewPrincipalID())[4:]
		_, err := pool.Exec(ctx,
			`INSERT INTO oauth_identity_providers (id, code, name, type, oidc_issuer_url, oidc_client_id)
			 VALUES ($1, $2, 'SSO test', 'OIDC', 'https://idp.example.test', 'client')`, idpID, "sso-"+domain)
		require.NoError(t, err)
		_, err = pool.Exec(ctx,
			`INSERT INTO tnt_email_domain_mappings (id, email_domain, identity_provider_id, scope_type)
			 VALUES ($1, $2, $3, 'ANCHOR')`, "edm_"+string(ids.NewPrincipalID())[4:], domain, idpID)
		require.NoError(t, err)
	}
	if lastLogin > 0 {
		a := loginattempt.New(loginattempt.AttemptUserLogin, loginattempt.OutcomeSuccess)
		a.PrincipalID = &pid
		email := "dev@" + domain
		a.Identifier = &email
		a.AttemptedAt = time.Now().Add(-lastLogin).UTC()
		require.NoError(t, loginattempt.NewRepository(pool).Record(ctx, a))
	}
	return pid, secret
}

func devToken(t *testing.T, s *State, pid, secret string) (int, string) {
	t.Helper()
	rr := doTokenRequest(t, s, url.Values{"grant_type": {"client_credentials"}, "client_id": {pid}, "client_secret": {secret}})
	return rr.Code, rr.Body.String()
}

func TestDeveloperCredentialSSOMaxAge(t *testing.T) {
	enc := ssoDevEncryption(t)
	s := ssoDevState(t, enc, 30)

	// what an inactive principal is told (the status and body the refusal must equal)
	inactive, inactiveSecret := ssoDeveloper(t, "inactive", false, 0)
	_, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), principalops.DeactivateUser(principal.NewRepository(testpg.Pool(t))),
		principalops.DeactivateCommand{ID: ids.PrincipalID(inactive)}, testpg.TestEC())
	require.NoError(t, err)
	wantStatus, wantBody := devToken(t, s, inactive, inactiveSecret)
	require.Equal(t, http.StatusUnauthorized, wantStatus)
	require.Contains(t, wantBody, "invalid_client")

	t.Run("SSO-managed, last sign-in older than the setting: refused like an inactive principal", func(t *testing.T) {
		pid, secret := ssoDeveloper(t, "stale", true, 40*day)
		status, body := devToken(t, s, pid, secret)
		require.Equal(t, wantStatus, status, body)
		require.Equal(t, wantBody, body)
		require.NotContains(t, body, "access_token")
	})
	t.Run("SSO-managed, never signed in: refused", func(t *testing.T) {
		pid, secret := ssoDeveloper(t, "never", true, 0)
		status, body := devToken(t, s, pid, secret)
		require.Equal(t, wantStatus, status, body)
		require.Equal(t, wantBody, body)
	})
	t.Run("SSO-managed, recent sign-in: token", func(t *testing.T) {
		pid, secret := ssoDeveloper(t, "recent", true, 10*day)
		status, body := devToken(t, s, pid, secret)
		require.Equal(t, http.StatusOK, status, body)
		require.Contains(t, body, "access_token")
	})
	t.Run("not SSO-managed, old or no sign-in: token", func(t *testing.T) {
		for tag, last := range map[string]time.Duration{"plainold": 400 * day, "plainnone": 0} {
			pid, secret := ssoDeveloper(t, tag, false, last)
			status, body := devToken(t, s, pid, secret)
			require.Equal(t, http.StatusOK, status, "%s: %s", tag, body)
		}
	})
}

// captureWarnings routes slog's default logger to a recorder for the test.
type warnRecorder struct {
	mu   sync.Mutex
	msgs []string
}

func (w *warnRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (w *warnRecorder) WithAttrs([]slog.Attr) slog.Handler       { return w }
func (w *warnRecorder) WithGroup(string) slog.Handler            { return w }
func (w *warnRecorder) Handle(_ context.Context, r slog.Record) error {
	if r.Level != slog.LevelWarn {
		return nil
	}
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool { b.WriteString(" " + a.Key + "=" + a.Value.String()); return true })
	w.mu.Lock()
	w.msgs = append(w.msgs, b.String())
	w.mu.Unlock()
	return nil
}

func (w *warnRecorder) countFor(pid string) int {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := 0
	for _, m := range w.msgs {
		if strings.Contains(m, "SSO-managed user") && strings.Contains(m, pid) {
			n++
		}
	}
	return n
}

func TestDeveloperCredentialSSOStaleIsLoggedOnceWhenSettingIsOff(t *testing.T) {
	enc := ssoDevEncryption(t)
	rec := &warnRecorder{}
	prev := slog.Default()
	slog.SetDefault(slog.New(rec))
	t.Cleanup(func() { slog.SetDefault(prev) })

	s := ssoDevState(t, enc, 0)
	stale, staleSecret := ssoDeveloper(t, "offstale", true, 40*day)
	recent, recentSecret := ssoDeveloper(t, "offrecent", true, 5*day)
	plain, plainSecret := ssoDeveloper(t, "offplain", false, 400*day)

	for range 2 {
		status, body := devToken(t, s, stale, staleSecret)
		require.Equal(t, http.StatusOK, status, "with the setting off the request proceeds: %s", body)
	}
	require.Equal(t, 1, rec.countFor(stale), "one warning per principal across two requests")

	for _, c := range []struct{ pid, secret string }{{recent, recentSecret}, {plain, plainSecret}} {
		status, body := devToken(t, s, c.pid, c.secret)
		require.Equal(t, http.StatusOK, status, body)
		require.Zero(t, rec.countFor(c.pid), "no warning for a recent sign-in or a non-SSO user")
	}
}
