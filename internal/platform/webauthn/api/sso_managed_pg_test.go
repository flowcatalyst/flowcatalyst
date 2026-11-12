//go:build integration

package api

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/webauthntest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// An SSO-managed user (email domain mapped to an OIDC identity provider)
// authenticates at the identity provider: a passkey can neither be registered for
// them nor sign them in, or a user disabled at the provider would keep platform
// access through the passkey.

// ssoRig is a rig whose State can tell SSO-managed accounts and mint sessions.
func ssoRig(t *testing.T, tag string) *rig {
	t.Helper()
	r := newRig(t, tag)
	pool := testpg.Pool(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	principals := principal.NewRepository(pool)
	prov, err := provider.NewProvider(provider.Config{Issuer: "https://sso.example.test", SigningKey: pemKey}, principals, role.NewRepository(pool))
	require.NoError(t, err)
	r.s.Principals = principals
	r.s.Mappings = emaildomainmapping.NewRepository(pool)
	r.s.IdentityProviders = identityprovider.NewRepository(pool)
	r.s.Provider = prov
	return r
}

// mapDomainToOIDC routes domain to a (new) OIDC identity provider.
func mapDomainToOIDC(t *testing.T, domain string) {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
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

func sessionCtx(p *principal.Principal) context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{PrincipalID: string(p.ID)})
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%v", err)
	require.Equal(t, code, ue.Code, "%v", err)
}

// shapeOf is the structure of a JSON value with every scalar replaced by its
// JSON type, so two responses compare by shape and not by their random values.
func shapeOf(t *testing.T, v any) any {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var generic any
	require.NoError(t, json.Unmarshal(b, &generic))
	var walk func(any) any
	walk = func(x any) any {
		switch tv := x.(type) {
		case map[string]any:
			out := map[string]any{}
			for k, e := range tv {
				out[k] = walk(e)
			}
			return out
		case []any:
			out := make([]any, len(tv))
			for i, e := range tv {
				out[i] = walk(e)
			}
			return out
		case string:
			return "string"
		case float64:
			return "number"
		case bool:
			return "bool"
		}
		return x
	}
	return walk(generic)
}

func TestPasskeyRegistrationRefusedForSSOManagedUser(t *testing.T) {
	r := ssoRig(t, "ssoreg")
	mapDomainToOIDC(t, "ssoreg.test")

	// SSO-managed: refused at begin and at complete, with the same code the
	// change-password path uses.
	_, err := r.s.registerBegin(sessionCtx(r.alice), &registerBeginInput{})
	requireCode(t, err, "SSO_MANAGED")
	_, err = r.s.registerComplete(sessionCtx(r.alice), &registerCompleteInput{
		Body: RegisterCompleteRequest{StateID: "x", Name: new("my key"), Credential: json.RawMessage(`{}`)},
	})
	requireCode(t, err, "SSO_MANAGED")

	// A user outside any SSO domain is unaffected.
	pool := testpg.Pool(t)
	_, err = pool.Exec(context.Background(),
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ('prn_ssoregcar', 'USER', 'ANCHOR', 'carol', TRUE, 'carol@plain-ssoreg.test')`)
	require.NoError(t, err)
	carol := &principal.Principal{ID: "prn_ssoregcar"}
	out, err := r.s.registerBegin(sessionCtx(carol), &registerBeginInput{})
	require.NoError(t, err)
	require.NotEmpty(t, out.Body.StateID)
}

// authenticate/begin for an SSO-managed email answers as it does for an unknown one.
func TestPasskeyAuthenticateBeginIsIndistinguishableForSSOManagedUser(t *testing.T) {
	r := ssoRig(t, "ssobeg")
	_, _ = r.register(r.alice) // alice holds a passkey, registered before her domain moved to SSO
	ctx := context.Background()

	real, err := r.s.authenticateBegin(ctx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@ssobeg.test"}})
	require.NoError(t, err)
	_, isReal := real.Body.Options.(*protocol.CredentialAssertion)
	require.True(t, isReal, "control: a non-SSO user with a passkey gets a real ceremony")

	mapDomainToOIDC(t, "ssobeg.test")
	sso, err := r.s.authenticateBegin(ctx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@ssobeg.test"}})
	require.NoError(t, err)
	unknown, err := r.s.authenticateBegin(ctx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "nobody@elsewhere.test"}})
	require.NoError(t, err)

	require.Equal(t, shapeOf(t, unknown.Body), shapeOf(t, sso.Body), "same response shape as an unknown email")
	_, isReal = sso.Body.Options.(*protocol.CredentialAssertion)
	require.False(t, isReal, "no real challenge for an SSO-managed user")

	// the decoy's state id is not a stored ceremony: completing against it fails
	consumed, err := r.s.Service.Ceremonies().ConsumeAuthentication(ctx, sso.Body.StateID)
	require.True(t, err != nil || consumed == nil)
}

// authenticate/complete cannot yield a session for an SSO-managed user, even with a
// valid assertion against a ceremony begun before the domain moved to SSO.
func TestPasskeyAuthenticateCompleteRefusedForSSOManagedUser(t *testing.T) {
	r := ssoRig(t, "ssocmp")
	_, a := r.register(r.alice)
	ctx := context.Background()

	assertion := func(begin *authenticateBeginOutput) *authenticateCompleteInput {
		opts, ok := begin.Body.Options.(*protocol.CredentialAssertion)
		require.True(t, ok)
		return &authenticateCompleteInput{Body: AuthenticateCompleteRequest{
			StateID:    begin.Body.StateID,
			Credential: a.AssertWithCounter(opts.Response.Challenge.String(), origin, rpID, []byte(r.alice.ID), 5),
		}}
	}

	// control: before the move the same flow signs alice in
	begin, err := r.s.authenticateBegin(ctx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@ssocmp.test"}})
	require.NoError(t, err)
	out, err := r.s.authenticateComplete(ctx, assertion(begin))
	require.NoError(t, err)
	require.NotEmpty(t, out.SetCookie, "control: a non-SSO user gets a session cookie")

	// begun while alice is not SSO-managed, completed after her domain moved
	begin, err = r.s.authenticateBegin(ctx, &authenticateBeginInput{Body: AuthenticateBeginRequest{Email: "alice@ssocmp.test"}})
	require.NoError(t, err)
	in := assertion(begin)
	in.Body.Credential = webauthntestAssert(t, a, begin, r.alice.ID, 9)
	mapDomainToOIDC(t, "ssocmp.test")
	out, err = r.s.authenticateComplete(ctx, in)
	requireCode(t, err, "INVALID_CREDENTIALS")
	require.Nil(t, out, "no session for an SSO-managed user")
}

func webauthntestAssert(t *testing.T, a *webauthntest.Authenticator, begin *authenticateBeginOutput, id ids.PrincipalID, counter uint32) json.RawMessage {
	t.Helper()
	opts, ok := begin.Body.Options.(*protocol.CredentialAssertion)
	require.True(t, ok)
	return a.AssertWithCounter(opts.Response.Challenge.String(), origin, rpID, []byte(id), counter)
}
