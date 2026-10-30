//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The service accounts page end to end against Postgres: the handlers run
// the API's use cases (ServiceAccountOps), so the rows, the events, the role
// ceiling and the audit records are the API's, and the credentials the dialogs
// show once are the ones stored.
func TestServiceAccountsPageAgainstPostgres(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key) // client secrets are hashed, webhook secrets encrypted with it
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	clients := client.NewRepository(pool)
	roles := role.NewRepository(pool)
	apps := application.NewRepository(pool)
	svc := authservice.NewWithSecret(authservice.Config{SecretKey: "ui-sa-test-secret-ui-sa-test-secret-ui", Issuer: "https://fc.test", Audience: "https://fc.test"})

	var clientIDs []string
	for _, ident := range []string{"ui-sa-acme", "ui-sa-globex"} {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: ident, Identifier: ident}, testpg.TestEC())
		require.NoError(t, err)
		clientIDs = append(clientIDs, ev.ClientID)
	}
	testpg.EnsureApplication(t, "uisa")
	mkRole := func(name string, perms ...string) string {
		t.Helper()
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: "uisa", RoleName: name, DisplayName: name, Permissions: perms}, testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	power := mkRole("power", "platform:iam:user:create", "platform:iam:user:view")
	viewer := mkRole("viewer", "platform:iam:user:view")
	appID, err := apps.FindByCode(ctx, "uisa")
	require.NoError(t, err)
	require.NotNil(t, appID)

	page := &ServiceAccounts{
		Accounts: saRepo, Principals: principals, OAuthClients: oauth, Roles: roles, Clients: clients, Apps: apps,
		Connections: connection.NewRepository(pool), Shell: shellForTest(),
		Ops: ServiceAccountOps{
			Repo: saRepo, Principals: principals, OAuthClients: oauth, Clients: clients, Grants: principal.NewClientAccessGrantRepo(pool),
			Roles: roles, Applications: apps, UoW: uow, Auth: svc,
			FlattenPermissions: func(context.Context, []string) ([]string, error) { return []string{"platform:events:create"}, nil },
			Audit:              audit.NewRepository(pool),
		},
	}
	as := func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
		t.Helper()
		return serveMounted(t, ac, q, page)
	}
	do := func(ac *auth.AuthContext, q req) string {
		t.Helper()
		rec := as(ac, q)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	admin := saAdmin()
	admin.PrincipalID = "prn_ui_sa_admin"

	// Create: the dialog shows the credentials that were stored.
	body := do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"UI SA one"}, "code": {"ui-sa-one"}, "description": {"From the UI"}, "scope": {"CLIENT"}, "clientIds": {clientIDs[0]},
	}})
	require.Contains(t, body, "Service Account Created", body)
	sa, err := saRepo.FindByCode(ctx, "ui-sa-one")
	require.NoError(t, err)
	require.NotNil(t, sa)
	saID := sa.ID
	assert.Equal(t, []string{clientIDs[0]}, sa.ClientIDs)
	assert.Equal(t, "CLIENT", deref(sa.Scope))
	require.NotNil(t, sa.WebhookCredentials.Token)
	require.NotNil(t, sa.WebhookCredentials.SigningSecret)
	assert.Contains(t, body, *sa.WebhookCredentials.Token, "the dialog shows the stored auth token")
	assert.Contains(t, body, *sa.WebhookCredentials.SigningSecret, "the dialog shows the stored signing secret")
	p, err := principals.FindByServiceAccount(ctx, saID)
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, principal.ScopeClient, p.Scope, "the principal's reach follows the links")
	assert.False(t, p.AllApplications, "a new account has no application access")
	ocs, err := oauth.FindByPrincipalID(ctx, string(p.ID))
	require.NoError(t, err)
	require.Len(t, ocs, 1)
	assert.Contains(t, body, ocs[0].ClientID)
	assert.Equal(t, 1, auditCount(t, saID), "create is audited like the API")

	// Nothing shows the credentials again.
	drawer := do(admin, req{target: "/ui/service-accounts/" + saID, datastar: true})
	assert.Contains(t, drawer, ocs[0].ClientID, "the public client id is shown")
	assert.NotContains(t, drawer, *sa.WebhookCredentials.Token)
	assert.NotContains(t, drawer, *sa.WebhookCredentials.SigningSecret)
	assert.NotContains(t, do(admin, req{target: "/ui/service-accounts"}), *sa.WebhookCredentials.Token)

	// The use case's refusals are shown on the form: a taken code, a scope that
	// disagrees with the links.
	dup := do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{"name": {"Again"}, "code": {"ui-sa-one"}, "scope": {"ANCHOR"}}})
	assert.Contains(t, dup, "already exists")
	assert.Contains(t, dup, `id="sa-form-code-field"`)
	mismatch := do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{"name": {"M"}, "code": {"ui-sa-mismatch"}, "scope": {"PARTNER"}, "clientIds": {clientIDs[0]}}})
	assert.Contains(t, mismatch, "does not match")
	gone, err := saRepo.FindByCode(ctx, "ui-sa-mismatch")
	require.NoError(t, err)
	assert.Nil(t, gone)

	// Edit: two links make it PARTNER, and its principal follows.
	do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/edit", datastar: true, form: url.Values{
		"name": {"UI SA one b"}, "description": {""}, "scope": {"PARTNER"}, "clientIds": {clientIDs[0], clientIDs[1]}, "drawer": {saID},
	}})
	edited, err := saRepo.FindByID(ctx, saID)
	require.NoError(t, err)
	assert.Equal(t, "UI SA one b", edited.Name)
	assert.Equal(t, "From the UI", deref(edited.Description), "a blank description is not sent: the stored one stays")
	assert.ElementsMatch(t, clientIDs, edited.ClientIDs)
	p, _ = principals.FindByServiceAccount(ctx, saID)
	assert.Equal(t, principal.ScopePartner, p.Scope)
	// Back to an anchor account: the links are cleared.
	do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/edit", datastar: true, form: url.Values{
		"name": {"UI SA one b"}, "scope": {"ANCHOR"}, "clientIds": {clientIDs[0]},
	}})
	edited, _ = saRepo.FindByID(ctx, saID)
	assert.Empty(t, edited.ClientIDs)
	p, _ = principals.FindByServiceAccount(ctx, saID)
	assert.Equal(t, principal.ScopeAnchor, p.Scope)

	// Roles: the role ceiling is the use case's. A caller that lacks the
	// permissions of a role cannot grant it, and nothing is written.
	held := func() []string {
		t.Helper()
		pr, err := principals.FindByServiceAccount(ctx, saID)
		require.NoError(t, err)
		out := []string{}
		for _, ra := range pr.Roles {
			out = append(out, ra.Role.String())
		}
		return out
	}
	lesser := &auth.AuthContext{
		PrincipalID: "prn_ui_sa_lesser", Scope: auth.ScopeAnchor, AllApplications: true, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:iam:service-account:view", "platform:iam:service-account:update", "platform:iam:role:view", "platform:iam:user:view"},
	}
	dialog := do(lesser, req{target: "/ui/service-accounts/" + saID + "/roles", datastar: true})
	assert.Contains(t, dialog, `value="`+viewer+`"`)
	assert.NotContains(t, dialog, `value="`+power+`"`, "a role above the caller is not offered")
	refused := do(lesser, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/roles", datastar: true, form: url.Values{"roles": {viewer, power}}})
	assert.Contains(t, refused, "You may only assign or remove roles whose permissions you hold yourself")
	assert.Empty(t, held(), "a refused assignment writes nothing")
	do(lesser, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/roles", datastar: true, form: url.Values{"roles": {viewer}}})
	assert.ElementsMatch(t, []string{viewer}, held())
	do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/roles", datastar: true, form: url.Values{"roles": {viewer, power}}})
	assert.ElementsMatch(t, []string{viewer, power}, held())
	// A held role above the caller is locked in the dialog and kept by a save.
	locked := do(lesser, req{target: "/ui/service-accounts/" + saID + "/roles", datastar: true})
	assert.Contains(t, locked, "Locked roles")
	assert.Contains(t, locked, `<input type="hidden" name="roles" value="`+power+`">`)
	do(lesser, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/roles", datastar: true, form: url.Values{"roles": {power}}})
	assert.ElementsMatch(t, []string{power}, held(), "the locked role stays; the one the caller may remove goes")

	// Credentials: each regeneration shows the new value once and stores it.
	oldToken := *sa.WebhookCredentials.Token
	rt := as(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/regenerate-token", datastar: true, form: url.Values{"drawer": {saID}}})
	require.Equal(t, http.StatusOK, rt.Code)
	after, _ := saRepo.FindByID(ctx, saID)
	require.NotNil(t, after.WebhookCredentials.Token)
	assert.NotEqual(t, oldToken, *after.WebhookCredentials.Token)
	assert.Contains(t, rt.Body.String(), *after.WebhookCredentials.Token)
	assert.NotContains(t, rt.Body.String(), oldToken)
	assert.Equal(t, "no-store", rt.Header().Get("Cache-Control"))
	rs := as(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/regenerate-secret", datastar: true, form: url.Values{"drawer": {saID}}})
	after2, _ := saRepo.FindByID(ctx, saID)
	assert.Contains(t, rs.Body.String(), *after2.WebhookCredentials.SigningSecret)
	assert.NotEqual(t, *sa.WebhookCredentials.SigningSecret, *after2.WebhookCredentials.SigningSecret)
	assert.NotContains(t, do(admin, req{target: "/ui/service-accounts/" + saID, datastar: true}), *after2.WebhookCredentials.SigningSecret)

	// Mint: a live bearer for the account, audited; the ceiling bounds it (the
	// account holds power, which the lesser caller lacks).
	mint := as(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/token", datastar: true, form: url.Values{"drawer": {saID}}})
	require.Equal(t, http.StatusOK, mint.Code)
	require.Contains(t, mint.Body.String(), "Bearer Token", mint.Body.String())
	assert.Equal(t, "no-store", mint.Header().Get("Cache-Control"))
	var audited int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM aud_logs WHERE entity_id = $1 AND operation = 'TOKEN_MINTED_BY_ADMIN' AND principal_id = $2`, saID, admin.PrincipalID).Scan(&audited))
	assert.Equal(t, 1, audited, "the mint is audited with the caller, never the token")
	above := do(lesser, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/token", datastar: true, form: url.Values{}})
	assert.Contains(t, above, "You may only mint a token for a service account whose roles")
	assert.NotContains(t, above, `id="sa-credentials"`)

	// Application access, through the principal use case.
	do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/applications", datastar: true, form: url.Values{"applicationIds": {string(appID.ID)}}})
	p, _ = principals.FindByServiceAccount(ctx, saID)
	assert.Equal(t, []string{string(appID.ID)}, idsOfApps(p))
	assert.False(t, p.AllApplications)
	do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/all-applications", datastar: true, form: url.Values{"allApplications": {"true"}}})
	p, _ = principals.FindByServiceAccount(ctx, saID)
	assert.True(t, p.AllApplications)
	assert.Equal(t, []string{string(appID.ID)}, idsOfApps(p), "the explicit grants are kept")

	// Forged posts: a non-anchor caller holding every permission changes nothing.
	clientAdmin := &auth.AuthContext{
		PrincipalID: "prn_ui_sa_client", Scope: auth.ScopeClient, Clients: []string{clientIDs[0]}, AllApplications: true, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:*:*:*"},
	}
	rolesBefore := held()
	for _, f := range []struct{ route string }{{"/roles"}, {"/regenerate-token"}, {"/regenerate-secret"}, {"/token"}, {"/delete"}, {"/edit"}, {"/applications"}} {
		rec := as(clientAdmin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + f.route, datastar: true, form: url.Values{"roles": {viewer}, "name": {"Forged"}, "scope": {"ANCHOR"}}})
		assert.NotContains(t, rec.Body.String(), `id="sa-credentials"`, f.route)
	}
	still, err := saRepo.FindByID(ctx, saID)
	require.NoError(t, err)
	require.NotNil(t, still, "a forged delete removed the account")
	assert.Equal(t, "UI SA one b", still.Name)
	assert.Equal(t, rolesBefore, held())
	assert.Equal(t, *after2.WebhookCredentials.Token, *still.WebhookCredentials.Token, "a forged regeneration rotated the token")

	// Delete from the drawer.
	del := do(admin, req{method: http.MethodPost, target: "/ui/service-accounts/" + saID + "/delete", datastar: true, form: url.Values{"drawer": {saID}}})
	assert.Contains(t, del, `"selSa":""`)
	deleted, err := saRepo.FindByID(ctx, saID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}

func idsOfApps(p *principal.Principal) []string {
	out := make([]string, 0, len(p.AccessibleApplicationIDs))
	for _, id := range p.AccessibleApplicationIDs {
		out = append(out, string(id))
	}
	return out
}
