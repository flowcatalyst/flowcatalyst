//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// The OAuth clients page end to end against Postgres: the handlers run the
// API's use cases (OAuthClientOps), so the rows, the events and the audit
// records are the API's, and the secrets the dialogs show once are the ones
// stored (verified against the stored references with the key).
func TestOAuthClientsPageAgainstPostgres(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key) // client secrets are hashed with it
	enc, err := encryption.New(key)
	require.NoError(t, err)
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	oauth := platformauth.NewRepository(pool).OAuthClients
	clients := client.NewRepository(pool)
	apps := application.NewRepository(pool)
	portalApps := portalidentity.NewAppRepository(pool)

	owner, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "OC Owner", Identifier: "ui-oc-owner"}, testpg.TestEC())
	require.NoError(t, err)
	other, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "OC Other", Identifier: "ui-oc-other"}, testpg.TestEC())
	require.NoError(t, err)
	appID := testpg.EnsureApplication(t, "uioc")
	portalApp := portalidentity.NewApp(owner.ClientID, "oc-portal", "OC Portal")
	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, portalApps.Persist(ctx, portalApp, usecasepgx.WrapTxForBootstrap(tx)))
	require.NoError(t, tx.Commit(ctx))

	page := &OAuthClients{
		Clients: oauth, Ops: OAuthClientOps{Repo: oauth, PortalApps: portalApps, UoW: uow},
		Apps: apps, ClientNames: clients, Portal: PortalReader{PortalApps: portalApps}, Shell: shellForTest(),
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
	admin := anchorAdmin()
	admin.PrincipalID = "prn_ui_oc_admin"
	byName := func(name string) *platformauth.OAuthClient {
		t.Helper()
		all, err := oauth.FindAll(ctx)
		require.NoError(t, err)
		for i := range all {
			if all[i].ClientName == name {
				return &all[i]
			}
		}
		return nil
	}

	// Create a CONFIDENTIAL client: the dialog shows the stored secret once.
	created := as(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"UI OC one"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://one.example.com/cb\nhttp://localhost:3000/cb"},
		"postLogoutRedirectUris": {"https://one.example.com/bye"}, "allowedOrigins": {"https://one.example.com/"},
		"grantTypes": {"authorization_code", "refresh_token"}, "defaultScopes": {"openid", "email"}, "pkceRequired": {"true"}, "applicationIds": {appID},
	}})
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	one := byName("UI OC one")
	require.NotNil(t, one)
	assert.Equal(t, platformauth.OAuthClientConfidential, one.ClientType)
	assert.Equal(t, []string{"https://one.example.com/cb", "http://localhost:3000/cb"}, one.RedirectURIs)
	assert.Equal(t, []string{"https://one.example.com"}, one.AllowedOrigins, "an origin is stored as scheme and host")
	assert.Equal(t, []string{"openid", "email"}, one.Scopes)
	assert.Equal(t, []string{appID}, one.ApplicationIDs)
	assert.True(t, one.PKCERequired)
	require.NotNil(t, one.SecretRef)
	body := created.Body.String()
	secret := secretIn(t, body, "oc-cred-1")
	ok, _ := enc.VerifySecret(*one.SecretRef, secret)
	assert.True(t, ok, "the dialog shows the stored secret")
	assert.Contains(t, body, one.ClientID)
	assert.NotContains(t, body, *one.SecretRef, "the stored reference is never shown")
	assert.Equal(t, "no-store", created.Header().Get("Cache-Control"))
	assert.Equal(t, 1, auditCount(t, one.ID), "create is audited like the API")

	// Nothing shows the secret again.
	assert.NotContains(t, do(admin, req{target: "/ui/oauth-clients/" + one.ID, datastar: true}), secret)
	assert.NotContains(t, do(admin, req{target: "/ui/oauth-clients"}), secret)
	assert.NotContains(t, do(admin, req{target: "/ui/oauth-clients/" + one.ID + "/edit", datastar: true}), secret)

	// Validation and the use case's refusals are shown on the form; nothing is written.
	dup := do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"UI OC conflict"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://c.example.com/cb"}, "grantTypes": {"authorization_code"},
		"portalClientId": {owner.ClientID}, "apiAccess": {"true"},
	}})
	assert.Contains(t, dup, "portal client cannot have apiAccess")
	assert.Nil(t, byName("UI OC conflict"))
	mismatch := do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"UI OC mismatch"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://c.example.com/cb"}, "grantTypes": {"authorization_code"},
		"portalClientId": {other.ClientID}, "portalAppId": {portalApp.ID},
	}})
	assert.Contains(t, mismatch, "belongs to a different client")
	assert.Nil(t, byName("UI OC mismatch"))

	// A PUBLIC portal client linked to the owner's portal app: PKCE is forced on,
	// the app is shown by name.
	do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"UI OC portal"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://p.example.com/cb"}, "grantTypes": {"authorization_code"},
		"portalAppId": {portalApp.ID}, "portalClientId": {owner.ClientID},
	}})
	portal := byName("UI OC portal")
	require.NotNil(t, portal)
	assert.True(t, portal.PKCERequired)
	require.NotNil(t, portal.PortalAppID)
	assert.Equal(t, portalApp.ID, *portal.PortalAppID)
	assert.Nil(t, portal.SecretRef, "a public client has no secret")
	drawer := do(admin, req{target: "/ui/oauth-clients/" + portal.ID, datastar: true})
	assert.Contains(t, drawer, "OC Owner")
	assert.Contains(t, drawer, "OC Portal (oc-portal)")
	opts := do(admin, req{target: "/ui/oauth-clients/portal-app-options?portalClientId=" + owner.ClientID, datastar: true})
	assert.Contains(t, opts, portalApp.ID)
	assert.NotContains(t, do(admin, req{target: "/ui/oauth-clients/portal-app-options?portalClientId=" + other.ClientID, datastar: true}), portalApp.ID)

	// Edit: emptied lists clear, the owner clears with its app.
	do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + portal.ID + "/edit", datastar: true, form: url.Values{
		"clientName": {"UI OC portal b"}, "redirectUris": {"https://p.example.com/cb2"}, "grantTypes": {"authorization_code"}, "drawer": {portal.ID},
	}})
	edited := byName("UI OC portal b")
	require.NotNil(t, edited)
	assert.Nil(t, edited.PortalClientID)
	assert.Nil(t, edited.PortalAppID)
	assert.Equal(t, []string{"https://p.example.com/cb2"}, edited.RedirectURIs)
	assert.Empty(t, edited.ApplicationIDs)
	assert.True(t, edited.PKCERequired, "a public client's PKCE stays on")

	// Rotation: the 24-hour overlap keeps the old secret, the new one is shown once.
	rot := as(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/rotate-secret", datastar: true, form: url.Values{"drawer": {one.ID}}})
	require.Equal(t, http.StatusOK, rot.Code)
	assert.Equal(t, "no-store", rot.Header().Get("Cache-Control"))
	after, err := oauth.FindByID(ctx, one.ID)
	require.NoError(t, err)
	newSecret := secretIn(t, rot.Body.String(), "oc-cred-0")
	ok, _ = enc.VerifySecret(*after.SecretRef, newSecret)
	assert.True(t, ok, "the dialog shows the new stored secret")
	ok, _ = enc.VerifySecret(*after.SecretRef, secret)
	assert.False(t, ok, "the old secret is no longer the current one")
	prev := after.UsablePreviousSecretRef()
	require.NotNil(t, prev, "the old secret keeps working for the overlap")
	ok, _ = enc.VerifySecret(*prev, secret)
	assert.True(t, ok)
	require.NotNil(t, after.PreviousSecretExpiresAt)
	assert.WithinDuration(t, after.UpdatedAt.Add(24*time.Hour), *after.PreviousSecretExpiresAt, 5*time.Second, "the overlap is 24 hours")
	assert.Contains(t, rot.Body.String(), "keeps working until")
	drawerWithOverlap := do(admin, req{target: "/ui/oauth-clients/" + one.ID, datastar: true})
	assert.Contains(t, drawerWithOverlap, "Previous secret still accepted")
	assert.Contains(t, drawerWithOverlap, "Revoke Previous Secret")
	assert.NotContains(t, drawerWithOverlap, newSecret)

	// Revoke the previous secret now.
	do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/revoke-previous-secret", datastar: true, form: url.Values{"drawer": {one.ID}}})
	revoked, err := oauth.FindByID(ctx, one.ID)
	require.NoError(t, err)
	assert.Nil(t, revoked.UsablePreviousSecretRef())
	assert.Nil(t, revoked.PreviousSecretRef)
	assert.NotContains(t, do(admin, req{target: "/ui/oauth-clients/" + one.ID, datastar: true}), "Previous secret still accepted")

	// Rotate without overlap: the old secret stops at once.
	now := as(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/rotate-secret-now", datastar: true, form: url.Values{"drawer": {one.ID}}})
	cut, err := oauth.FindByID(ctx, one.ID)
	require.NoError(t, err)
	assert.Nil(t, cut.PreviousSecretRef, "no overlap is kept")
	ok, _ = enc.VerifySecret(*cut.SecretRef, secretIn(t, now.Body.String(), "oc-cred-0"))
	assert.True(t, ok)
	assert.Contains(t, now.Body.String(), "The previous secret stopped working immediately.")
	// A public client cannot be rotated.
	refused := do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + edited.ID + "/rotate-secret", datastar: true, form: url.Values{}})
	assert.Contains(t, refused, "Only CONFIDENTIAL clients have rotatable secrets")
	assert.NotContains(t, refused, `id="oc-credentials"`)

	// Deactivate, activate, delete.
	do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/deactivate", datastar: true, form: url.Values{"drawer": {one.ID}}})
	off, _ := oauth.FindByID(ctx, one.ID)
	assert.False(t, off.Active)
	do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/activate", datastar: true, form: url.Values{}})
	on, _ := oauth.FindByID(ctx, one.ID)
	assert.True(t, on.Active)

	// Forged posts: a caller with only the view permission, and a non-anchor holding
	// every permission, change nothing and are shown no secret.
	viewer := ocUser(ocPermView)
	clientAdmin := &auth.AuthContext{
		PrincipalID: "prn_ui_oc_client", Scope: auth.ScopeClient, Clients: []string{owner.ClientID}, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{"platform:*:*:*"},
	}
	secretBefore := *cut.SecretRef
	for _, caller := range []*auth.AuthContext{viewer, clientAdmin} {
		for _, route := range []string{"/deactivate", "/delete", "/rotate-secret", "/rotate-secret-now", "/revoke-previous-secret", "/edit"} {
			rec := as(caller, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + route, datastar: true, form: url.Values{"clientName": {"Forged"}, "grantTypes": {"client_credentials"}}})
			assert.NotContains(t, rec.Body.String(), `id="oc-credentials"`, route)
		}
		rec := as(caller, req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
			"clientName": {"Forged"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://f.example.com/cb"}, "grantTypes": {"authorization_code"},
		}})
		assert.NotContains(t, rec.Body.String(), `id="oc-credentials"`)
	}
	still, err := oauth.FindByID(ctx, one.ID)
	require.NoError(t, err)
	require.NotNil(t, still, "a forged delete removed the client")
	assert.True(t, still.Active)
	assert.Equal(t, "UI OC one", still.ClientName)
	assert.Equal(t, secretBefore, *still.SecretRef, "a forged rotation changed the secret")
	assert.Nil(t, byName("Forged"))

	del := do(admin, req{method: http.MethodPost, target: "/ui/oauth-clients/" + one.ID + "/delete", datastar: true, form: url.Values{"drawer": {one.ID}}})
	assert.Contains(t, del, `"selOc":""`)
	gone, err := oauth.FindByID(ctx, one.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	if rec := as(admin, req{target: "/ui/oauth-clients/" + one.ID}); rec.Code != http.StatusNotFound {
		t.Errorf("a deleted client's drawer: %d", rec.Code)
	}
}

// secretIn reads the text of the credential element id from a response.
func secretIn(t *testing.T, body, id string) string {
	t.Helper()
	_, rest, ok := strings.Cut(body, `id="`+id+`" class="fc-mono-chip break-all">`)
	require.True(t, ok, "no %s in %s", id, body)
	value, _, _ := strings.Cut(rest, "</code>")
	require.NotEmpty(t, value)
	return value
}
