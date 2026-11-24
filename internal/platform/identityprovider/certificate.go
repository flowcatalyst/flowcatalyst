package identityprovider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // the SHA-1 thumbprint is what Entra's portal shows and what the x5t header carries; it is an identifier, not a security control.
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// CertificateStatus is a certificate's place in the rotation: at most one
// ACTIVE (signs assertions) and one NEXT (uploaded to the provider ahead of
// being made active) per identity provider.
type CertificateStatus string

const (
	CertificateActive CertificateStatus = "ACTIVE"
	CertificateNext   CertificateStatus = "NEXT"
)

// CertificateTable is the table certificates live in; it is also the prefix of
// the AAD their private keys are sealed with.
const CertificateTable = "oauth_identity_provider_certificates"

// certificateBits is the RSA key size of a generated certificate.
const certificateBits = 3072

// certificateLifetimeMonths is how long a generated certificate is valid.
const certificateLifetimeMonths = 24

// Certificate is a platform-generated key pair for authenticating an OIDC
// identity provider's token requests with a client assertion. The private key
// never leaves the platform: PrivateKeyRef is a PKCS#8 PEM sealed with the app
// key (EncryptBound), bound to the row by CertificateAAD.
type Certificate struct {
	ID                 string
	IdentityProviderID string
	Status             CertificateStatus
	PrivateKeyRef      string
	CertificatePEM     string
	// ThumbprintSHA1 is the upper-case hex SHA-1 of the DER certificate (what
	// Entra's portal shows); ThumbprintSHA256 likewise for SHA-256.
	ThumbprintSHA1   string
	ThumbprintSHA256 string
	NotBefore        time.Time
	NotAfter         time.Time
	CreatedAt        time.Time
}

// IDStr satisfies usecase.HasID.
func (c Certificate) IDStr() string { return c.ID }

// CertificateAAD is the associated data the private key of certificate id is
// sealed with: a stolen ciphertext cannot be replayed into another row.
func CertificateAAD(id string) []byte { return []byte(CertificateTable + ":" + id) }

// ErrCertificateEncryptionNotConfigured is returned when a certificate is
// generated or opened without an encryption service.
var ErrCertificateEncryptionNotConfigured = errors.New("encryption is not configured (set FLOWCATALYST_APP_KEY)")

// GenerateCertificate creates a self-signed RSA 3072 certificate for the
// provider (subject CN=FlowCatalyst <code>, random serial, valid from five
// minutes ago for 24 months, key usage digital signature) and seals its
// private key with enc. It uses a freshly generated key, never the platform's
// JWT signing key. The returned certificate has no status; the caller sets it.
func GenerateCertificate(enc *encryption.Service, idpID, idpCode string, now time.Time) (*Certificate, error) {
	if enc == nil {
		return nil, ErrCertificateEncryptionNotConfigured
	}
	key, err := rsa.GenerateKey(rand.Reader, certificateBits)
	if err != nil {
		return nil, fmt.Errorf("generate certificate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	now = now.UTC()
	notBefore := now.Add(-5 * time.Minute)
	notAfter := now.AddDate(0, certificateLifetimeMonths, 0)
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "FlowCatalyst " + idpCode},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal certificate key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})
	id := tsid.Generate(tsid.IdentityProviderCertificate)
	sealed, err := enc.EncryptBound(string(keyPEM), CertificateAAD(id))
	if err != nil {
		return nil, fmt.Errorf("seal certificate key: %w", err)
	}
	s1 := sha1.Sum(der) //nolint:gosec // see the import
	s256 := sha256.Sum256(der)
	return &Certificate{
		ID:                 id,
		IdentityProviderID: idpID,
		PrivateKeyRef:      sealed,
		CertificatePEM:     string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		ThumbprintSHA1:     strings.ToUpper(hex.EncodeToString(s1[:])),
		ThumbprintSHA256:   strings.ToUpper(hex.EncodeToString(s256[:])),
		NotBefore:          notBefore,
		NotAfter:           notAfter,
		CreatedAt:          now,
	}, nil
}

// DER returns the DER-encoded X.509 certificate (what a ".cer" download holds).
func (c *Certificate) DER() ([]byte, error) {
	block, _ := pem.Decode([]byte(c.CertificatePEM))
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("certificate %s: stored PEM is not a certificate", c.ID)
	}
	return block.Bytes, nil
}

// PrivateKey opens the sealed private key. The error never carries key
// material.
func (c *Certificate) PrivateKey(enc *encryption.Service) (*rsa.PrivateKey, error) {
	if enc == nil {
		return nil, ErrCertificateEncryptionNotConfigured
	}
	pt, bound, err := enc.DecryptBound(c.PrivateKeyRef, CertificateAAD(c.ID))
	if err != nil {
		return nil, fmt.Errorf("decrypt certificate key %s: %w", c.ID, err)
	}
	if !bound {
		return nil, fmt.Errorf("certificate key %s is not bound to its row", c.ID)
	}
	block, _ := pem.Decode([]byte(pt))
	if block == nil {
		return nil, fmt.Errorf("certificate key %s: stored key is not PEM", c.ID)
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("certificate key %s: parse: %w", c.ID, err)
	}
	rk, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("certificate key %s: not an RSA key", c.ID)
	}
	return rk, nil
}

// ExpiresWithin reports whether the certificate's validity ends within d of now.
func (c *Certificate) ExpiresWithin(now time.Time, d time.Duration) bool {
	return c.NotAfter.Before(now.Add(d))
}
