package appupdate

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/secretref"
)

func testKey(t *testing.T, bits int) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, bits)
	require.NoError(t, err)
	return k
}

func pkcs1PEM(k *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}

// The header a client sends (Android CodeSigningConfiguration.getAcceptSignatureHeader
// serialises a Dictionary; iOS createAcceptSignatureHeader formats the same text).
func TestParseExpectSignature(t *testing.T) {
	t.Parallel()
	got, ok, err := ParseExpectSignature(`sig, keyid="main", alg="rsa-v1_5-sha256"`)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "main", got.KeyID)

	got, ok, err = ParseExpectSignature(`sig,keyid="root",alg="rsa-v1_5-sha256"`)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "root", got.KeyID)

	got, ok, err = ParseExpectSignature("sig")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Empty(t, got.KeyID)

	_, ok, err = ParseExpectSignature("")
	require.NoError(t, err)
	assert.False(t, ok)

	_, _, err = ParseExpectSignature(`sig, keyid="main", alg="ecdsa-p256"`)
	require.ErrorIs(t, err, ErrUnsupportedAlg)

	for _, bad := range []string{`sig, keyid=main`, `sig, keyid="ma in"`, `keyid="x`, `sig,`, `SIG`, `sig, keyid=?1`, `sig; =`, `sig, keyid="` + string(make([]byte, 2000)) + `"`} {
		_, _, err := ParseExpectSignature(bad)
		assert.ErrorIs(t, err, ErrBadExpectSignature, bad)
	}
}

func TestParseSFVDictionary(t *testing.T) {
	t.Parallel()
	d, err := ParseSFVDictionary(`sig="YWJj+/==", keyid="k\"1", alg=token/x, n=-42, b=?0;param=1`)
	require.NoError(t, err)
	assert.Equal(t, SFItem{Kind: "string", Str: "YWJj+/=="}, d["sig"])
	assert.Equal(t, `k"1`, d["keyid"].Str)
	assert.Equal(t, SFItem{Kind: "token", Str: "token/x"}, d["alg"])
	assert.Equal(t, int64(-42), d["n"].Int)
	assert.Equal(t, SFItem{Kind: "boolean", Bool: false}, d["b"])
	_, err = ParseSFVDictionary(`a=(1 2)`)
	require.Error(t, err, "inner lists are not used by these headers")
}

func TestSignVerifiesLikeTheClient(t *testing.T) {
	t.Parallel()
	key := testKey(t, 2048)
	kr := NewKeyringFromKeys("main", map[string]*rsa.PrivateKey{"main": key})
	body := []byte(`{"type":"noUpdateAvailable"}`)
	h, err := kr.Sign(context.Background(), "main", body)
	require.NoError(t, err)

	// Android SignatureHeaderInfo.parseSignatureHeader: sig a StringItem,
	// keyid a StringItem, alg a StringItem.
	d, err := ParseSFVDictionary(h)
	require.NoError(t, err)
	require.Equal(t, "string", d["sig"].Kind)
	assert.Equal(t, SFItem{Kind: "string", Str: "main"}, d["keyid"])
	assert.Equal(t, SFItem{Kind: "string", Str: AlgRSASHA256}, d["alg"])
	sig, err := base64.StdEncoding.DecodeString(d["sig"].Str) // Base64.DEFAULT / Data(base64Encoded:)
	require.NoError(t, err)
	sum := sha256.Sum256(body)
	require.NoError(t, rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig))

	_, err = kr.Sign(context.Background(), "other", body)
	require.ErrorIs(t, err, ErrUnknownKey)
	var empty *Keyring
	assert.False(t, empty.Configured())
}

func TestParseKeySpec(t *testing.T) {
	dir := t.TempDir()
	key := testKey(t, 2048)
	keyPath := filepath.Join(dir, "k.pem")
	require.NoError(t, os.WriteFile(keyPath, pkcs1PEM(key), 0o600))
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	t.Setenv("FC_TEST_APP_UPDATE_KEY", `"`+string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))+`"`)

	refs := fakeRefs{"aws-sm://prod/app-updates#old": string(pkcs1PEM(key))}
	kr, err := ParseKeySpec(" main=file:"+keyPath+", env1=env:FC_TEST_APP_UPDATE_KEY ,old=aws-sm://prod/app-updates#old", "", refs)
	require.NoError(t, err)
	assert.True(t, kr.Configured())
	assert.Equal(t, "main", kr.DefaultKeyID())
	require.NoError(t, kr.Validate(context.Background()))
	for _, id := range []string{"main", "env1", "old"} {
		pub, err := kr.PublicKey(context.Background(), id)
		require.NoError(t, err, id)
		assert.True(t, pub.Equal(&key.PublicKey), id)
	}

	kr, err = ParseKeySpec("a=file:"+keyPath+",b=file:"+keyPath, "b", nil)
	require.NoError(t, err)
	assert.Equal(t, "b", kr.DefaultKeyID())

	for _, bad := range []string{"main", "=file:/x", "main=", "main=/plain/path", "a b=file:/x", "a=file:/x,a=file:/y", "main=-----BEGIN RSA PRIVATE KEY-----"} {
		_, err := ParseKeySpec(bad, "", nil)
		assert.Error(t, err, bad)
		assert.NotContains(t, errString(err), "BEGIN RSA PRIVATE KEY", "an error never echoes key material")
	}
	_, err = ParseKeySpec("a=file:/x", "b", nil)
	require.Error(t, err, "the default must be in the keyring")

	kr, err = ParseKeySpec("", "", nil)
	require.NoError(t, err)
	assert.False(t, kr.Configured())
	_, err = kr.Sign(context.Background(), "main", nil)
	require.ErrorIs(t, err, ErrNoKeys)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestParsePrivateKeyPEMRefusesSmallKeys(t *testing.T) {
	t.Parallel()
	_, err := ParsePrivateKeyPEM(pkcs1PEM(testKey(t, 1024)))
	require.Error(t, err)
	_, err = ParsePrivateKeyPEM([]byte("not pem"))
	require.Error(t, err)
}

// A rotated value behind a reference is picked up; an unreadable one fails.
func TestKeyringReferenceRotation(t *testing.T) {
	t.Parallel()
	k1, k2 := testKey(t, 2048), testKey(t, 2048)
	refs := fakeRefs{"aws-ps:///updates/key": string(pkcs1PEM(k1))}
	kr, err := ParseKeySpec("main=aws-ps:///updates/key", "", refs)
	require.NoError(t, err)
	pub, err := kr.PublicKey(context.Background(), "main")
	require.NoError(t, err)
	assert.True(t, pub.Equal(&k1.PublicKey))
	refs["aws-ps:///updates/key"] = string(pkcs1PEM(k2))
	pub, err = kr.PublicKey(context.Background(), "main")
	require.NoError(t, err)
	assert.True(t, pub.Equal(&k2.PublicKey))
	delete(refs, "aws-ps:///updates/key")
	_, err = kr.PublicKey(context.Background(), "main")
	require.Error(t, err)
}

type fakeRefs map[string]string

func (f fakeRefs) Resolve(_ context.Context, ref string, _ secretref.Owner) (string, error) {
	if v, ok := f[ref]; ok {
		return v, nil
	}
	return "", errors.New("not found")
}

// The certificate is what @expo/code-signing-certificates makes and both
// clients accept: self-signed, key usage digital signature, extended key usage
// code signing, both critical.
func TestCodeSigningCertificate(t *testing.T) {
	t.Parallel()
	key := testKey(t, 2048)
	now := time.Now().UTC()
	certPEM, err := NewCodeSigningCertificate(key, CertificateOptions{CommonName: "acme driver-app updates", NotBefore: now.Add(-time.Hour), NotAfter: now.AddDate(10, 0, 0)})
	require.NoError(t, err)
	cert, err := CheckCodeSigningCertificate(certPEM, &key.PublicKey, now)
	require.NoError(t, err)
	assert.Equal(t, x509.SHA256WithRSA, cert.SignatureAlgorithm)
	assert.Equal(t, cert.Subject.String(), cert.Issuer.String())
	assert.False(t, cert.IsCA)
	var kuCritical, ekuCritical bool
	for _, e := range cert.Extensions {
		switch e.Id.String() {
		case "2.5.29.15":
			kuCritical = e.Critical
		case "2.5.29.37":
			ekuCritical = e.Critical
		case "1.2.840.113556.1.8000.2554.43437.254.128.102.157.7894389.20439.2.1":
			t.Fatal("no Expo project-information extension: the client would compare an EAS project id")
		}
	}
	assert.True(t, kuCritical)
	assert.True(t, ekuCritical)
	assert.Equal(t, []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning}, cert.ExtKeyUsage)

	_, err = CheckCodeSigningCertificate(certPEM, &key.PublicKey, now.AddDate(11, 0, 0))
	require.Error(t, err, "expired")
	_, err = CheckCodeSigningCertificate(certPEM, &key.PublicKey, now.Add(-2*time.Hour))
	require.Error(t, err, "not yet valid")
	_, err = CheckCodeSigningCertificate(certPEM, &testKey(t, 2048).PublicKey, now)
	require.Error(t, err, "another key")
}
