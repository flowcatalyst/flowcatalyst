//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// The identity provider and email domain pages end to end against Postgres:
// the handlers run the real use cases (IdpOps and EmailDomainOps over the unit
// of work, with the platform's encryption), so the rows, the mappings the
// provider's domains create, the domain events and the audit records are the
// API's. Domains and codes are this test's own, so it shares the database.
func TestIdentityProviderAndEmailDomainPagesAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	clients := client.NewRepository(pool)
	idps := identityprovider.NewRepository(pool)
	edms := emaildomainmapping.NewRepository(pool)
	principals := principal.NewRepository(pool)

	ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "Idp UI Acme", Identifier: "idp-ui-acme"}, testpg.TestEC())
	require.NoError(t, err)
	acmeID := ev.ClientID

	idpOps := IdpOps{Repo: idps, Mappings: edms, Principals: principals, Enc: enc, UoW: uow}
	edmOps := EmailDomainOps{Repo: edms, IDPs: idps, Principals: principals, UoW: uow}
	// The seeded password provider the released domains fall back to.
	internal, err := idps.FindByCode(ctx, "internal")
	require.NoError(t, err)
	if internal == nil {
		_, err := idpOps.Create(ctx, idpops.CreateCommand{Code: "internal", Name: "Internal", Type: "INTERNAL"})
		require.NoError(t, err)
		internal, err = idps.FindByCode(ctx, "internal")
		require.NoError(t, err)
	}

	pages := func(ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
		return serveMounted(t, ac, q,
			&IdentityProviders{Store: idps, Ops: idpOps, Clients: clients, ClientNames: clients, Shell: shellForTest()},
			&EmailDomains{Store: edms, Ops: edmOps, IdPs: idps, Clients: clients, ClientNames: clients, Shell: shellForTest()})
	}
	do := func(ac *auth.AuthContext, q req) string {
		t.Helper()
		rec := pages(ac, q)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		return rec.Body.String()
	}
	admin := anchorAdmin()
	const secret = "pg-typed-client-secret-1"

	// Create an OIDC provider with two domains linked to a client, through the
	// dialog: the provider, its mappings and their client link are written in
	// one transaction and audited; the secret is stored encrypted.
	body := do(admin, postForm("/ui/identity-providers/new",
		"code", "idp-ui-entra", "name", "Idp UI Entra", "type", "OIDC", "multiTenant", "true",
		"issuerUrl", "https://login.idp-ui.test/organizations/v2.0", "clientId", "idp-ui-app", "secretKind", "encrypt", "secretValue", secret,
		"domains", "one.idp-ui.test\ntwo.idp-ui.test", "scope", "CLIENT", "client", "idp-ui-acme", "sync", "true"))
	created, err := idps.FindByCode(ctx, "idp-ui-entra")
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "Idp UI Entra", created.Name)
	assert.True(t, created.OIDCMultiTenant)
	assert.True(t, created.SyncRolesFromIDP)
	require.NotNil(t, created.OIDCClientSecretRef)
	assert.True(t, strings.HasPrefix(*created.OIDCClientSecretRef, "encrypted:"))
	assert.NotContains(t, *created.OIDCClientSecretRef, secret, "the secret is stored encrypted")
	plain, err := enc.Decrypt(strings.TrimPrefix(*created.OIDCClientSecretRef, "encrypted:"))
	require.NoError(t, err)
	assert.Equal(t, secret, plain)
	assert.ElementsMatch(t, []string{"one.idp-ui.test", "two.idp-ui.test"}, created.AllowedEmailDomains)
	assert.Equal(t, 1, auditCount(t, created.ID), "create is audited like the API")
	one, err := edms.FindByEmailDomain(ctx, "one.idp-ui.test")
	require.NoError(t, err)
	require.NotNil(t, one)
	assert.Equal(t, emaildomainmapping.ScopeClient, one.ScopeType)
	require.NotNil(t, one.PrimaryClientID)
	assert.Equal(t, acmeID, string(*one.PrimaryClientID))
	assert.Equal(t, created.ID, one.IdentityProviderID)
	// The secret, typed or stored, is in no answer.
	list := do(admin, req{target: "/ui/identity-providers"})
	assert.Contains(t, list, "Idp UI Entra")
	for _, target := range []string{"/ui/identity-providers", "/ui/identity-providers/" + created.ID, "/ui/identity-providers/" + created.ID + "/edit", "/ui/email-domains"} {
		answer := do(admin, req{target: target})
		assert.NotContains(t, answer, secret, target)
		assert.NotContains(t, answer, *created.OIDCClientSecretRef, target)
	}
	assert.NotContains(t, body, secret)
	assert.Contains(t, do(admin, req{target: "/ui/identity-providers/" + created.ID, datastar: true}), "Configured")

	// Adding a domain without a scope is the use case's refusal, shown next to
	// the scope; nothing changes.
	refused := do(admin, postForm("/ui/identity-providers/"+created.ID+"/edit",
		"name", "Idp UI Entra", "multiTenant", "true", "issuerUrl", "https://login.idp-ui.test/organizations/v2.0", "clientId", "idp-ui-app", "secretKind", "encrypt",
		"domains", "one.idp-ui.test\ntwo.idp-ui.test\nthree.idp-ui.test"))
	assert.Contains(t, refused, `id="idp-form-scope-err"`)
	assert.Contains(t, refused, "three.idp-ui.test")
	nope, err := edms.FindByEmailDomain(ctx, "three.idp-ui.test")
	require.NoError(t, err)
	assert.Nil(t, nope)

	// Releasing a domain waits for the confirmation; confirmed, the mapping
	// falls back to the internal provider and keeps its client link.
	edit := func(extra ...string) string {
		kv := append([]string{
			"name", "Idp UI Entra", "multiTenant", "true", "issuerUrl", "https://login.idp-ui.test/organizations/v2.0", "clientId", "idp-ui-app", "secretKind", "encrypt",
			"domains", "one.idp-ui.test",
		}, extra...)
		return do(admin, postForm("/ui/identity-providers/"+created.ID+"/edit", kv...))
	}
	unconfirmed := edit()
	assert.Contains(t, unconfirmed, "Remove Email Domains")
	still, err := edms.FindByEmailDomain(ctx, "two.idp-ui.test")
	require.NoError(t, err)
	assert.Equal(t, created.ID, still.IdentityProviderID, "nothing moved before the confirmation")
	edit("confirmRelease", "yes")
	released, err := edms.FindByEmailDomain(ctx, "two.idp-ui.test")
	require.NoError(t, err)
	assert.Equal(t, internal.ID, released.IdentityProviderID)
	require.NotNil(t, released.PrimaryClientID, "the mapping keeps its client")
	// Editing without a secret keeps the stored one.
	after, err := idps.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, created.OIDCClientSecretRef, after.OIDCClientSecretRef)

	// Delete is the use case's: refused while a domain still routes here.
	blocked := do(admin, postForm("/ui/identity-providers/"+created.ID+"/delete"))
	assert.Contains(t, blocked, "still routes email domains")
	kept, err := idps.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.NotNil(t, kept)
	do(admin, postForm("/ui/identity-providers/"+internal.ID+"/delete"))
	stillInternal, err := idps.FindByID(ctx, internal.ID)
	require.NoError(t, err)
	assert.NotNil(t, stillInternal, "the internal provider is never deleted")

	// Email domains: a mapping on the internal provider with a two-factor
	// policy, the edit, the confirmed move to the OIDC provider, and delete.
	do(admin, postForm("/ui/email-domains/new",
		"emailDomain", "mapped.idp-ui.test", "identityProviderId", internal.ID, "scopeType", "CLIENT", "client", "idp-ui-acme",
		"require2fa", "true", "methods", "TOTP", "methods", "EMAIL_PIN", "remember", "true", "days", "9"))
	m, err := edms.FindByEmailDomain(ctx, "mapped.idp-ui.test")
	require.NoError(t, err)
	require.NotNil(t, m)
	assert.True(t, m.Require2FA)
	assert.ElementsMatch(t, []string{"TOTP", "EMAIL_PIN"}, m.Allowed2FAMethods)
	assert.Equal(t, 9, m.RememberDeviceDays)
	assert.Equal(t, 1, auditCount(t, m.ID))
	// A second mapping for the same domain is the use case's conflict.
	dup := do(admin, postForm("/ui/email-domains/new", "emailDomain", "mapped.idp-ui.test", "identityProviderId", internal.ID, "scopeType", "ANCHOR"))
	assert.Contains(t, dup, `id="edm-form-email-domain-err"`)
	assert.Contains(t, dup, "already mapped")

	do(admin, postForm("/ui/email-domains/"+m.ID+"/edit", "identityProviderId", internal.ID, "client", "idp-ui-acme", "methods", "EMAIL_PIN", "require2fa", "true"))
	updated, err := edms.FindByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, []string{"EMAIL_PIN"}, updated.Allowed2FAMethods)
	require.NotNil(t, updated.PrimaryClientID, "an edit that names the client keeps it")

	unmoved := do(admin, postForm("/ui/email-domains/"+m.ID+"/edit", "identityProviderId", created.ID, "client", "idp-ui-acme", "tenant", "tenant-1"))
	assert.Contains(t, unmoved, "Change Identity Provider")
	same, err := edms.FindByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, internal.ID, same.IdentityProviderID, "nothing moved before the confirmation")
	do(admin, postForm("/ui/email-domains/"+m.ID+"/edit", "identityProviderId", created.ID, "client", "idp-ui-acme", "tenant", "tenant-1", "confirmMove", "yes"))
	moved, err := edms.FindByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Equal(t, created.ID, moved.IdentityProviderID)
	assert.Equal(t, "tenant-1", deref(moved.RequiredOIDCTenantID))

	// Forged posts by a viewer and by a non-anchor holder of every permission
	// write nothing.
	viewer := connUser(edmPermView, idpPermView)
	nonAnchor := &auth.AuthContext{
		PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{acmeID}, Roles: []string{"r"}, PrincipalType: auth.PrincipalTypeUser,
		Permissions: []string{edmPermView, edmPermCreate, edmPermUpdate, edmPermDelete, idpPermView, idpPermCreate, idpPermUpdate, idpPermDelete},
	}
	before := audits(t)
	for _, ac := range []*auth.AuthContext{viewer, nonAnchor} {
		for _, q := range []req{
			postForm("/ui/email-domains/" + m.ID + "/delete"),
			postForm("/ui/email-domains/"+m.ID+"/edit", "identityProviderId", internal.ID, "confirmMove", "yes"),
			postForm("/ui/email-domains/new", "emailDomain", "forged.idp-ui.test", "identityProviderId", internal.ID, "scopeType", "ANCHOR"),
			postForm("/ui/identity-providers/" + created.ID + "/delete"),
			postForm("/ui/identity-providers/new", "code", "idp-ui-forged", "name", "Forged", "type", "INTERNAL"),
			postForm("/ui/identity-providers/"+created.ID+"/edit", "name", "Hacked", "issuerUrl", "https://evil.test", "clientId", "x", "domains", ""),
		} {
			rec := pages(ac, q)
			assert.Equal(t, http.StatusOK, rec.Code, q.target) // a Datastar refusal is a flash on a 200
			refusal := strings.ToLower(rec.Body.String())
			assert.True(t, strings.Contains(refusal, "permission required") || strings.Contains(refusal, "anchor scope required"), q.target)
		}
	}
	assert.Equal(t, before, audits(t), "refused posts wrote nothing")
	renamed, err := idps.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Equal(t, "Idp UI Entra", renamed.Name)

	// Delete the mapping, then (nothing routes there any more) the provider.
	do(admin, postForm("/ui/email-domains/"+m.ID+"/delete"))
	gone, err := edms.FindByID(ctx, m.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	do(admin, postForm("/ui/identity-providers/"+created.ID+"/edit",
		"name", "Idp UI Entra", "multiTenant", "true", "issuerUrl", "https://login.idp-ui.test/organizations/v2.0", "clientId", "idp-ui-app", "secretKind", "encrypt",
		"domains", "", "confirmRelease", "yes"))
	do(admin, postForm("/ui/identity-providers/"+created.ID+"/delete"))
	deleted, err := idps.FindByID(ctx, created.ID)
	require.NoError(t, err)
	assert.Nil(t, deleted)
}
