//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Credential publishing in the console, end to end against Postgres: the
// pages run the real operations and read the real publication tables (the
// worker is not run, so what is queued stays PENDING_PUT).
func TestCredentialPublishingConsoleAgainstPostgres(t *testing.T) {
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	cfg := credentialpublish.Config{
		Store: credentialpublish.StoreAWSParameterStore, Prefix: "/fc/ui/",
		ServiceAccountPath: credentialpublish.DefaultServiceAccountPath, OAuthClientPath: credentialpublish.DefaultOAuthClientPath,
	}
	saRepo := serviceaccount.NewRepository(pool)
	principals := principal.NewRepository(pool)
	oauth := platformauth.NewRepository(pool).OAuthClients
	clients := client.NewRepository(pool)
	apps := application.NewRepository(pool)
	pubs := credentialpublish.NewRepository(pool)
	oauthOps := OAuthClientOps{Repo: oauth, UoW: uow, CredentialPublish: cfg}
	publish := CredentialPublishOps{
		Cfg: cfg, Repo: pubs, Accounts: saRepo, Principals: principals, OAuthClients: oauth, Apps: apps, OAuth: oauthOps, UoW: uow,
	}
	saPage := &ServiceAccounts{
		Accounts: saRepo, Principals: principals, OAuthClients: oauth, Roles: role.NewRepository(pool), Clients: clients, Apps: apps,
		Connections: connection.NewRepository(pool), Publish: publish, Shell: shellForTest(),
		Ops: ServiceAccountOps{
			Repo: saRepo, Principals: principals, OAuthClients: oauth, Clients: clients, Grants: principal.NewClientAccessGrantRepo(pool),
			Roles: role.NewRepository(pool), Applications: apps, UoW: uow, Audit: audit.NewRepository(pool), CredentialPublish: cfg,
			FlattenPermissions: func(context.Context, []string) ([]string, error) { return nil, nil },
		},
	}
	ocPage := &OAuthClients{
		Clients: oauth, Ops: oauthOps, Apps: apps, ClientNames: clients, Accounts: saRepo, Publish: publish, Shell: shellForTest(),
	}
	admin := saAdmin()
	admin.PrincipalID = "prn_ui_pub_admin"
	do := func(q req, pages ...mounter) string {
		t.Helper()
		rec := serveMounted(t, admin, q, pages...)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	status := func(o credentialpublish.Owner) (*credentialpublish.Publication, map[string]credentialpublish.Item) {
		t.Helper()
		pub, items, err := pubs.Status(ctx, o)
		require.NoError(t, err)
		by := map[string]credentialpublish.Item{}
		for _, it := range items {
			by[it.Credential] = it
		}
		return pub, by
	}
	saOwner := func(code string) (credentialpublish.Owner, string) {
		t.Helper()
		sa, err := saRepo.FindByCode(ctx, code)
		require.NoError(t, err)
		require.NotNil(t, sa, code)
		return credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: sa.ID}, sa.ID
	}

	// The dialog offers the control, unticked: the deployment default is off.
	dlg := do(req{target: "/ui/service-accounts/new", datastar: true}, saPage)
	require.Contains(t, dlg, "Publish credentials to Parameter Store")
	assert.False(t, ticked(boxTag(t, dlg)))

	// Created with the box ticked: everything is queued, and the reveal says where.
	body := do(req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, multipart: true, form: url.Values{
		"name": {"UI pub one"}, "code": {"ui-pub-one"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"},
	}}, saPage)
	require.Contains(t, body, "Service Account Created", body)
	owner, id := saOwner("ui-pub-one")
	base := "/fc/ui/service-accounts/ui-pub-one/"
	pub, items := status(owner)
	require.NotNil(t, pub)
	assert.Equal(t, base, pub.BasePath)
	require.Len(t, items, 4)
	for _, c := range []string{credentialpublish.CredClientID, credentialpublish.CredClientSecret, credentialpublish.CredAuthToken, credentialpublish.CredSigningSecret} {
		assert.Equal(t, credentialpublish.StatePendingPut, items[c].State, c)
		assert.Contains(t, body, `Also published to <code class="fc-mono-chip break-all">`+base+c+`</code>`, c)
	}
	// The drawer, in the same answer, shows the section.
	assert.Contains(t, body, "Publishing…")

	// A custom path outside the prefix is refused on the field, and nothing is made.
	refused := do(req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"UI pub bad"}, "code": {"ui-pub-bad"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"}, "publishBasePath": {"/elsewhere/"},
	}}, saPage)
	assert.Contains(t, refused, `id="sa-form-publish-path-err"`)
	assert.Contains(t, refused, "must lie under /fc/ui/")
	gone, err := saRepo.FindByCode(ctx, "ui-pub-bad")
	require.NoError(t, err)
	assert.Nil(t, gone)
	// A custom path inside it is used.
	do(req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"UI pub custom"}, "code": {"ui-pub-custom"}, "scope": {"ANCHOR"}, "publishCredentials": {"true"}, "publishBasePath": {"/fc/ui/custom/path/"},
	}}, saPage)
	customOwner, _ := saOwner("ui-pub-custom")
	customPub, _ := status(customOwner)
	require.NotNil(t, customPub)
	assert.Equal(t, "/fc/ui/custom/path/", customPub.BasePath)

	// Created without the box: nothing queued; the drawer says so. Enable from
	// the drawer queues all but the client secret, which is not publishable.
	do(req{method: http.MethodPost, target: "/ui/service-accounts/new", datastar: true, form: url.Values{
		"name": {"UI pub two"}, "code": {"ui-pub-two"}, "scope": {"ANCHOR"},
	}}, saPage)
	two, twoID := saOwner("ui-pub-two")
	pub, items = status(two)
	assert.Nil(t, pub)
	assert.Empty(t, items)
	drawer := do(req{target: "/ui/service-accounts/" + twoID, datastar: true}, saPage)
	assert.Contains(t, drawer, "Not published")
	assert.Contains(t, drawer, "/ui/service-accounts/"+twoID+"/publish")
	dlg = do(req{target: "/ui/service-accounts/" + twoID + "/publish", datastar: true}, saPage)
	assert.Contains(t, dlg, "/fc/ui/service-accounts/ui-pub-two/")
	// A path outside the prefix is refused in the dialog.
	bad := do(req{method: http.MethodPost, target: "/ui/service-accounts/" + twoID + "/publish", datastar: true, form: url.Values{"drawer": {twoID}, "publishBasePath": {"/elsewhere/"}}}, saPage)
	assert.Contains(t, bad, `id="pub-form-path-err"`)
	pub, _ = status(two)
	assert.Nil(t, pub)
	enabled := do(req{method: http.MethodPost, target: "/ui/service-accounts/" + twoID + "/publish", datastar: true, form: url.Values{"drawer": {twoID}}}, saPage)
	assert.Contains(t, enabled, "Credential publishing switched on")
	pub, items = status(two)
	require.NotNil(t, pub)
	require.Len(t, items, 4)
	assert.Equal(t, credentialpublish.StateNotPublishable, items[credentialpublish.CredClientSecret].State)
	assert.Equal(t, credentialpublish.StatePendingPut, items[credentialpublish.CredAuthToken].State)
	assert.Contains(t, enabled, "Not published: rotate the client secret to publish it")

	// Rotate and publish: the new secret is queued and the reveal says where.
	rot := do(req{method: http.MethodPost, target: "/ui/service-accounts/" + twoID + "/publish/rotate", datastar: true, form: url.Values{"drawer": {twoID}}}, saPage)
	assert.Contains(t, rot, "New Client Secret")
	assert.Contains(t, rot, `Also published to <code class="fc-mono-chip break-all">/fc/ui/service-accounts/ui-pub-two/client-secret</code>`)
	_, items = status(two)
	assert.Equal(t, credentialpublish.StatePendingPut, items[credentialpublish.CredClientSecret].State)

	// The account's own OAuth client page says it is published with the account.
	p, err := principals.FindByServiceAccount(ctx, twoID)
	require.NoError(t, err)
	linked, err := oauth.FindByPrincipalID(ctx, string(p.ID))
	require.NoError(t, err)
	require.Len(t, linked, 1)
	ocBody := do(req{target: "/ui/oauth-clients/" + linked[0].ID, datastar: true}, ocPage)
	assert.Contains(t, ocBody, "Published with service account")
	assert.Contains(t, ocBody, `href="/ui/service-accounts/`+twoID+`"`)
	assert.Contains(t, ocBody, "UI pub two")
	assert.NotContains(t, ocBody, "Stop publishing")
	// A rotation of the linked client reveals the account's parameter.
	lrot := do(req{method: http.MethodPost, target: "/ui/oauth-clients/" + linked[0].ID + "/rotate-secret", datastar: true, form: url.Values{"drawer": {linked[0].ID}}}, ocPage)
	assert.Contains(t, lrot, "/fc/ui/service-accounts/ui-pub-two/client-secret")

	// Retry is accepted while nothing failed (it only moves the due time).
	do(req{method: http.MethodPost, target: "/ui/service-accounts/" + twoID + "/publish/retry", datastar: true, form: url.Values{"drawer": {twoID}}}, saPage)

	// Stop publishing: the parameters are queued for deletion, and the drawer says so.
	stopped := do(req{method: http.MethodPost, target: "/ui/service-accounts/" + twoID + "/publish/stop", datastar: true, form: url.Values{"drawer": {twoID}}}, saPage)
	assert.Contains(t, stopped, "Credential publishing switched off")
	pub, items = status(two)
	assert.Nil(t, pub)
	assert.Equal(t, credentialpublish.StatePendingDelete, items[credentialpublish.CredAuthToken].State)
	assert.Contains(t, stopped, "Removing…")
	assert.NotContains(t, stopped, "Stop publishing")
	// The account itself is unchanged.
	still, err := saRepo.FindByID(ctx, twoID)
	require.NoError(t, err)
	require.NotNil(t, still)
	assert.NotNil(t, still.WebhookCredentials.Token)

	// The drawer's stop confirmation names the parameters.
	drawer = do(req{target: "/ui/service-accounts/" + id, datastar: true}, saPage)
	assert.Contains(t, drawer, "Deletes these 4 parameters from Parameter Store. The credentials in FlowCatalyst are not changed.")
	assert.Contains(t, drawer, base+"signing-secret")

	// A stand-alone confidential OAuth client, created with the box ticked.
	created := do(req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, multipart: true, form: url.Values{
		"clientName": {"UI pub client"}, "clientType": {"CONFIDENTIAL"}, "redirectUris": {"https://pub.example.com/cb"}, "grantTypes": {"authorization_code"},
		"publishCredentials": {"true"},
	}}, ocPage)
	require.Contains(t, created, "Client Secret Generated", created)
	var ocRow, ocClientID string
	all, err := oauth.FindAll(ctx)
	require.NoError(t, err)
	for _, c := range all {
		if c.ClientName == "UI pub client" {
			ocRow, ocClientID = c.ID, c.ClientID
		}
	}
	require.NotEmpty(t, ocRow)
	ocOwner := credentialpublish.Owner{Type: credentialpublish.OwnerOAuthClient, ID: ocRow}
	pub, items = status(ocOwner)
	require.NotNil(t, pub)
	assert.Equal(t, "/fc/ui/oauth-clients/"+ocClientID+"/", pub.BasePath)
	assert.Equal(t, credentialpublish.StatePendingPut, items[credentialpublish.CredClientSecret].State)
	assert.Contains(t, created, `Also published to <code class="fc-mono-chip break-all">/fc/ui/oauth-clients/`+ocClientID+`/client-secret</code>`)
	ocDrawer := do(req{target: "/ui/oauth-clients/" + ocRow, datastar: true}, ocPage)
	assert.Contains(t, ocDrawer, "Parameter Store")
	assert.Contains(t, ocDrawer, "Stop publishing")
	do(req{method: http.MethodPost, target: "/ui/oauth-clients/" + ocRow + "/publish/stop", datastar: true, form: url.Values{"drawer": {ocRow}}}, ocPage)
	pub, _ = status(ocOwner)
	assert.Nil(t, pub)
	// A public client is offered nothing and publishes nothing.
	do(req{method: http.MethodPost, target: "/ui/oauth-clients/new", datastar: true, form: url.Values{
		"clientName": {"UI pub spa"}, "clientType": {"PUBLIC"}, "redirectUris": {"https://spa.example.com/cb"}, "grantTypes": {"authorization_code"},
		"publishCredentials": {"true"},
	}}, ocPage)

	// An application's provision flow, with the box ticked.
	appPage := &Applications{
		Apps: apps, Logins: oauth, Docs: appdocs.NewRepository(pool), Publish: publish, Shell: shellForTest(),
		Ops: ApplicationOps{
			Repo: apps, ServiceAccounts: saRepo, Principals: principals, OAuthClients: oauth, Roles: role.NewRepository(pool), UoW: uow,
			CredentialPublish: cfg,
		},
	}
	a, err := apps.FindByCode(ctx, "uipubapp")
	require.NoError(t, err)
	if a == nil {
		testpg.EnsureApplication(t, "uipubapp")
		a, err = apps.FindByCode(ctx, "uipubapp")
		require.NoError(t, err)
	}
	require.NotNil(t, a)
	form := do(req{target: "/ui/applications/" + string(a.ID), datastar: true}, appPage)
	assert.Contains(t, form, "/fc/ui/service-accounts/app.uipubapp/")
	prov := do(req{method: http.MethodPost, target: "/ui/applications/" + string(a.ID) + "/provision-service-account", datastar: true, form: url.Values{
		"drawer": {string(a.ID)}, "publishCredentials": {"true"},
	}}, appPage)
	require.Contains(t, prov, "Service Account Provisioned", prov)
	acct, err := saRepo.FindFirstByApplicationID(ctx, string(a.ID))
	require.NoError(t, err)
	require.NotNil(t, acct)
	provOwner := credentialpublish.Owner{Type: credentialpublish.OwnerServiceAccount, ID: acct.ID}
	pub, items = status(provOwner)
	require.NotNil(t, pub)
	assert.Equal(t, "/fc/ui/service-accounts/app.uipubapp/", pub.BasePath, "the ':' of the application account's code is written '.'")
	require.Len(t, items, 4)
	assert.True(t, strings.Contains(prov, `Also published to <code class="fc-mono-chip break-all">/fc/ui/service-accounts/app.uipubapp/signing-secret</code>`), prov)
}
