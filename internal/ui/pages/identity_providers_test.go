package pages

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The stored client secret the fixtures hold, and the one a test types: neither
// may appear in any answer (the typed one is only ever in the command).
const (
	idpStoredSecret = "encrypted:STORED_SECRET_BLOB_DO_NOT_RENDER"
	idpTypedSecret  = "typed-secret-value-xyz"
)

type fakeIdpStore struct {
	providers []identityprovider.IdentityProvider
}

func (f *fakeIdpStore) FindAll(context.Context) ([]identityprovider.IdentityProvider, error) {
	return append([]identityprovider.IdentityProvider(nil), f.providers...), nil
}

func (f *fakeIdpStore) FindByID(_ context.Context, id string) (*identityprovider.IdentityProvider, error) {
	for i := range f.providers {
		if f.providers[i].ID == id {
			p := f.providers[i]
			return &p, nil
		}
	}
	return nil, nil
}

type fakeIdpOps struct {
	store   *fakeIdpStore
	created []idpops.CreateCommand
	updated []idpops.UpdateCommand
	deleted []string
	err     error
}

func (o *fakeIdpOps) Create(_ context.Context, cmd idpops.CreateCommand) (idpops.CreateResult, error) {
	o.created = append(o.created, cmd)
	if o.err != nil {
		return idpops.CreateResult{}, o.err
	}
	p := idpFixture("idp_9", cmd.Code, cmd.Name, identityprovider.Type(cmd.Type))
	p.AllowedEmailDomains = cmd.AllowedEmailDomains
	o.store.providers = append(o.store.providers, p)
	return idpops.CreateResult{IdentityProviderID: p.ID, Code: p.Code}, nil
}

func (o *fakeIdpOps) Update(_ context.Context, cmd idpops.UpdateCommand) (idpops.UpdateResult, error) {
	o.updated = append(o.updated, cmd)
	return idpops.UpdateResult{IdentityProviderID: cmd.ID}, o.err
}

func (o *fakeIdpOps) Delete(_ context.Context, id string) error {
	o.deleted = append(o.deleted, id)
	return o.err
}

func idpFixture(id, code, name string, typ identityprovider.Type) identityprovider.IdentityProvider {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	p := *identityprovider.New(code, name, typ)
	p.ID, p.CreatedAt, p.UpdatedAt = id, day, day
	return p
}

type fakeIdpRoles []role.Role

func (f fakeIdpRoles) FindAll(context.Context) ([]role.Role, error) { return []role.Role(f), nil }

type idpHarness struct {
	store *fakeIdpStore
	ops   *fakeIdpOps
}

func newIdpHarness() *idpHarness {
	issuer, clientID, pattern := "https://login.example.com/<tenant>/v2.0", "entra-app-id", "https://login.example.com/{tenantId}/v2.0"
	secret := idpStoredSecret
	entra := idpFixture("idp_2", "entra", "Entra <ID>", identityprovider.TypeOIDC)
	entra.OIDCIssuerURL, entra.OIDCClientID, entra.OIDCClientSecretRef, entra.OIDCIssuerPattern = &issuer, &clientID, &secret, &pattern
	entra.OIDCMultiTenant, entra.SyncRolesFromIDP = true, true
	entra.AllowedEmailDomains = []string{"acme.com", "globex.com", "initech.com", "hooli.com"}
	entra.AllowedRoleIDs = []string{"rol_1"}
	internal := idpFixture("idp_1", "internal", "Internal", identityprovider.TypeInternal)
	plain := idpFixture("idp_3", "keycloak", "Keycloak", identityprovider.TypeOIDC)
	plain.OIDCIssuerURL = &issuer
	plain.OIDCClientID = &clientID
	store := &fakeIdpStore{providers: []identityprovider.IdentityProvider{internal, entra, plain}}
	return &idpHarness{store: store, ops: &fakeIdpOps{store: store}}
}

func (h *idpHarness) page() *IdentityProviders {
	return &IdentityProviders{
		Store: h.store, Ops: h.ops,
		Roles: fakeIdpRoles{
			{ID: "rol_1", Name: "orders:reader", DisplayName: "Orders reader"},
			{ID: "rol_2", Name: "platform:ops", DisplayName: "Operations"},
		},
		Clients: someClients(), ClientNames: &fakeClientStore{clients: []client.Client{cli("clt_acme", "acme", "Acme Corp", client.StatusActive)}},
		Shell: shellForTest(),
	}
}

func (h *idpHarness) rec(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

func (h *idpHarness) do(t *testing.T, ac *auth.AuthContext, q req) string {
	t.Helper()
	return h.rec(t, ac, q).Body.String()
}

const (
	idpPermView   = "platform:iam:idp:view"
	idpPermCreate = "platform:iam:idp:create"
	idpPermUpdate = "platform:iam:idp:update"
	idpPermDelete = "platform:iam:idp:delete"
)

func TestIdentityProvidersList(t *testing.T) {
	h := newIdpHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers"})
	mustContain(t, body, `<title>Identity Providers · FlowCatalyst</title>`, "Configure external identity providers for federated authentication.",
		"Add Identity Provider", `href="/ui/identity-providers/new"`, "Entra &lt;ID&gt;", `<code class="font-mono text-[13px]">entra</code>`,
		">OIDC<", ">INTERNAL<", "https://login.example.com/&lt;tenant&gt;/v2.0", "acme.com", "+1 more", "All domains",
		"Showing 1 to 3 of 3 identity providers", `<dialog id="idp-delete"`, `<aside id="idp-drawer"`, `data-signals="{selIdp: &#39;&#39;}"`)
	mustContain(t, body, `href="/ui/identity-providers" class="fc-nav-item" title="Identity Providers" aria-current="page"`)
	// The fourth domain is behind "+1 more", not listed.
	mustNotContain(t, body, "hooli.com", idpStoredSecret)

	filtered := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers?q=KEYC", datastar: true})
	mustContain(t, filtered, "Keycloak", "replaceState", "q=KEYC")
	mustNotContain(t, filtered, "Entra &lt;ID&gt;")
	sorted := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers?sort=code&dir=desc", datastar: true})
	if strings.Index(sorted, "keycloak") > strings.Index(sorted, "entra") {
		t.Error("sort=code&dir=desc: keycloak must come before entra")
	}
}

func TestIdentityProvidersDrawer(t *testing.T) {
	h := newIdpHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2", datastar: true})
	mustContain(t, body, "Entra &lt;ID&gt;", "OIDC Configuration", "https://login.example.com/{tenantId}/v2.0", "entra-app-id", "Configured",
		"Orders reader", "acme.com", "hooli.com", `data-action="/ui/identity-providers/idp_2/delete"`, `href="/ui/identity-providers/idp_2/edit"`, "selIdp")
	// The secret is write-only: only whether one is configured.
	mustNotContain(t, body, idpStoredSecret, "STORED_SECRET")

	plain := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_3", datastar: true})
	mustContain(t, plain, "Not configured", "No domains routed to this provider")
	internal := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_1", datastar: true})
	mustNotContain(t, internal, "OIDC Configuration", `data-action="/ui/identity-providers/idp_1/delete"`)
	mustContain(t, internal, undeletableInternal)

	// The same drawer on a full page.
	page := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2"})
	mustContain(t, page, `data-signals="{selIdp: &#39;idp_2&#39;}"`, "OIDC Configuration")
	mustNotContain(t, page, idpStoredSecret)
	rec := h.rec(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_404", datastar: true})
	mustNotContain(t, rec.Body.String(), "OIDC Configuration")
	assert.Equal(t, http.StatusNotFound, h.rec(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_404"}).Code)
}

// TestIdentityProvidersPermissionMatrix pins the API's gates: every control is
// hidden without its permission and every route refuses a forged post before
// the use case runs.
func TestIdentityProvidersPermissionMatrix(t *testing.T) {
	forged := []struct {
		name, target string
		form         url.Values
	}{
		{"create", "/ui/identity-providers/new", url.Values{"code": {"x"}, "name": {"X"}, "type": {"INTERNAL"}}},
		{"edit", "/ui/identity-providers/idp_2/edit", url.Values{"name": {"Hacked"}, "issuerUrl": {"https://evil.test"}, "clientId": {"x"}}},
		{"delete", "/ui/identity-providers/idp_2/delete", url.Values{}},
	}
	callers := []struct {
		name string
		ac   *auth.AuthContext
	}{
		{"no permission", connUser()},
		{"viewer", connUser(idpPermView)},
		// Holding every permission is not enough without anchor scope.
		{"non-anchor with every permission", &auth.AuthContext{
			PrincipalID: "prn_c", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"r"},
			Permissions: []string{idpPermView, idpPermCreate, idpPermUpdate, idpPermDelete}, PrincipalType: auth.PrincipalTypeUser,
		}},
	}
	for _, c := range callers {
		t.Run(c.name, func(t *testing.T) {
			h := newIdpHarness()
			for _, f := range forged {
				body := h.do(t, c.ac, req{method: http.MethodPost, target: f.target, datastar: true, form: f.form})
				mustNotContain(t, body, idpStoredSecret)
				if len(h.ops.created)+len(h.ops.updated)+len(h.ops.deleted) != 0 {
					t.Fatalf("%s: a forged post by the %s reached the use case: %+v", f.name, c.name, h.ops)
				}
			}
			// The forms are refused too.
			for _, target := range []string{"/ui/identity-providers/new", "/ui/identity-providers/idp_2/edit"} {
				if code := h.rec(t, c.ac, req{target: target, datastar: true}).Code; code != http.StatusForbidden && code != http.StatusOK {
					t.Errorf("GET %s: status %d", target, code)
				}
				mustNotContain(t, h.do(t, c.ac, req{target: target, datastar: true}), `id="idp-form"`)
			}
			rec := h.rec(t, c.ac, req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/delete", form: url.Values{}})
			if rec.Code < 400 || len(h.ops.deleted) != 0 {
				t.Errorf("native forged post: status %d, deleted %v", rec.Code, h.ops.deleted)
			}
		})
	}

	h := newIdpHarness()
	// A viewer sees the list and the drawer, with no control.
	viewer := h.do(t, connUser(idpPermView), req{target: "/ui/identity-providers"})
	mustContain(t, viewer, "Entra &lt;ID&gt;")
	mustNotContain(t, viewer, "Add Identity Provider", `aria-label="Edit `, `<dialog id="idp-delete"`, "Delete…")
	drawer := h.do(t, connUser(idpPermView), req{target: "/ui/identity-providers/idp_2", datastar: true})
	mustNotContain(t, drawer, `href="/ui/identity-providers/idp_2/edit"`, "/delete", "Delete Identity Provider")
	// Without the view permission there is no page at all.
	assert.Equal(t, http.StatusForbidden, h.rec(t, connUser(), req{target: "/ui/identity-providers"}).Code)
	assert.Equal(t, http.StatusForbidden, h.rec(t, connUser(), req{target: "/ui/identity-providers/idp_2"}).Code)
	// Update alone offers edit and not delete (and the reverse).
	updater := h.do(t, connUser(idpPermView, idpPermUpdate), req{target: "/ui/identity-providers"})
	mustContain(t, updater, `aria-label="Edit `)
	mustNotContain(t, updater, "Delete…", `<dialog id="idp-delete"`, "Add Identity Provider")
	deleter := h.do(t, connUser(idpPermView, idpPermDelete), req{target: "/ui/identity-providers"})
	mustContain(t, deleter, "Delete…", `<dialog id="idp-delete"`)
	mustNotContain(t, deleter, `aria-label="Edit `)
	// Editing needs the read as well as the update (the SPA loads the provider).
	assert.Equal(t, http.StatusForbidden, h.rec(t, connUser(idpPermUpdate), req{target: "/ui/identity-providers/idp_2/edit"}).Code)
	// Create alone opens the create dialog.
	assert.Equal(t, http.StatusOK, h.rec(t, connUser(idpPermCreate), req{target: "/ui/identity-providers/new", datastar: true}).Code)
	mustNotContain(t, h.do(t, connUser(idpPermView), req{target: "/ui/identity-providers/new", datastar: true}), `id="idp-form"`)
}

func TestIdentityProvidersCreateForm(t *testing.T) {
	h := newIdpHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/new", datastar: true})
	mustContain(t, body, `id="idp-form"`, "Create Identity Provider", `name="code"`, `name="type"`, `name="secretKind"`, `name="secretValue"`, `type="password"`,
		"Local Encrypted", "AWS Secrets Manager", "HashiCorp Vault", `name="domains"`, `name="scope"`, `name="roles"`, "Orders reader (orders:reader)",
		`idpFormType: &#39;OIDC&#39;`, `action="/ui/identity-providers/new"`)
	// A native navigation renders the list with the dialog open.
	page := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/new"})
	mustContain(t, page, `id="idp-form"`, "Entra &lt;ID&gt;")
}

func TestIdentityProvidersCreate(t *testing.T) {
	h := newIdpHarness()
	form := url.Values{
		"code": {"azure-ad"}, "name": {"Azure AD"}, "type": {"OIDC"}, "multiTenant": {"true"},
		"issuerUrl": {" https://login.microsoftonline.com/organizations/v2.0 "}, "issuerPattern": {"https://login.microsoftonline.com/{tenantId}/v2.0"},
		"clientId": {"app-123"}, "secretKind": {"encrypt"}, "secretValue": {idpTypedSecret}, "secretRef": {"ignored"},
		"domains": {"Acme.com, globex.com\nacme.com"}, "scope": {"CLIENT"}, "client": {"acme"},
		"sync": {"true"}, "roles": {"rol_2"},
	}
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: form})
	require.Len(t, h.ops.created, 1)
	cmd := h.ops.created[0]
	assert.Equal(t, "azure-ad", cmd.Code)
	assert.Equal(t, "OIDC", cmd.Type)
	assert.Equal(t, []string{"acme.com", "globex.com"}, cmd.AllowedEmailDomains)
	require.NotNil(t, cmd.MappingScope)
	assert.Equal(t, "CLIENT", *cmd.MappingScope)
	require.NotNil(t, cmd.PrimaryClientID)
	assert.Equal(t, "clt_acme", *cmd.PrimaryClientID, "the identifier is resolved to the client's id")
	assert.True(t, cmd.SyncRolesFromIDP)
	assert.Equal(t, []string{"rol_2"}, cmd.AllowedRoleIDs)
	assert.Equal(t, "https://login.microsoftonline.com/organizations/v2.0", deref(cmd.OIDCIssuerURL))
	assert.Equal(t, "app-123", deref(cmd.OIDCClientID))
	require.NotNil(t, cmd.OIDCMultiTenant)
	assert.True(t, *cmd.OIDCMultiTenant)
	// The SPA's SecretRefInput: the local kind sends "encrypt:" and the secret.
	assert.Equal(t, "encrypt:"+idpTypedSecret, deref(cmd.OIDCClientSecretRef))
	// The answer: the dialog closes, the new provider's drawer opens, a flash.
	mustContain(t, body, "Identity provider created successfully", "selIdp", "Azure AD")
	mustNotContain(t, body, idpTypedSecret, idpStoredSecret)

	// An external reference carries its scheme and no encryption directive.
	h2 := newIdpHarness()
	h2.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: url.Values{
		"code": {"vault-idp"}, "name": {"Vault"}, "type": {"OIDC"}, "issuerUrl": {"https://i.example.com"}, "clientId": {"c"},
		"secretKind": {"vault"}, "secretRef": {"kv/idp#secret"}, "secretValue": {idpTypedSecret},
	}})
	require.Len(t, h2.ops.created, 1)
	assert.Equal(t, "vault://kv/idp#secret", deref(h2.ops.created[0].OIDCClientSecretRef))
	assert.Nil(t, h2.ops.created[0].MappingScope, "no domains, no scope")
	assert.Nil(t, h2.ops.created[0].AllowedEmailDomains)

	// No secret entered: none is sent. An internal provider takes no OIDC part.
	h3 := newIdpHarness()
	h3.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: url.Values{
		"code": {"local-2"}, "name": {"Local 2"}, "type": {"INTERNAL"}, "issuerUrl": {"https://ignored.example.com"}, "secretValue": {idpTypedSecret},
		"domains": {"initech.com"}, "scope": {"ANCHOR"}, "client": {"acme"},
	}})
	require.Len(t, h3.ops.created, 1)
	c3 := h3.ops.created[0]
	assert.Nil(t, c3.OIDCIssuerURL)
	assert.Nil(t, c3.OIDCClientSecretRef)
	assert.Equal(t, "ANCHOR", deref(c3.MappingScope))
	assert.Nil(t, c3.PrimaryClientID, "an anchor scope links no client")
}

func TestIdentityProvidersCreateRefusals(t *testing.T) {
	post := func(h *idpHarness, f url.Values) string {
		return h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: f})
	}
	base := func() url.Values {
		return url.Values{"code": {"okta"}, "name": {"Okta"}, "type": {"OIDC"}, "issuerUrl": {"https://i.example.com"}, "clientId": {"c"}, "secretKind": {"encrypt"}, "secretValue": {idpTypedSecret}}
	}

	// The SPA's validators run before the use case: the code, a domain.
	h := newIdpHarness()
	f := base()
	f.Set("code", "Bad Code")
	body := post(h, f)
	mustContain(t, body, `id="idp-form-code-err"`, idpCodeHelp)
	mustNotContain(t, body, idpTypedSecret)
	f = base()
	f.Set("domains", "good.com not a domain")
	f.Set("scope", "ANCHOR")
	mustContain(t, post(h, f), `id="idp-form-domains-err"`, "&#34;not&#34; is not a valid domain name.")
	// A client scope without a client, and a client that is not one.
	f = base()
	f.Set("domains", "good.com")
	f.Set("scope", "CLIENT")
	h.ops.err = usecase.Validation("PRIMARY_CLIENT_REQUIRED", "primaryClientId is required when mappingScope is CLIENT")
	mustContain(t, post(h, f), `id="idp-form-client-err"`, "PrimaryClientId is required")
	f.Set("client", "nobody")
	mustContain(t, post(h, f), noSuchClient)
	assert.Len(t, h.ops.created, 1, "the unknown client never reached the use case; only the missing one did")

	// A domain without a scope: the use case's code is shown next to the scope.
	h = newIdpHarness()
	h.ops.err = usecase.Validation("MAPPING_SCOPE_REQUIRED", "mappingScope is required: domain 'good.com' has no mapping yet; choose ANCHOR or CLIENT")
	f = base()
	f.Set("domains", "good.com")
	body = post(h, f)
	mustContain(t, body, `id="idp-form-scope-err"`, "choose ANCHOR or CLIENT")
	mustNotContain(t, body, idpTypedSecret)
	// A code that exists, and a secret the platform cannot store.
	h.ops.err = usecase.Conflict("CODE_EXISTS", "Identity provider with code 'okta' already exists")
	mustContain(t, post(h, base()), `id="idp-form-code-err"`, "already exists")
	h.ops.err = usecase.Validation("ENCRYPTION_NOT_CONFIGURED", "cannot store OIDC client secret: FLOWCATALYST_APP_KEY is not configured")
	body = post(h, base())
	mustContain(t, body, `id="idp-form-secret-err"`, "FLOWCATALYST_APP_KEY")
	mustNotContain(t, body, idpTypedSecret)
	// A role that is not in the list is refused before the use case.
	h = newIdpHarness()
	f = base()
	f.Set("roles", "rol_nope")
	mustContain(t, post(h, f), `id="idp-form-roles-err"`, idpUnknownRole)
	assert.Empty(t, h.ops.created)

	// A native post that fails renders the page with the form and the status;
	// the typed secret is not in it.
	rec := h.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", form: func() url.Values { f := base(); f.Set("code", "Bad"); return f }()})
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	mustContain(t, rec.Body.String(), `id="idp-form-code-err"`, `id="idp-form"`)
	mustNotContain(t, rec.Body.String(), idpTypedSecret)
}

func TestIdentityProvidersEditForm(t *testing.T) {
	h := newIdpHarness()
	// A native navigation, where the textarea's lines are not split by the stream.
	body := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2/edit"})
	mustContain(t, body, "Edit Identity Provider", `value="Entra &lt;ID&gt;"`, `value="entra-app-id"`, "acme.com\nglobex.com\ninitech.com\nhooli.com",
		"Current secret is configured. Enter a new value to replace it, or leave blank to keep it.", `action="/ui/identity-providers/idp_2/edit"`,
		`name="drawer"`)
	// Never the stored secret, and no scope is inferred.
	mustNotContain(t, body, idpStoredSecret, "STORED_SECRET", `name="code"`)
	assert.NotContains(t, body, `<option value="ANCHOR" selected`)
	assert.NotContains(t, body, `<option value="CLIENT" selected`)
	// An internal provider has no OIDC settings to fill.
	internal := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_1/edit", datastar: true})
	mustContain(t, internal, `idpFormType: &#39;INTERNAL&#39;`)
}

func TestIdentityProvidersUpdate(t *testing.T) {
	h := newIdpHarness()
	form := func() url.Values {
		return url.Values{
			"name": {"Entra renamed"}, "multiTenant": {"true"}, "issuerUrl": {"https://login.example.com/<tenant>/v2.0"}, "clientId": {"entra-app-id"},
			"issuerPattern": {"https://login.example.com/{tenantId}/v2.0"}, "secretKind": {"encrypt"}, "sync": {"true"}, "roles": {"rol_1"},
			"domains": {"acme.com\nglobex.com\ninitech.com\nhooli.com"}, "drawer": {"idp_2"},
		}
	}
	// A blank secret keeps the stored one: none is sent.
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: form()})
	require.Len(t, h.ops.updated, 1)
	cmd := h.ops.updated[0]
	assert.Equal(t, "idp_2", cmd.ID)
	assert.Equal(t, "Entra renamed", deref(cmd.Name))
	assert.Nil(t, cmd.OIDCClientSecretRef)
	assert.Nil(t, cmd.MappingScope, "the provider stores no scope: none is sent unless chosen")
	assert.Equal(t, []string{"acme.com", "globex.com", "initech.com", "hooli.com"}, cmd.AllowedEmailDomains)
	assert.Equal(t, []string{"rol_1"}, cmd.AllowedRoleIDs)
	mustContain(t, body, "Identity provider updated successfully")
	mustNotContain(t, body, idpStoredSecret)

	// A new secret is sent as the local kind; the answer never has it.
	f := form()
	f.Set("secretValue", idpTypedSecret)
	body = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: f})
	require.Len(t, h.ops.updated, 2)
	assert.Equal(t, "encrypt:"+idpTypedSecret, deref(h.ops.updated[1].OIDCClientSecretRef))
	mustNotContain(t, body, idpTypedSecret, idpStoredSecret)

	// The type is the provider's own: an INTERNAL provider's post cannot make
	// it OIDC, and sends no OIDC part.
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_1/edit", datastar: true, form: url.Values{
		"name": {"Internal"}, "type": {"OIDC"}, "issuerUrl": {"https://evil.test"}, "clientId": {"x"}, "secretValue": {idpTypedSecret}, "domains": {""},
	}})
	require.Len(t, h.ops.updated, 3)
	assert.Nil(t, h.ops.updated[2].OIDCIssuerURL)
	assert.Nil(t, h.ops.updated[2].OIDCClientSecretRef)
	assert.NotNil(t, h.ops.updated[2].AllowedEmailDomains, "the whole domain set is sent, empty included")

	// An OIDC provider needs its issuer and client id, as the SPA's form does.
	f = form()
	f.Set("issuerUrl", "")
	mustContain(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: f}), `id="idp-form-issuer-url-err"`)
	assert.Len(t, h.ops.updated, 3)
}

// TestIdentityProvidersUpdateReleasesDomainsOnlyWhenConfirmed: removing a
// domain sends its users back to password sign-in, so the save waits for the
// SPA's confirmation; a post that names no confirmation never reaches the use
// case.
func TestIdentityProvidersUpdateReleasesDomainsOnlyWhenConfirmed(t *testing.T) {
	h := newIdpHarness()
	form := func() url.Values {
		return url.Values{
			"name": {"Entra"}, "issuerUrl": {"https://login.example.com"}, "clientId": {"entra-app-id"}, "secretKind": {"encrypt"},
			"domains": {"acme.com\nglobex.com"}, "scope": {""},
		}
	}
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: form()})
	assert.Empty(t, h.ops.updated, "nothing is saved before the release is confirmed")
	mustContain(t, body, "Remove Email Domains", "initech.com", "hooli.com", "fall back to internal password authentication", `name="confirmRelease"`)

	f := form()
	f.Set("confirmRelease", "yes")
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: f})
	require.Len(t, h.ops.updated, 1)
	assert.Equal(t, []string{"acme.com", "globex.com"}, h.ops.updated[0].AllowedEmailDomains)

	// Adding a domain needs no confirmation; the scope and client go along.
	g := form()
	g.Set("domains", "acme.com\nglobex.com\ninitech.com\nhooli.com\nnew.com")
	g.Set("scope", "CLIENT")
	g.Set("client", "acme")
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: g})
	require.Len(t, h.ops.updated, 2)
	assert.Equal(t, "CLIENT", deref(h.ops.updated[1].MappingScope))
	assert.Equal(t, "clt_acme", deref(h.ops.updated[1].PrimaryClientID))
}

func TestIdentityProvidersRolesNeedThePermissionToListThem(t *testing.T) {
	h := newIdpHarness()
	// An update-only caller cannot list roles: the allowed roles stay as stored.
	ac := connUser(idpPermView, idpPermUpdate)
	form := url.Values{
		"name": {"Entra"}, "issuerUrl": {"https://login.example.com"}, "clientId": {"entra-app-id"}, "secretKind": {"encrypt"}, "sync": {"true"},
		"domains": {"acme.com\nglobex.com\ninitech.com\nhooli.com"}, "roles": {"rol_2"},
	}
	body := h.do(t, ac, req{target: "/ui/identity-providers/idp_2/edit", datastar: true})
	mustContain(t, body, "Choosing roles needs permission to view roles")
	h.do(t, ac, req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: form})
	require.Len(t, h.ops.updated, 1)
	assert.Equal(t, []string{"rol_1"}, h.ops.updated[0].AllowedRoleIDs, "the role a caller could not list is not taken from the post")
}

func TestIdentityProvidersDelete(t *testing.T) {
	h := newIdpHarness()
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_3/delete", datastar: true, form: url.Values{"drawer": {"idp_3"}}})
	assert.Equal(t, []string{"idp_3"}, h.ops.deleted)
	mustContain(t, body, "Identity provider deleted", "selIdp")

	// The use case's refusal (domains still mapped) is the message shown.
	h.ops.err = usecase.Conflict("DOMAINS_STILL_MAPPED", "Identity provider still routes email domains (acme.com); move or delete those mappings first")
	refused := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/delete", datastar: true})
	mustContain(t, refused, "still routes email domains")
	mustNotContain(t, refused, "Identity provider deleted")

	// A native post redirects with the flash key.
	h2 := newIdpHarness()
	rec := h2.rec(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_3/delete", form: url.Values{}})
	assert.Equal(t, http.StatusSeeOther, rec.Code)
	assert.Equal(t, "/ui/identity-providers", rec.Header().Get("Location"))
}

func TestIdentityProvidersSecretKindsAreTheSPAs(t *testing.T) {
	var got []string
	for _, k := range idpSecretKinds {
		got = append(got, k.Prefix)
	}
	assert.Equal(t, []string{"encrypt:", "aws-sm://", "aws-ps://", "gcp-sm://", "vault://"}, got)
	f := idpForm{SecretKind: "bogus"}
	assert.Equal(t, "encrypt", f.secretKindValue())
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	_, ok := f.secretRefFrom(r)
	assert.True(t, ok, "an unknown kind falls back to the local one")
}

// TestIdpOpsEncryptTheSecretLikeTheAPI: a plaintext secret is encrypted before
// the use case sees it (and refused without a key), a reference to a secret
// held elsewhere passes through, nothing entered stays nothing.
func TestIdpOpsEncryptTheSecretLikeTheAPI(t *testing.T) {
	plain := "encrypt:" + idpTypedSecret
	_, err := IdpOps{}.encryptSecretRef(&plain)
	ue, ok := errors.AsType[*usecase.Error](err)
	require.True(t, ok, "%v", err)
	assert.Equal(t, "ENCRYPTION_NOT_CONFIGURED", ue.Code)
	assert.NotContains(t, err.Error(), idpTypedSecret)

	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	ops := IdpOps{Enc: enc}
	out, err := ops.encryptSecretRef(&plain)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(deref(out), "encrypted:"))
	assert.NotContains(t, deref(out), idpTypedSecret)

	ref := "vault://kv/idp#secret"
	out, err = ops.encryptSecretRef(&ref)
	require.NoError(t, err)
	assert.Equal(t, ref, deref(out))
	bogus := "aws-smm://typo"
	_, err = ops.encryptSecretRef(&bogus)
	ue, ok = errors.AsType[*usecase.Error](err)
	require.True(t, ok)
	assert.Equal(t, "UNSUPPORTED_SECRET_SCHEME", ue.Code)
	out, err = ops.encryptSecretRef(nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}
