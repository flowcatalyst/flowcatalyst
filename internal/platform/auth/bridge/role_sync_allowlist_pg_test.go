//go:build integration

package bridge

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A provider that serves a client or partner domain, syncs roles and has no
// allow-list could grant any platform role that any provider's role mapping
// names. Saving that is refused; one stored earlier keeps working with a
// once-per-provider warning, until FC_OIDC_ROLE_SYNC_REQUIRE_ALLOWLIST makes
// its sign-ins grant nothing.

// syncLog collects slog output for the test's duration.
type syncLog struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *syncLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *syncLog) count(substr string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return strings.Count(l.buf.String(), substr)
}

func captureLogs(t *testing.T) *syncLog {
	t.Helper()
	l := &syncLog{}
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(l, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return l
}

// resetRoleSyncWarnings forgets which providers were warned about, and sets the
// flag for the test.
func resetRoleSyncWarnings(t *testing.T, requireAllowList bool) {
	t.Helper()
	warnedRoleSyncIdPs.Range(func(k, _ any) bool { warnedRoleSyncIdPs.Delete(k); return true })
	SetRoleSyncRequireAllowList(requireAllowList)
	t.Cleanup(func() {
		SetRoleSyncRequireAllowList(false)
		warnedRoleSyncIdPs.Range(func(k, _ any) bool { warnedRoleSyncIdPs.Delete(k); return true })
	})
}

// provision signs the user in once while the fixture is still anchor-scoped
// with an allow-list, so the principal exists (a CLIENT account would need a
// client row). No roles are claimed.
func (f *oidcTestFixture) provision(t *testing.T, email string) {
	t.Helper()
	rec := f.login(t, map[string]any{"nonce": "n-prov-" + randString(6), "email": email}, "opaque")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
}

// makeTenantProvider turns the fixture's provider into one stored before the
// rule: its domain's mapping is CLIENT scope, and (when stripAllowList) its
// allow-list is gone. Written directly, as the use case now refuses this.
func (f *oidcTestFixture) makeTenantProvider(t *testing.T, stripAllowList bool) {
	t.Helper()
	ctx := context.Background()
	_, err := f.pool.Exec(ctx,
		`UPDATE tnt_email_domain_mappings SET scope_type = 'CLIENT', primary_client_id = $2 WHERE id = $1`,
		f.mappingID, "cli_"+strings.ToLower(randString(8)))
	require.NoError(t, err)
	if stripAllowList {
		_, err = f.pool.Exec(ctx,
			`DELETE FROM oauth_identity_provider_allowed_roles WHERE identity_provider_id = $1`, f.identityProviderID)
		require.NoError(t, err)
	}
}

// loginWithRole signs email in with the fixture's mapped raw role in the claim
// and returns the platform roles the user then holds.
func (f *oidcTestFixture) loginWithRole(t *testing.T, email string) []string {
	t.Helper()
	rec := f.login(t, map[string]any{
		"nonce": "n-role-" + randString(6), "email": email, "roles": []string{f.rawIdpRoleName},
	}, "opaque")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.True(t, f.lastSessionCalled, "the login completes")
	rows, err := f.pool.Query(context.Background(), `SELECT r.role_name FROM iam_principal_roles r
		JOIN iam_principals p ON p.id = r.principal_id WHERE p.email = $1 ORDER BY 1`, email)
	require.NoError(t, err)
	defer rows.Close()
	var got []string
	for rows.Next() {
		var n string
		require.NoError(t, rows.Scan(&n))
		got = append(got, n)
	}
	return got
}

const noAllowListWarning = "OIDC role sync has no allow-list"

// TA2: flag off, a tenant provider without an allow-list still grants the
// mapped role (today's behaviour) and is warned about once across two logins.
func TestRoleSync_FlagOff_TenantProviderWithoutAllowListStillGrantsAndWarnsOnce(t *testing.T) {
	f := newOidcTestFixture(t)
	resetRoleSyncWarnings(t, false)
	email := "ta2@" + f.domain
	f.provision(t, email)
	f.makeTenantProvider(t, true)
	logs := captureLogs(t)

	assert.Equal(t, []string{f.platformRoleName}, f.loginWithRole(t, email), "granted, as before")
	assert.Equal(t, []string{f.platformRoleName}, f.loginWithRole(t, email), "and again")
	assert.Equal(t, 1, logs.count(noAllowListWarning), "one warning per provider per process, not per login")
	assert.Equal(t, 1, logs.count("identity_provider="+f.identityProviderID))
}

// TA3: flag on, the same provider grants nothing; an anchor provider with no
// allow-list, and a tenant provider with an allow-list naming the role, are
// unaffected.
func TestRoleSync_FlagOn_TenantProviderWithoutAllowListGrantsNothing(t *testing.T) {
	f := newOidcTestFixture(t)
	resetRoleSyncWarnings(t, true)
	email := "ta3a@" + f.domain
	f.provision(t, email)
	f.makeTenantProvider(t, true)
	logs := captureLogs(t)

	assert.Empty(t, f.loginWithRole(t, email), "no roles granted")
	assert.Empty(t, f.loginWithRole(t, email))
	assert.Equal(t, 1, logs.count("OIDC role sync grants nothing"), "the refusal is logged once")
}

func TestRoleSync_FlagOn_FlagDropsRolesThatWereSyncedBefore(t *testing.T) {
	f := newOidcTestFixture(t)
	resetRoleSyncWarnings(t, false)
	email := "ta3d@" + f.domain
	f.provision(t, email)
	assert.Equal(t, []string{f.platformRoleName}, f.loginWithRole(t, email), "granted while still an anchor provider")

	f.makeTenantProvider(t, true)
	SetRoleSyncRequireAllowList(true)
	assert.Empty(t, f.loginWithRole(t, email), "the IdP-synced role is dropped, like an IdP that sent no mapped roles")
}

func TestRoleSync_FlagOn_AnchorProviderWithoutAllowListIsUnaffected(t *testing.T) {
	f := newOidcTestFixture(t)
	resetRoleSyncWarnings(t, true)
	email := "ta3b@" + f.domain
	f.provision(t, email)
	_, err := f.pool.Exec(context.Background(),
		`DELETE FROM oauth_identity_provider_allowed_roles WHERE identity_provider_id = $1`, f.identityProviderID)
	require.NoError(t, err)
	logs := captureLogs(t)

	assert.Equal(t, []string{f.platformRoleName}, f.loginWithRole(t, email))
	assert.Zero(t, logs.count(noAllowListWarning), "an anchor provider is not warned about")
}

func TestRoleSync_FlagOn_TenantProviderWithAllowListIsUnaffected(t *testing.T) {
	f := newOidcTestFixture(t)
	resetRoleSyncWarnings(t, true)
	email := "ta3c@" + f.domain
	f.provision(t, email)
	f.makeTenantProvider(t, false) // the allow-list (naming the role) stays
	logs := captureLogs(t)

	assert.Equal(t, []string{f.platformRoleName}, f.loginWithRole(t, email))
	assert.Zero(t, logs.count(noAllowListWarning))
}

// The start-up scan names each provider in the state once.
func TestWarnRoleSyncWithoutAllowList_LogsTheProviderOnce(t *testing.T) {
	f := newOidcTestFixture(t)
	f.makeTenantProvider(t, true)
	logs := captureLogs(t)

	WarnRoleSyncWithoutAllowList(context.Background(), f.endpoint.bridge.idps)
	assert.Equal(t, 1, logs.count("identity_provider="+f.identityProviderID), "named once")
	assert.Positive(t, logs.count("no allow-list"))
}
