package pages

import (
	"context"
	"crypto/x509"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/go-sdk/usecase"
)

type fakeCertStore struct {
	certs []identityprovider.Certificate
}

func (f *fakeCertStore) FindCertificates(_ context.Context, idpID string) ([]identityprovider.Certificate, error) {
	var out []identityprovider.Certificate
	for _, c := range f.certs {
		if c.IdentityProviderID == idpID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f *fakeCertStore) FindCertificateByID(_ context.Context, id string) (*identityprovider.Certificate, error) {
	for i := range f.certs {
		if f.certs[i].ID == id {
			c := f.certs[i]
			return &c, nil
		}
	}
	return nil, nil
}

type fakeCertOps struct {
	calls []string
	err   error
}

func (o *fakeCertOps) run(name, id string) (idpops.CertificateResult, error) {
	o.calls = append(o.calls, name+":"+id)
	return idpops.CertificateResult{}, o.err
}

func (o *fakeCertOps) GenerateCertificate(_ context.Context, id string) (idpops.CertificateResult, error) {
	return o.run("generate", id)
}

func (o *fakeCertOps) PromoteCertificate(_ context.Context, id string) (idpops.CertificateResult, error) {
	return o.run("promote", id)
}

func (o *fakeCertOps) DiscardCertificate(_ context.Context, id string) (idpops.CertificateResult, error) {
	return o.run("discard", id)
}

type certHarness struct {
	*idpHarness
	certs *fakeCertStore
	cops  *fakeCertOps
}

// newCertHarness makes the Keycloak fixture (idp_3) a CERTIFICATE provider and
// the Entra fixture (idp_2) a FEDERATED one.
func newCertHarness(t *testing.T, _ string) *certHarness {
	t.Helper()
	h := newIdpHarness()
	for i := range h.store.providers {
		switch h.store.providers[i].ID {
		case "idp_3":
			h.store.providers[i].OIDCClientAuthMethod = identityprovider.ClientAuthCertificate
		case "idp_2":
			h.store.providers[i].OIDCClientAuthMethod = identityprovider.ClientAuthFederated
		}
	}
	return &certHarness{idpHarness: h, certs: &fakeCertStore{}, cops: &fakeCertOps{}}
}

func (h *certHarness) page() *IdentityProviders {
	p := h.idpHarness.page()
	p.Certs, p.CertOps, p.Issuer = h.certs, h.cops, "https://fc.example.com"
	return p
}

func certFixture(t *testing.T, id string, st identityprovider.CertificateStatus, notAfter time.Time) identityprovider.Certificate {
	t.Helper()
	k, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(k)
	require.NoError(t, err)
	c, err := identityprovider.GenerateCertificate(enc, "idp_3", "keycloak", time.Now())
	require.NoError(t, err)
	c.ID, c.Status, c.NotAfter = id, st, notAfter
	return *c
}

func TestIdentityProviderDrawerCertificateSection(t *testing.T) {
	h := newCertHarness(t, "")
	far := time.Now().AddDate(1, 0, 0)
	serve := func(perms ...string) string {
		ac := anchorAdmin()
		if perms != nil {
			ac = connUser(perms...)
		}
		return serveMounted(t, ac, req{target: "/ui/identity-providers/idp_3", datastar: true}, h.page()).Body.String()
	}

	// No certificate yet: the only action is to generate one.
	body := serve()
	mustContain(t, body, "Client credential", "Certificate", "No certificate yet", "Generate certificate",
		`data-action="/ui/identity-providers/idp_3/certificates/generate"`)
	mustNotContain(t, body, "Generate next", "Make active", "Discard", "Not configured") // no secret row for this method

	// ACTIVE only: generate the next one.
	active := certFixture(t, "ipc_active", identityprovider.CertificateActive, far)
	h.certs.certs = []identityprovider.Certificate{active}
	body = serve()
	mustContain(t, body, "Generate next", active.ThumbprintSHA1, "Download .cer",
		`href="/ui/identity-providers/idp_3/certificates/ipc_active/download"`, "Upload the certificate in Entra")
	mustNotContain(t, body, "Generate certificate", "Make active", "Certificate expiry")

	// ACTIVE and NEXT: make the next active, or discard it.
	next := certFixture(t, "ipc_next", identityprovider.CertificateNext, far)
	h.certs.certs = []identityprovider.Certificate{active, next}
	body = serve()
	mustContain(t, body, "Make active", "Discard", `data-action="/ui/identity-providers/idp_3/certificates/promote"`,
		`data-action="/ui/identity-providers/idp_3/certificates/discard"`)
	// The shared confirm dialogs are on the page, for an updater only.
	page := serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_3"}, h.page()).Body.String()
	mustContain(t, page, `id="idp-cert-promote"`, `id="idp-cert-discard"`)
	viewerPage := serveMounted(t, connUser(idpPermView), req{target: "/ui/identity-providers/idp_3"}, h.page()).Body.String()
	mustNotContain(t, viewerPage, `id="idp-cert-promote"`, `id="idp-cert-discard"`)
	mustNotContain(t, body, "Generate next", "Generate certificate")

	// A viewer sees the certificates and their download, with no control.
	body = serve(idpPermView)
	mustContain(t, body, "Download .cer", active.ThumbprintSHA1)
	mustNotContain(t, body, "Make active", "Generate next", "/certificates/promote", "/certificates/discard", `id="idp-cert-promote"`)

	// An ACTIVE certificate close to expiry (or past it) is warned about.
	soon := certFixture(t, "ipc_active", identityprovider.CertificateActive, time.Now().AddDate(0, 0, 30))
	h.certs.certs = []identityprovider.Certificate{soon}
	mustContain(t, serve(), "Certificate expiry", "expires on")
	gone := certFixture(t, "ipc_active", identityprovider.CertificateActive, time.Now().AddDate(0, 0, -2))
	h.certs.certs = []identityprovider.Certificate{gone}
	mustContain(t, serve(), "Certificate expiry", "expired on")
}

func TestIdentityProviderDrawerFederatedSection(t *testing.T) {
	h := newCertHarness(t, "")
	body := serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2", datastar: true}, h.page()).Body.String()
	mustContain(t, body, "Client credential", "Federated credential", "https://fc.example.com", "identity-provider:idp_2",
		"api://AzureADTokenExchange", "Other issuer", "does not expire")
	mustNotContain(t, body, "Issuer is not public HTTPS", "Generate certificate", "Configured")

	// A local issuer cannot be reached by the provider.
	p := h.page()
	p.Issuer = "http://localhost:8080"
	body = serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2", datastar: true}, p).Body.String()
	mustContain(t, body, "Issuer is not public HTTPS")

	// The client-secret provider shows what it always did.
	body = serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_1", datastar: true}, h.page()).Body.String()
	mustNotContain(t, body, "Client credential")
}

func TestIdentityProviderClientSecretCredentialSection(t *testing.T) {
	h := newIdpHarness()
	body := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_3", datastar: true})
	mustContain(t, body, "Client credential", "Client secret", "Not configured")
	mustNotContain(t, body, "Generate certificate", "identity-provider:idp_3")
}

func TestIdentityProviderCertificateDownload(t *testing.T) {
	h := newCertHarness(t, "")
	active := certFixture(t, "ipc_active", identityprovider.CertificateActive, time.Now().AddDate(1, 0, 0))
	other := certFixture(t, "ipc_other", identityprovider.CertificateActive, time.Now().AddDate(1, 0, 0))
	other.IdentityProviderID = "idp_2"
	h.certs.certs = []identityprovider.Certificate{active, other}
	rec := serveMounted(t, connUser(idpPermView), req{target: "/ui/identity-providers/idp_3/certificates/ipc_active/download"}, h.page())
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, `attachment; filename="keycloak-`+active.ThumbprintSHA1+`.cer"`, rec.Header().Get("Content-Disposition"))
	assert.Equal(t, "application/x-x509-ca-cert", rec.Header().Get("Content-Type"))
	cert, err := x509.ParseCertificate(rec.Body.Bytes())
	require.NoError(t, err, "the body is the DER certificate")
	assert.Equal(t, "FlowCatalyst keycloak", cert.Subject.CommonName)
	assert.NotContains(t, rec.Body.String(), "PRIVATE KEY")

	// Another provider's certificate is not served under this one's id.
	assert.Equal(t, http.StatusNotFound, serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_3/certificates/ipc_other/download"}, h.page()).Code)
	assert.Equal(t, http.StatusNotFound, serveMounted(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_3/certificates/ipc_missing/download"}, h.page()).Code)
	// The read permission is needed, and anchor scope.
	assert.Equal(t, http.StatusForbidden, serveMounted(t, connUser(), req{target: "/ui/identity-providers/idp_3/certificates/ipc_active/download"}, h.page()).Code)
}

func TestIdentityProviderCertificateActions(t *testing.T) {
	for _, tc := range []struct{ path, call string }{
		{"generate", "generate:idp_3"}, {"promote", "promote:idp_3"}, {"discard", "discard:idp_3"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			h := newCertHarness(t, "")
			target := "/ui/identity-providers/idp_3/certificates/" + tc.path

			// A datastar post runs the operation and re-renders the drawer.
			rec := serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: target, datastar: true, form: url.Values{}}, h.page())
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, []string{tc.call}, h.cops.calls)
			mustContain(t, rec.Body.String(), "Client credential", "Keycloak")

			// A native post redirects back to the provider.
			h2 := newCertHarness(t, "")
			rec = serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: target, form: url.Values{}}, h2.page())
			assert.Equal(t, http.StatusSeeOther, rec.Code)
			assert.Equal(t, "/ui/identity-providers/idp_3", rec.Header().Get("Location"))

			// Without the update permission (or off the anchor) the use case is never reached.
			for name, ac := range map[string]*auth.AuthContext{"viewer": connUser(idpPermView), "none": connUser(), "update only": connUser(idpPermUpdate)} {
				h3 := newCertHarness(t, "")
				serveMounted(t, ac, req{method: http.MethodPost, target: target, datastar: true, form: url.Values{}}, h3.page())
				assert.Empty(t, h3.cops.calls, name+": a forged post reached the use case")
			}

			// A refusal is reported, not swallowed.
			h4 := newCertHarness(t, "")
			h4.cops.err = usecase.BusinessRule("REFUSED_FOR_TEST", "refused for the test")
			rec = serveMounted(t, anchorAdmin(), req{method: http.MethodPost, target: target, datastar: true, form: url.Values{}}, h4.page())
			mustContain(t, rec.Body.String(), "refused for the test")
		})
	}
}

func TestIdentityProviderFormClientAuthentication(t *testing.T) {
	h := newIdpHarness()
	form := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/new", datastar: true})
	mustContain(t, form, `name="clientAuth"`, "Client authentication", "Client secret", "Certificate", "Federated credential",
		`idpFormAuth: &#39;CLIENT_SECRET&#39;`, `data-show="$idpFormAuth === 'CLIENT_SECRET'"`)

	// Certificate: the typed secret is hidden by the form and never sent.
	base := func(method string) url.Values {
		return url.Values{
			"code": {"cert-idp"}, "name": {"Cert"}, "type": {"OIDC"}, "issuerUrl": {"https://i.example.com"}, "clientId": {"c"},
			"secretKind": {"encrypt"}, "secretValue": {idpTypedSecret}, "clientAuth": {method},
		}
	}
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: base("CERTIFICATE")})
	require.Len(t, h.ops.created, 1)
	assert.Equal(t, "CERTIFICATE", deref(h.ops.created[0].OIDCClientAuthMethod))
	assert.Nil(t, h.ops.created[0].OIDCClientSecretRef)

	// Client secret (the default): the secret goes through as before.
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: base("CLIENT_SECRET")})
	require.Len(t, h.ops.created, 2)
	assert.Equal(t, "encrypt:"+idpTypedSecret, deref(h.ops.created[1].OIDCClientSecretRef))

	// Edit: the provider's method is selected, a switch is sent, the stored secret is left.
	for i := range h.store.providers {
		if h.store.providers[i].ID == "idp_2" {
			h.store.providers[i].OIDCClientAuthMethod = identityprovider.ClientAuthFederated
		}
	}
	edit := h.do(t, anchorAdmin(), req{target: "/ui/identity-providers/idp_2/edit", datastar: true})
	mustContain(t, edit, `idpFormAuth: &#39;FEDERATED&#39;`)
	upd := url.Values{
		"name": {"Entra"}, "issuerUrl": {"https://login.example.com/<tenant>/v2.0"}, "clientId": {"entra-app-id"}, "multiTenant": {"true"},
		"issuerPattern": {"https://login.example.com/{tenantId}/v2.0"}, "secretKind": {"encrypt"}, "secretValue": {idpTypedSecret},
		"clientAuth": {"FEDERATED"}, "domains": {"acme.com\nglobex.com\ninitech.com\nhooli.com"}, "sync": {"true"}, "roles": {"rol_1"},
	}
	h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/idp_2/edit", datastar: true, form: upd})
	require.Len(t, h.ops.updated, 1)
	assert.Equal(t, "FEDERATED", deref(h.ops.updated[0].OIDCClientAuthMethod))
	assert.Nil(t, h.ops.updated[0].OIDCClientSecretRef, "a stored secret is kept when the method moves away from it")

	// An unknown method comes back on its own field.
	h.ops.err = usecase.Validation("INVALID_CLIENT_AUTH_METHOD", "oidcClientAuthMethod must be CLIENT_SECRET, CERTIFICATE or FEDERATED")
	body := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/identity-providers/new", datastar: true, form: base("MAGIC")})
	mustContain(t, body, "OidcClientAuthMethod must be", `id="idp-form-client-auth-err"`)
}
