package appupdate

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"
)

var (
	oidExtKeyUsage = asn1.ObjectIdentifier{2, 5, 29, 37}
	// oidCodeSigning is id-kp-codeSigning, the OID both clients look for
	// (CODE_SIGNING_OID "1.3.6.1.5.5.7.3.3").
	oidCodeSigning = asn1.ObjectIdentifier{1, 3, 6, 1, 5, 5, 7, 3, 3}
)

// CertificateOptions describe a code-signing certificate for expo-updates.
type CertificateOptions struct {
	CommonName string
	NotBefore  time.Time
	NotAfter   time.Time
}

// NewCodeSigningCertificate makes the certificate an app embeds
// (updates.codeSigningCertificate) for key: what
// @expo/code-signing-certificates' generateSelfSignedCodeSigningCertificate
// makes and both clients require (Android CertificateChain.isCodeSigningCertificate,
// iOS X509Certificate.isCodeSigningCertificate): self-signed (subject equals
// issuer, signed by its own key), SHA-256 with RSA, key usage digital
// signature (critical), extended key usage code signing (critical), and no
// Expo project-information extension, so the client compares no EAS project id
// or scope key. Returns the PEM.
func NewCodeSigningCertificate(key *rsa.PrivateKey, o CertificateOptions) ([]byte, error) {
	if o.CommonName == "" {
		return nil, errors.New("certificate common name is required")
	}
	if !o.NotAfter.After(o.NotBefore) {
		return nil, errors.New("certificate must end after it starts")
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 72))
	if err != nil {
		return nil, err
	}
	// Go writes extended key usage non-critical; Expo's tooling marks it
	// critical, so it is supplied as an extension of its own.
	eku, err := asn1.Marshal([]asn1.ObjectIdentifier{oidCodeSigning})
	if err != nil {
		return nil, err
	}
	name := pkix.Name{CommonName: o.CommonName}
	tmpl := &x509.Certificate{
		SerialNumber:       new(big.Int).Add(serial, big.NewInt(1)),
		Subject:            name,
		Issuer:             name,
		NotBefore:          o.NotBefore.UTC(),
		NotAfter:           o.NotAfter.UTC(),
		KeyUsage:           x509.KeyUsageDigitalSignature,
		ExtraExtensions:    []pkix.Extension{{Id: oidExtKeyUsage, Critical: true, Value: eku}},
		SignatureAlgorithm: x509.SHA256WithRSA,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), nil
}

// CheckCodeSigningCertificate applies the client's checks to a certificate
// PEM as an app embeds it (no chain): it parses, is within its validity at
// now, is self-signed and verifies under its own key, has key usage digital
// signature and extended key usage code signing, and (when key is not nil)
// holds key's public half. It is what `fcdev updates keygen` and the
// conformance test run before trusting a certificate.
func CheckCodeSigningCertificate(certPEM []byte, key *rsa.PublicKey, now time.Time) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, errors.New("no CERTIFICATE PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, err
	}
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return nil, fmt.Errorf("certificate is not valid at %s (valid %s to %s)", now.UTC().Format(time.RFC3339),
			cert.NotBefore.UTC().Format(time.RFC3339), cert.NotAfter.UTC().Format(time.RFC3339))
	}
	if cert.Subject.String() != cert.Issuer.String() {
		return nil, errors.New("certificate is not self-signed (subject differs from issuer)")
	}
	if err := cert.CheckSignatureFrom(cert); err != nil {
		// CheckSignatureFrom insists on CA basic constraints for a parent;
		// the client verifies the signature under the certificate's own key
		// (Android X509Certificate.verify(publicKey)).
		if err := cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature); err != nil {
			return nil, fmt.Errorf("certificate signature does not verify under its own key: %w", err)
		}
	}
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, errors.New("certificate lacks key usage digital signature")
	}
	codeSigning := false
	for _, u := range cert.ExtKeyUsage {
		codeSigning = codeSigning || u == x509.ExtKeyUsageCodeSigning
	}
	if !codeSigning {
		return nil, errors.New("certificate lacks extended key usage code signing")
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("certificate key is not RSA")
	}
	if key != nil && !pub.Equal(key) {
		return nil, errors.New("certificate is not for this key")
	}
	return cert, nil
}
