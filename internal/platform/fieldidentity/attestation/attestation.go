// Package attestation checks the key attestation a phone sends with an unlock
// key (docs/field-unlock-keys-spec.md section 6). Evaluate is pure: it reads the
// certificate chain, checks its shape, signatures, names, issuers and dates the
// way Google's own verifier does (github.com/android/keyattestation,
// KeyAttestationCertPathValidator), compares its root with the configured roots,
// checks the certificates against a revocation list the caller supplies, and
// reads the Android key description of the leaf. What the platform enforces is
// the policy's call ([Summary.MeetsRequirement]); Evaluate says what the chain
// shows. The revocation list is fetched in the background by [RunStatusFetcher]
// (status.go), never on a request.
package attestation

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Format names what kind of attestation was sent.
type Format string

const (
	// FormatAndroidKey is an Android Keystore key attestation chain.
	FormatAndroidKey Format = "ANDROID_KEY"
	// FormatNone is no attestation (iOS, or a phone that sent none).
	FormatNone Format = "NONE"
)

// Verdict is how far the chain could be checked.
type Verdict string

const (
	// VerdictNone means no chain was sent.
	VerdictNone Verdict = "NONE"
	// VerdictMalformed means the chain cannot be trusted whatever its root: a
	// certificate does not parse, a signature or a name does not chain, the root
	// is not self-signed, a certificate issues another it may not issue, a key
	// description appears above the leaf (a chain extended to fake an
	// attestation), or the leaf does not certify the registered key.
	VerdictMalformed Verdict = "MALFORMED"
	// VerdictUntrusted means the chain is well formed but its root is not one
	// of the configured roots.
	VerdictUntrusted Verdict = "UNTRUSTED"
	// VerdictTrusted means the chain is well formed and its root is configured.
	VerdictTrusted Verdict = "TRUSTED"
)

// SecurityLevel is where Android says a key lives.
type SecurityLevel string

const (
	LevelSoftware  SecurityLevel = "SOFTWARE"
	LevelTEE       SecurityLevel = "TEE"
	LevelStrongBox SecurityLevel = "STRONGBOX"
)

// Provisioning is how the chain's attestation key reached the phone.
type Provisioning string

const (
	// ProvisioningFactory: the certificate under the root names a SERIALNUMBER
	// (a key written at the factory; such certificates may have expired and are
	// still trusted, as Google's verifier trusts them).
	ProvisioningFactory Provisioning = "FACTORY"
	// ProvisioningRemote: anything else under a trusted root (Remote Key
	// Provisioning's short-lived certificates, whose dates are always checked).
	ProvisioningRemote Provisioning = "REMOTE"
)

// DatesState says whether the chain's certificates were within their validity
// when the key was registered.
type DatesState string

const (
	DatesValid   DatesState = "VALID"
	DatesInvalid DatesState = "INVALID"
)

// RevocationState says what the revocation list said about the chain.
type RevocationState string

const (
	// RevocationClear: a usable list was checked and names no certificate.
	RevocationClear RevocationState = "CLEAR"
	// RevocationRevoked / RevocationSuspended: the list names a certificate.
	RevocationRevoked   RevocationState = "REVOKED"
	RevocationSuspended RevocationState = "SUSPENDED"
	// RevocationOff: the platform's check is switched off
	// (FC_FIELD_ATTESTATION_STATUS_URL empty).
	RevocationOff RevocationState = "NOT_CHECKED_OFF"
	// RevocationUnavailable: no usable list (never fetched, or too old).
	RevocationUnavailable RevocationState = "NOT_CHECKED_UNAVAILABLE"
)

// Limits on what a registration may send.
const (
	MaxCertificates   = 10
	MaxCertificateLen = 16 << 10
)

// keyDescriptionOID is the Android key attestation extension.
var keyDescriptionOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}

// Authorization list tags and values (Android KeyMint).
const (
	tagPurpose        = 1
	tagAlgorithm      = 2
	tagECCurve        = 10
	tagNoAuthRequired = 503
	tagUserAuthType   = 504
	tagAuthTimeout    = 505

	purposeSign        = 2
	purposeAttestKey   = 7
	algorithmEC        = 3
	curveP256          = 1
	userAuthBiometric  = 2
	contextSpecificTag = asn1.ClassContextSpecific
)

// Summary is what the platform records about an unlock key's attestation.
type Summary struct {
	Format  Format  `json:"format"`
	Verdict Verdict `json:"verdict"`
	// Problem says why a chain is MALFORMED (for the console; never key
	// material).
	Problem                  string        `json:"problem,omitempty"`
	SecurityLevel            SecurityLevel `json:"securityLevel,omitempty"`
	AttestationSecurityLevel SecurityLevel `json:"attestationSecurityLevel,omitempty"`
	AttestationVersion       int           `json:"attestationVersion,omitempty"`
	// ChallengeMatches: the description's challenge is [Challenge] of the
	// registering session.
	ChallengeMatches bool `json:"challengeMatches"`
	// AuthPerUse: in the hardware-enforced list, user authentication is
	// required (no noAuthRequired, a userAuthType) with no timeout.
	AuthPerUse bool `json:"authPerUse"`
	// BiometricOnly: the hardware-enforced userAuthType is exactly biometric.
	BiometricOnly bool `json:"biometricOnly"`
	// SignP256: the hardware-enforced purpose includes SIGN, the algorithm is
	// EC and the curve P-256.
	SignP256 bool `json:"signP256"`
	// RootFingerprint is the hex SHA-256 of the root's SubjectPublicKeyInfo.
	RootFingerprint string       `json:"rootFingerprint,omitempty"`
	Provisioning    Provisioning `json:"provisioning,omitempty"`
	// Dates and DatesProblem: the certificates' validity at CheckedAt (the
	// registration), checked once and never again.
	Dates        DatesState `json:"dates,omitempty"`
	DatesProblem string     `json:"datesProblem,omitempty"`
	// Revocation and RevokedSerial: what the revocation list said at CheckedAt.
	Revocation    RevocationState `json:"revocation,omitempty"`
	RevokedSerial string          `json:"revokedSerial,omitempty"`
	// Serials are the chain's certificate serial numbers above the leaf, as the
	// revocation list writes them (lowercase hex), kept so a list refreshed
	// later can still name the key's chain. Not on the wire.
	Serials   []string   `json:"serials,omitempty"`
	CheckedAt *time.Time `json:"checkedAt,omitempty"`
}

// None is the summary of a registration that sent no chain.
func None() Summary { return Summary{Format: FormatNone, Verdict: VerdictNone} }

// MeetsRequirement reports whether the summary satisfies the policy's
// REQUIRED rule: a trusted, well-formed chain, every certificate within its
// validity when registered (expired factory certificates excepted) and none on
// the revocation list (or the platform's list check switched off), attesting
// from secure hardware a key in secure hardware that signs with P-256 only
// after a strong biometric, for this registration.
func (s Summary) MeetsRequirement() bool {
	return s.Verdict == VerdictTrusted && hardwareLevel(s.SecurityLevel) && hardwareLevel(s.AttestationSecurityLevel) &&
		s.ChallengeMatches && s.AuthPerUse && s.BiometricOnly && s.SignP256 && s.Dates == DatesValid &&
		(s.Revocation == RevocationClear || s.Revocation == RevocationOff)
}

// Challenge is the attestation challenge a library sets when it makes an
// unlock key: SHA-256 of the prefix, the session (sid), the worker and the
// device, newline-separated (docs/field-unlock-keys-spec.md section 3).
func Challenge(sessionID, workerID, deviceID string) []byte {
	sum := sha256.Sum256([]byte("fc-unlock-key-attestation:v1\n" + sessionID + "\n" + workerID + "\n" + deviceID))
	return sum[:]
}

// Roots is the set of trusted attestation roots, by the SHA-256 of their
// SubjectPublicKeyInfo. The zero value (or nil) trusts nothing.
type Roots struct{ spki map[string]bool }

// ParseRootsPEM reads every CERTIFICATE block of a PEM file. A file with no
// certificate, or one that does not parse, is an error.
func ParseRootsPEM(data []byte) (*Roots, error) {
	r := &Roots{spki: map[string]bool{}}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("attestation root: %w", err)
		}
		r.spki[spkiFingerprint(cert)] = true
	}
	if len(r.spki) == 0 {
		return nil, errors.New("attestation roots: no certificate in the file")
	}
	return r, nil
}

// Configured reports whether any root is trusted.
func (r *Roots) Configured() bool { return r != nil && len(r.spki) > 0 }

func (r *Roots) trusts(fingerprint string) bool { return r != nil && r.spki[fingerprint] }

func spkiFingerprint(c *x509.Certificate) string {
	sum := sha256.Sum256(c.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

// Options are what Evaluate checks the chain against.
type Options struct {
	Roots *Roots
	// Now is the registration time (dates are checked once, at registration).
	Now time.Time
	// Status is the revocation list; StatusOff says the platform's check is
	// switched off. Status nil and StatusOff false is "no usable list".
	Status    *StatusList
	StatusOff bool
}

// Evaluate checks a chain (base64 DER certificates, leaf first) for key and
// the expected challenge. An empty chain is [None]. It never fails: what
// cannot be read is a MALFORMED verdict with its problem.
//
// The shape rules follow Google's verifier and the real chains it is tested on
// (factory chains of Pixel 2 to 8 and a Sony phone, Remote Key Provisioning
// chains of Pixel 9 to 11): the key description is on the leaf and on no other
// certificate (Google: "only the first occurrence of the extension in the chain
// can be trusted"), except an attestation key of the app's own (hardware
// purpose ATTEST_KEY only) directly above the leaf; each certificate's issuer
// is the next one's subject; every certificate above the leaf's issuer is a CA
// allowed to sign certificates, within any path length it states. The leaf's
// issuer is exempt from the CA rule: real factory chains (Pixel 2, Sony Xperia
// 10 III) carry a batch certificate without the CA flag.
func Evaluate(chain []string, key *ecdsa.PublicKey, challenge []byte, o Options) Summary {
	if len(chain) == 0 {
		return None()
	}
	out := Summary{Format: FormatAndroidKey, Verdict: VerdictMalformed}
	if !o.Now.IsZero() {
		at := o.Now.UTC()
		out.CheckedAt = &at
	}
	malformed := func(problem string) Summary { out.Problem = problem; return out }
	if len(chain) > MaxCertificates {
		return malformed("too many certificates")
	}
	if len(chain) < 2 {
		return malformed("the chain has no issuer for the leaf")
	}
	certs := make([]*x509.Certificate, 0, len(chain))
	for _, enc := range chain {
		der, err := base64.StdEncoding.DecodeString(enc)
		if err != nil || len(der) == 0 || len(der) > MaxCertificateLen {
			return malformed("a certificate is not base64 DER of an acceptable size")
		}
		c, err := x509.ParseCertificate(der)
		if err != nil {
			return malformed("a certificate does not parse")
		}
		certs = append(certs, c)
	}
	for i := 0; i+1 < len(certs); i++ {
		child, parent := certs[i], certs[i+1]
		if !bytes.Equal(child.RawIssuer, parent.RawSubject) {
			return malformed("a certificate's issuer is not the next certificate's subject")
		}
		if err := parent.CheckSignature(child.SignatureAlgorithm, child.RawTBSCertificate, child.Signature); err != nil {
			return malformed("the chain does not verify")
		}
	}
	root := certs[len(certs)-1]
	if !bytes.Equal(root.RawIssuer, root.RawSubject) {
		return malformed("the root is not self-issued")
	}
	if err := root.CheckSignature(root.SignatureAlgorithm, root.RawTBSCertificate, root.Signature); err != nil {
		return malformed("the root is not self-signed")
	}
	if problem := issuersMayIssue(certs); problem != "" {
		return malformed(problem)
	}
	out.RootFingerprint = spkiFingerprint(root)
	leaf := certs[0]
	leafKey, ok := leaf.PublicKey.(*ecdsa.PublicKey)
	if !ok || key == nil || !leafKey.Equal(key) {
		return malformed("the leaf does not certify the unlock key")
	}
	ext := keyDescriptionOf(leaf)
	if ext == nil {
		return malformed("the leaf has no key description")
	}
	for i := 1; i < len(certs); i++ {
		above := keyDescriptionOf(certs[i])
		if above == nil {
			continue
		}
		// A key description above the leaf is a chain extended to fake an
		// attestation, unless it is the app's own attestation key (one the
		// hardware uses only to sign attestations) right above the leaf.
		if i != 1 {
			return malformed("a key description appears above the leaf's issuer")
		}
		d, err := parseKeyDescription(above)
		if err != nil || !slices.Equal(d.hardware.purposes, []int{purposeAttestKey}) {
			return malformed("a certificate above the leaf carries a key description and is not an attestation key")
		}
		// The attestation key must itself live in the secure hardware and have
		// been attested there: a software attestation key could sign any leaf.
		if !hardwareLevel(d.attestationLevel) || !hardwareLevel(d.keyLevel) {
			return malformed("the attestation key above the leaf is not in TEE or StrongBox")
		}
	}
	desc, err := parseKeyDescription(ext)
	if err != nil {
		return malformed("the key description does not parse")
	}
	out.AttestationVersion = desc.version
	out.AttestationSecurityLevel = desc.attestationLevel
	out.SecurityLevel = desc.keyLevel
	out.ChallengeMatches = len(challenge) > 0 && bytes.Equal(desc.challenge, challenge)
	hw, sw := desc.hardware, desc.software
	out.AuthPerUse = !hw.noAuthRequired && !sw.noAuthRequired && hw.userAuthType != nil &&
		(hw.authTimeout == nil || *hw.authTimeout == 0)
	out.BiometricOnly = hw.userAuthType != nil && *hw.userAuthType == userAuthBiometric
	out.SignP256 = slices.Contains(hw.purposes, purposeSign) &&
		hw.algorithm != nil && *hw.algorithm == algorithmEC && hw.ecCurve != nil && *hw.ecCurve == curveP256

	out.Provisioning = ProvisioningRemote
	if certs[len(certs)-2].Subject.SerialNumber != "" {
		out.Provisioning = ProvisioningFactory
	}
	out.Dates, out.DatesProblem = checkDates(certs, out.Provisioning, o.Now)
	for _, c := range certs[1:] {
		out.Serials = append(out.Serials, c.SerialNumber.Text(16))
	}
	switch {
	case o.StatusOff:
		out.Revocation = RevocationOff
	case o.Status == nil:
		out.Revocation = RevocationUnavailable
	default:
		out.Revocation = RevocationClear
		for _, serial := range out.Serials {
			if st, listed := o.Status.Status(serial); listed {
				out.Revocation, out.RevokedSerial = st, serial
				break
			}
		}
	}
	if roots := o.Roots; roots.trusts(out.RootFingerprint) {
		out.Verdict = VerdictTrusted
	} else {
		out.Verdict = VerdictUntrusted
	}
	return out
}

func keyDescriptionOf(c *x509.Certificate) []byte {
	for _, e := range c.Extensions {
		if e.Id.Equal(keyDescriptionOID) {
			return e.Value
		}
	}
	return nil
}

// issuersMayIssue checks that every certificate above the leaf's issuer is a
// CA allowed to sign certificates, and that no certificate's stated path
// length is exceeded by the certificates under it (the leaf excepted).
func issuersMayIssue(certs []*x509.Certificate) string {
	for i := 2; i < len(certs); i++ {
		c := certs[i]
		if !c.BasicConstraintsValid || !c.IsCA {
			return "a certificate that issues another is not a CA"
		}
		if c.KeyUsage != 0 && c.KeyUsage&x509.KeyUsageCertSign == 0 {
			return "a certificate that issues another may not sign certificates"
		}
	}
	for i := 1; i < len(certs); i++ {
		c := certs[i]
		if !c.BasicConstraintsValid || !c.IsCA || (c.MaxPathLen < 0) || (c.MaxPathLen == 0 && !c.MaxPathLenZero) {
			continue
		}
		// Intermediate certificates between c and the leaf (exclusive).
		if below := i - 1; below > c.MaxPathLen {
			return "a certificate's path length is exceeded"
		}
	}
	return ""
}

// checkDates is the validity rule of Google's verifier, at the registration:
// the leaf is not checked (its dates are set on the phone); the root is the
// trust anchor and is not checked (the 2016 Google root, still at the top of
// older phones' chains, expired in May 2026); every other certificate must have
// begun, and must not have expired unless the chain is factory-provisioned.
func checkDates(certs []*x509.Certificate, prov Provisioning, now time.Time) (DatesState, string) {
	if now.IsZero() {
		return DatesInvalid, "no time to check against"
	}
	for _, c := range certs[1 : len(certs)-1] {
		if now.Before(c.NotBefore) {
			return DatesInvalid, "a certificate is not yet valid"
		}
		if now.After(c.NotAfter) && prov != ProvisioningFactory {
			return DatesInvalid, "a certificate has expired"
		}
	}
	return DatesValid, ""
}

// hardwareLevel reports whether a security level is the secure hardware's.
func hardwareLevel(l SecurityLevel) bool { return l == LevelTEE || l == LevelStrongBox }

type keyDescription struct {
	version          int
	attestationLevel SecurityLevel
	keyLevel         SecurityLevel
	challenge        []byte
	software         authList
	hardware         authList
}

type rawKeyDescription struct {
	AttestationVersion       int
	AttestationSecurityLevel asn1.Enumerated
	KeymasterVersion         int
	KeymasterSecurityLevel   asn1.Enumerated
	AttestationChallenge     []byte
	UniqueID                 []byte
	SoftwareEnforced         asn1.RawValue
	HardwareEnforced         asn1.RawValue
}

func parseKeyDescription(der []byte) (keyDescription, error) {
	var raw rawKeyDescription
	if _, err := asn1.Unmarshal(der, &raw); err != nil {
		return keyDescription{}, err
	}
	sw, err := parseAuthList(raw.SoftwareEnforced)
	if err != nil {
		return keyDescription{}, err
	}
	hw, err := parseAuthList(raw.HardwareEnforced)
	if err != nil {
		return keyDescription{}, err
	}
	return keyDescription{
		version:          raw.AttestationVersion,
		attestationLevel: level(raw.AttestationSecurityLevel),
		keyLevel:         level(raw.KeymasterSecurityLevel),
		challenge:        raw.AttestationChallenge,
		software:         sw,
		hardware:         hw,
	}, nil
}

func level(e asn1.Enumerated) SecurityLevel {
	switch e {
	case 1:
		return LevelTEE
	case 2:
		return LevelStrongBox
	default:
		return LevelSoftware
	}
}

// authList is the part of an AuthorizationList the platform reads. Tags it
// does not read are skipped, so a newer KeyMint's list still parses.
type authList struct {
	purposes       []int
	algorithm      *int
	ecCurve        *int
	noAuthRequired bool
	userAuthType   *int
	authTimeout    *int
}

func parseAuthList(seq asn1.RawValue) (authList, error) {
	var out authList
	if seq.Class != asn1.ClassUniversal || seq.Tag != asn1.TagSequence {
		return out, errors.New("authorization list is not a sequence")
	}
	rest := seq.Bytes
	for len(rest) > 0 {
		var item asn1.RawValue
		var err error
		rest, err = asn1.Unmarshal(rest, &item)
		if err != nil {
			return out, err
		}
		if item.Class != contextSpecificTag {
			continue
		}
		switch item.Tag {
		case tagPurpose:
			var v []int
			if _, err := asn1.UnmarshalWithParams(item.Bytes, &v, "set"); err != nil {
				return out, err
			}
			out.purposes = v
		case tagAlgorithm:
			if out.algorithm, err = explicitInt(item.Bytes); err != nil {
				return out, err
			}
		case tagECCurve:
			if out.ecCurve, err = explicitInt(item.Bytes); err != nil {
				return out, err
			}
		case tagNoAuthRequired:
			out.noAuthRequired = true
		case tagUserAuthType:
			if out.userAuthType, err = explicitInt(item.Bytes); err != nil {
				return out, err
			}
		case tagAuthTimeout:
			if out.authTimeout, err = explicitInt(item.Bytes); err != nil {
				return out, err
			}
		}
	}
	return out, nil
}

func explicitInt(b []byte) (*int, error) {
	var v int
	if _, err := asn1.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// Shortfall says, in a phrase, the first reason the summary does not meet the
// REQUIRED rule ("" when it does): for the refusal's message and the console.
func (s Summary) Shortfall() string {
	secure := hardwareLevel
	switch {
	case s.Verdict == VerdictNone:
		return "no attestation was sent"
	case s.Verdict == VerdictMalformed:
		return "the attestation cannot be trusted: " + s.Problem
	case s.Verdict == VerdictUntrusted:
		return "the attestation's root is not a trusted root"
	case !secure(s.AttestationSecurityLevel):
		return "the attestation was not made by secure hardware"
	case !secure(s.SecurityLevel):
		return "the key is not in secure hardware"
	case !s.ChallengeMatches:
		return "the attestation was not made for this sign-in"
	case !s.AuthPerUse:
		return "the key does not need authentication for every use"
	case !s.BiometricOnly:
		return "the key accepts more than a strong biometric"
	case !s.SignP256:
		return "the key is not a P-256 signing key"
	case s.Dates != DatesValid:
		return "the certificate dates: " + s.DatesProblem
	case s.Revocation == RevocationRevoked || s.Revocation == RevocationSuspended:
		return "the attestation key status list marks a certificate of the chain " + string(s.Revocation)
	case s.Revocation == RevocationUnavailable:
		return "the attestation key status list is not available"
	}
	return ""
}

// ListedSerial reports the first of serials the list names, for a key
// registered before the list was refreshed.
func ListedSerial(l *StatusList, serials []string) (string, bool) {
	for _, s := range serials {
		if _, ok := l.Status(s); ok {
			return s, true
		}
	}
	return "", false
}
