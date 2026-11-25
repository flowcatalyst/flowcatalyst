//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
)

func certDeps(t *testing.T) (operations.Deps, *encryption.Service) {
	t.Helper()
	d := deps(t)
	key, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(key)
	require.NoError(t, err)
	d.Enc = enc
	return d, enc
}

func certByStatus(t *testing.T, d operations.Deps, idpID string, st identityprovider.CertificateStatus) *identityprovider.Certificate {
	t.Helper()
	c, err := d.Repo.FindCertificate(context.Background(), idpID, st)
	require.NoError(t, err)
	return c
}

func certCount(t *testing.T, idpID string) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_identity_provider_certificates WHERE identity_provider_id = $1`, idpID).Scan(&n))
	return n
}

func TestCertificateLifecycle_GeneratePromoteDiscard(t *testing.T) {
	t.Parallel()
	d, enc := certDeps(t)
	uow := testpg.NewUoW(t)
	idp := mustCreateOIDC(t, d, uow, "cert-life", nil)
	id := idp.IdentityProviderID
	cmd := operations.CertificateCommand{IdentityProviderID: id}

	// First certificate becomes ACTIVE.
	first, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	assert.Equal(t, "ACTIVE", first.Status)
	require.NotNil(t, certByStatus(t, d, id, identityprovider.CertificateActive))
	assert.Nil(t, certByStatus(t, d, id, identityprovider.CertificateNext))

	// The next generate becomes NEXT; another replaces it.
	second, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	assert.Equal(t, "NEXT", second.Status)
	third, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	assert.Equal(t, "NEXT", third.Status)
	assert.NotEqual(t, second.CertificateID, third.CertificateID)
	assert.Equal(t, 2, certCount(t, id), "one ACTIVE and one NEXT: the replaced NEXT is gone")
	assert.Equal(t, third.CertificateID, certByStatus(t, d, id, identityprovider.CertificateNext).ID)

	// The private key opens with its own AAD and with no other row's.
	active := certByStatus(t, d, id, identityprovider.CertificateActive)
	_, err = active.PrivateKey(enc)
	require.NoError(t, err)
	_, _, err = enc.DecryptBound(active.PrivateKeyRef, identityprovider.CertificateAAD(third.CertificateID))
	require.Error(t, err, "a ciphertext moved to another row does not open")
	_, _, err = enc.DecryptBound(active.PrivateKeyRef, []byte("oauth_identity_providers:"+active.ID))
	require.Error(t, err, "nor does one under another table's AAD")
	moved := *active
	moved.ID = third.CertificateID
	_, err = moved.PrivateKey(enc)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), active.PrivateKeyRef, "no key material in errors")

	// Promote: NEXT becomes ACTIVE, the old ACTIVE is deleted.
	promoted, err := runAuthorizedTx(uow, operations.PromoteIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	assert.Equal(t, third.CertificateID, promoted.CertificateID)
	assert.Equal(t, third.CertificateID, certByStatus(t, d, id, identityprovider.CertificateActive).ID)
	assert.Nil(t, certByStatus(t, d, id, identityprovider.CertificateNext))
	assert.Equal(t, 1, certCount(t, id))
	gone, err := d.Repo.FindCertificateByID(context.Background(), first.CertificateID)
	require.NoError(t, err)
	assert.Nil(t, gone, "the retired ACTIVE is deleted")

	// Nothing to promote or discard now.
	_, err = runAuthorizedTx(uow, operations.PromoteIdentityProviderCertificate(d), cmd)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "NO_NEXT_CERTIFICATE")
	_, err = runAuthorizedTx(uow, operations.DiscardIdentityProviderCertificate(d), cmd)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "NO_NEXT_CERTIFICATE")

	// Discard removes only the NEXT.
	next, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	disc, err := runAuthorizedTx(uow, operations.DiscardIdentityProviderCertificate(d), cmd)
	require.NoError(t, err)
	assert.Equal(t, next.CertificateID, disc.CertificateID)
	assert.Equal(t, 1, certCount(t, id))
	assert.NotNil(t, certByStatus(t, d, id, identityprovider.CertificateActive))
}

func TestCertificate_RefusalsAndEvents(t *testing.T) {
	t.Parallel()
	d, _ := certDeps(t)
	uow := testpg.NewUoW(t)

	internal := mustCreate(t, d, uow, "cert-internal", "Internal-ish")
	_, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d),
		operations.CertificateCommand{IdentityProviderID: internal.IdentityProviderID})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "IDENTITY_PROVIDER_NOT_OIDC")

	oidc := mustCreateOIDC(t, d, uow, "cert-noenc", nil)
	noEnc := d
	noEnc.Enc = nil
	_, err = runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(noEnc),
		operations.CertificateCommand{IdentityProviderID: oidc.IdentityProviderID})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ENCRYPTION_NOT_CONFIGURED")
	assert.Zero(t, certCount(t, oidc.IdentityProviderID))

	_, err = runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d),
		operations.CertificateCommand{IdentityProviderID: "idp_missing0000"})
	require.Error(t, err)

	// One event per operation, each with its own type.
	res, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d),
		operations.CertificateCommand{IdentityProviderID: oidc.IdentityProviderID})
	require.NoError(t, err)
	var data string
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(),
		`SELECT data::text FROM msg_events WHERE type = $1 AND subject = $2`,
		operations.CertificateGeneratedType, "platform.identityprovider."+oidc.IdentityProviderID).Scan(&data))
	assert.JSONEq(t, `{"identityProviderId":"`+oidc.IdentityProviderID+`","code":"cert-noenc","certificateId":"`+res.CertificateID+
		`","status":"ACTIVE","thumbprintSha1":"`+certByStatus(t, d, oidc.IdentityProviderID, identityprovider.CertificateActive).ThumbprintSHA1+
		`","notAfter":"`+certByStatus(t, d, oidc.IdentityProviderID, identityprovider.CertificateActive).NotAfter.UTC().Format("2006-01-02T15:04:05Z")+`"}`, data)
}

func TestDeleteIdentityProvider_RemovesItsCertificates(t *testing.T) {
	t.Parallel()
	d, _ := certDeps(t)
	uow := testpg.NewUoW(t)
	idp := mustCreateOIDC(t, d, uow, "cert-cascade", nil)
	other := mustCreateOIDC(t, d, uow, "cert-cascade-other", nil)
	for _, id := range []string{idp.IdentityProviderID, idp.IdentityProviderID, other.IdentityProviderID} {
		_, err := runAuthorizedTx(uow, operations.GenerateIdentityProviderCertificate(d), operations.CertificateCommand{IdentityProviderID: id})
		require.NoError(t, err)
	}
	require.Equal(t, 2, certCount(t, idp.IdentityProviderID))

	_, err := runAuthorized(uow, operations.DeleteIdentityProvider(d), operations.DeleteCommand{ID: idp.IdentityProviderID})
	require.NoError(t, err)
	assert.Zero(t, certCount(t, idp.IdentityProviderID), "the provider's certificates go with it")
	assert.Equal(t, 1, certCount(t, other.IdentityProviderID), "another provider's are untouched")
}

func TestClientAuthMethod_CreateUpdateValidation(t *testing.T) {
	t.Parallel()
	d, _ := certDeps(t)
	uow := testpg.NewUoW(t)
	ctx := context.Background()
	issuer, client := "https://login.cam.example.com/v2.0", "cam-client"

	_, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code: "cam-bad", Name: "x", Type: "OIDC", OIDCIssuerURL: &issuer, OIDCClientID: &client, OIDCClientAuthMethod: new("MAGIC"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_CLIENT_AUTH_METHOD")

	_, err = runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code: "cam-internal", Name: "x", Type: "INTERNAL", OIDCClientAuthMethod: new("CERTIFICATE"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "CLIENT_AUTH_METHOD_NOT_OIDC")

	secret := "encrypted:abc"
	res, err := runAuthorizedTx(uow, operations.CreateIdentityProvider(d), operations.CreateCommand{
		Code: "cam-ok", Name: "x", Type: "OIDC", OIDCIssuerURL: &issuer, OIDCClientID: &client, OIDCClientSecretRef: &secret,
	})
	require.NoError(t, err)
	ip, err := d.Repo.FindByID(ctx, res.IdentityProviderID)
	require.NoError(t, err)
	assert.Equal(t, identityprovider.ClientAuthSecret, ip.OIDCClientAuthMethod, "the default")

	// Switching away keeps the stored secret; switching back finds it.
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID: ip.ID, OIDCClientAuthMethod: new("FEDERATED"),
	})
	require.NoError(t, err)
	ip, err = d.Repo.FindByID(ctx, ip.ID)
	require.NoError(t, err)
	assert.Equal(t, identityprovider.ClientAuthFederated, ip.OIDCClientAuthMethod)
	require.NotNil(t, ip.OIDCClientSecretRef)
	assert.Equal(t, secret, *ip.OIDCClientSecretRef)

	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID: ip.ID, OIDCClientAuthMethod: new("nope"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_CLIENT_AUTH_METHOD")

	internal := mustCreate(t, d, uow, "cam-internal2", "i")
	_, err = runAuthorizedTx(uow, operations.UpdateIdentityProvider(d), operations.UpdateCommand{
		ID: internal.IdentityProviderID, OIDCClientAuthMethod: new("CERTIFICATE"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "CLIENT_AUTH_METHOD_NOT_OIDC")
}
