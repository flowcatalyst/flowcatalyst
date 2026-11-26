// Package attestationtest builds Android key attestation chains for tests: a
// self-signed root, an intermediate and a leaf certifying a given key with a
// key description extension. Test code only.
package attestationtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"time"
)

// Description is what the leaf's key description says.
type Description struct {
	Version          int
	AttestationLevel int // 0 software, 1 TEE, 2 StrongBox
	KeyLevel         int
	Challenge        []byte
	// Hardware is the hardware-enforced list; Software the software one.
	Hardware, Software AuthList
}

// AuthList is the subset of an AuthorizationList the platform reads, plus an
// unknown tag to prove it is skipped.
type AuthList struct {
	Purposes       []int
	Algorithm      *int
	ECCurve        *int
	NoAuthRequired bool
	UserAuthType   *int
	AuthTimeout    *int
	// Unknown adds tag 718 (vendorPatchLevel) the parser must skip.
	Unknown bool
}

// Int is a pointer to v.
//
//go:fix inline
func Int(v int) *int { return new(v) }

// StrongBiometric is the description of a key made the way
// docs/field-unlock-keys-spec.md section 12 says, in a TEE.
func StrongBiometric(challenge []byte) Description {
	return Description{
		Version: 200, AttestationLevel: 1, KeyLevel: 1, Challenge: challenge,
		Hardware: AuthList{
			Purposes: []int{2}, Algorithm: new(3), ECCurve: new(1), UserAuthType: new(2), Unknown: true,
		},
	}
}

// Chain is a built chain.
type Chain struct {
	// Base64 is the chain as a library sends it: base64 DER, leaf first.
	Base64 []string
	// RootPEM is the root certificate, for a roots file.
	RootPEM []byte
	Root    *x509.Certificate
	// Certs are the parsed certificates, leaf first.
	Certs []*x509.Certificate
}

// Options shape a chain like the real ones (docs/field-unlock-keys-spec.md
// section 6.2): root -> intermediate CA -> attestation key certificate -> leaf.
type Options struct {
	// Factory names a SERIALNUMBER in the intermediate's subject, as factory
	// chains do (and so their expired certificates are still trusted).
	Factory bool
	// AttestNotCA gives the attestation key certificate (the leaf's issuer) no
	// CA flag and digitalSignature only, as Pixel 2 and Sony factory chains do.
	AttestNotCA bool
	// AttestNotBefore / AttestNotAfter override the attestation key
	// certificate's validity (default 1970 to 2100).
	AttestNotBefore, AttestNotAfter time.Time
	// AttestKey puts a key description on the attestation key certificate (the
	// app's own attestation key).
	AttestKey *Description
	// InterPathLen sets the intermediate's path length (-1 = none).
	InterPathLen int
	// InterNotCA makes the intermediate a non-CA.
	InterNotCA bool
}

// Build makes a chain shaped as Options' zero value says (an RKP-style chain),
// the leaf certifying key with desc.
func Build(key *ecdsa.PublicKey, desc Description) Chain {
	return BuildWith(key, desc, Options{InterPathLen: -1})
}

// BuildWith makes root -> intermediate -> attestation key -> leaf.
func BuildWith(key *ecdsa.PublicKey, desc Description, o Options) Chain {
	far := time.Unix(4102444800, 0)
	rootKey, interKey, attestKey := mustKey(), mustKey(), mustKey()
	root := sign(&x509.Certificate{
		SerialNumber: big.NewInt(0x1001), Subject: pkix.Name{CommonName: "Test Attestation Root"},
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		NotBefore: time.Unix(0, 0), NotAfter: far,
	}, nil, &rootKey.PublicKey, rootKey)
	interSubject := pkix.Name{CommonName: "Test Attestation Intermediate"}
	if o.Factory {
		interSubject = pkix.Name{SerialNumber: "87f4514475ba0a2b"}
	}
	inter := &x509.Certificate{
		SerialNumber: big.NewInt(0x2002), Subject: interSubject,
		IsCA: !o.InterNotCA, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
		NotBefore: time.Unix(0, 0), NotAfter: far, MaxPathLen: o.InterPathLen, MaxPathLenZero: o.InterPathLen == 0,
	}
	interCert := sign(inter, root, &interKey.PublicKey, rootKey)
	attest := &x509.Certificate{
		SerialNumber: big.NewInt(0x3003), Subject: pkix.Name{CommonName: "Test Attestation Key", Organization: []string{"TEE"}},
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign, MaxPathLen: -1,
		NotBefore: time.Unix(0, 0), NotAfter: far,
	}
	if o.AttestNotCA {
		attest.IsCA, attest.KeyUsage = false, x509.KeyUsageDigitalSignature
	}
	if !o.AttestNotBefore.IsZero() {
		attest.NotBefore = o.AttestNotBefore
	}
	if !o.AttestNotAfter.IsZero() {
		attest.NotAfter = o.AttestNotAfter
	}
	if o.AttestKey != nil {
		attest.ExtraExtensions = []pkix.Extension{descriptionExtension(*o.AttestKey)}
	}
	attestCert := sign(attest, interCert, &attestKey.PublicKey, interKey)
	leaf := sign(&x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Android Keystore Key"},
		KeyUsage:  x509.KeyUsageDigitalSignature,
		NotBefore: time.Unix(0, 0), NotAfter: far,
		ExtraExtensions: []pkix.Extension{descriptionExtension(desc)},
	}, attestCert, key, attestKey)
	certs := []*x509.Certificate{leaf, attestCert, interCert, root}
	out := Chain{
		RootPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw}),
		Root:    root, Certs: certs,
	}
	for _, c := range certs {
		out.Base64 = append(out.Base64, base64.StdEncoding.EncodeToString(c.Raw))
	}
	return out
}

func descriptionExtension(d Description) pkix.Extension {
	ext, err := asn1.Marshal(rawDescription(d))
	if err != nil {
		panic(err)
	}
	return pkix.Extension{Id: asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}, Value: ext}
}

func mustKey() *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	return k
}

func sign(tmpl, parent *x509.Certificate, pub *ecdsa.PublicKey, signer *ecdsa.PrivateKey) *x509.Certificate {
	if parent == nil {
		parent = tmpl
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, pub, signer)
	if err != nil {
		panic(err)
	}
	c, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	return c
}

type rawDesc struct {
	AttestationVersion       int
	AttestationSecurityLevel asn1.Enumerated
	KeymasterVersion         int
	KeymasterSecurityLevel   asn1.Enumerated
	AttestationChallenge     []byte
	UniqueID                 []byte
	SoftwareEnforced         asn1.RawValue
	HardwareEnforced         asn1.RawValue
}

func rawDescription(d Description) rawDesc {
	return rawDesc{
		AttestationVersion: d.Version, AttestationSecurityLevel: asn1.Enumerated(d.AttestationLevel),
		KeymasterVersion: d.Version, KeymasterSecurityLevel: asn1.Enumerated(d.KeyLevel),
		AttestationChallenge: d.Challenge, UniqueID: []byte{},
		SoftwareEnforced: authList(d.Software), HardwareEnforced: authList(d.Hardware),
	}
}

func explicit(tag int, inner []byte) []byte {
	b, err := asn1.Marshal(asn1.RawValue{Class: asn1.ClassContextSpecific, Tag: tag, IsCompound: true, Bytes: inner})
	if err != nil {
		panic(err)
	}
	return b
}

func marshal(v any, params string) []byte {
	b, err := asn1.MarshalWithParams(v, params)
	if err != nil {
		panic(err)
	}
	return b
}

func authList(a AuthList) asn1.RawValue {
	var body []byte
	if a.Purposes != nil {
		body = append(body, explicit(1, marshal(a.Purposes, "set"))...)
	}
	if a.Algorithm != nil {
		body = append(body, explicit(2, marshal(*a.Algorithm, ""))...)
	}
	if a.ECCurve != nil {
		body = append(body, explicit(10, marshal(*a.ECCurve, ""))...)
	}
	if a.NoAuthRequired {
		body = append(body, explicit(503, marshal(asn1.NullRawValue, ""))...)
	}
	if a.UserAuthType != nil {
		body = append(body, explicit(504, marshal(*a.UserAuthType, ""))...)
	}
	if a.AuthTimeout != nil {
		body = append(body, explicit(505, marshal(*a.AuthTimeout, ""))...)
	}
	if a.Unknown {
		body = append(body, explicit(718, marshal(20261101, ""))...)
	}
	return asn1.RawValue{Class: asn1.ClassUniversal, Tag: asn1.TagSequence, IsCompound: true, Bytes: body}
}
