package identityprovider

import (
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // test: the SHA-1 thumbprint
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func certTestEnc(t *testing.T) *encryption.Service {
	t.Helper()
	k, err := encryption.GenerateKey()
	require.NoError(t, err)
	enc, err := encryption.New(k)
	require.NoError(t, err)
	return enc
}

func TestGenerateCertificate(t *testing.T) {
	enc := certTestEnc(t)
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	c, err := GenerateCertificate(enc, "idp_abc", "entra-prod", now)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(c.ID, "ipc_"), "the ipc TSID prefix: %s", c.ID)
	assert.Equal(t, "idp_abc", c.IdentityProviderID)

	// The PEM parses as a self-signed RSA 3072 certificate.
	block, _ := pem.Decode([]byte(c.CertificatePEM))
	require.NotNil(t, block)
	require.Equal(t, "CERTIFICATE", block.Type)
	cert, err := x509.ParseCertificate(block.Bytes)
	require.NoError(t, err)
	assert.Equal(t, "FlowCatalyst entra-prod", cert.Subject.CommonName)
	assert.Equal(t, cert.Subject.String(), cert.Issuer.String(), "self-signed")
	require.NoError(t, cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature))
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	require.True(t, ok)
	assert.Equal(t, 3072, pub.N.BitLen())
	assert.Equal(t, x509.KeyUsageDigitalSignature, cert.KeyUsage)
	assert.Positive(t, cert.SerialNumber.Sign())

	// Validity: five minutes of clock slack back, 24 months forward.
	assert.True(t, c.NotBefore.Equal(now.Add(-5*time.Minute)), "not_before = now - 5m")
	assert.True(t, c.NotAfter.Equal(now.AddDate(0, 24, 0)), "not_after = now + 24 months")
	assert.True(t, cert.NotBefore.Equal(c.NotBefore.Truncate(time.Second)))
	assert.True(t, cert.NotAfter.Equal(c.NotAfter.Truncate(time.Second)))

	// Thumbprints are the upper-case hex of the DER digests.
	s1 := sha1.Sum(block.Bytes) //nolint:gosec // test
	s256 := sha256.Sum256(block.Bytes)
	assert.Equal(t, strings.ToUpper(hex.EncodeToString(s1[:])), c.ThumbprintSHA1)
	assert.Equal(t, strings.ToUpper(hex.EncodeToString(s256[:])), c.ThumbprintSHA256)
	assert.Len(t, c.ThumbprintSHA1, 40)
	assert.Len(t, c.ThumbprintSHA256, 64)
	der, err := c.DER()
	require.NoError(t, err)
	assert.Equal(t, block.Bytes, der)

	// The private key is sealed (not PEM), opens with its own AAD, and is the
	// certificate's key.
	assert.False(t, strings.Contains(c.PrivateKeyRef, "PRIVATE KEY"))
	key, err := c.PrivateKey(enc)
	require.NoError(t, err)
	assert.True(t, key.PublicKey.Equal(pub))
	_, _, err = enc.DecryptBound(c.PrivateKeyRef, CertificateAAD("ipc_other"))
	require.Error(t, err, "bound to its own row")
	assert.Equal(t, []byte("oauth_identity_provider_certificates:"+c.ID), CertificateAAD(c.ID))
}

func TestGenerateCertificateIsFreshEachTime(t *testing.T) {
	enc := certTestEnc(t)
	a, err := GenerateCertificate(enc, "idp_a", "x", time.Now())
	require.NoError(t, err)
	b, err := GenerateCertificate(enc, "idp_a", "x", time.Now())
	require.NoError(t, err)
	assert.NotEqual(t, a.ThumbprintSHA256, b.ThumbprintSHA256, "a new key pair and serial each time")
	assert.NotEqual(t, a.ID, b.ID)
}

func TestGenerateCertificateNeedsEncryption(t *testing.T) {
	_, err := GenerateCertificate(nil, "idp_a", "x", time.Now())
	assert.ErrorIs(t, err, ErrCertificateEncryptionNotConfigured)
	c := &Certificate{ID: "ipc_1"}
	_, err = c.PrivateKey(nil)
	assert.ErrorIs(t, err, ErrCertificateEncryptionNotConfigured)
}

func TestCertificateExpiresWithin(t *testing.T) {
	now := time.Now()
	c := &Certificate{NotAfter: now.Add(30 * 24 * time.Hour)}
	assert.True(t, c.ExpiresWithin(now, 60*24*time.Hour))
	assert.False(t, c.ExpiresWithin(now, 10*24*time.Hour))
}

func TestParseClientAuthMethod(t *testing.T) {
	for _, s := range []string{"CLIENT_SECRET", "CERTIFICATE", "FEDERATED"} {
		m, ok := ParseClientAuthMethod(s)
		assert.True(t, ok, s)
		assert.Equal(t, ClientAuthMethod(s), m)
	}
	for _, s := range []string{"", "client_secret", "NONE", "CERTIFICATE "} {
		_, ok := ParseClientAuthMethod(s)
		assert.False(t, ok, "%q", s)
	}
}
