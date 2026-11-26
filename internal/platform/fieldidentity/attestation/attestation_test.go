package attestation_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/attestation/attestationtest"
)

var testNow = time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

func newKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return k
}

// trusting is Options that trust the chain's root, at testNow, with the
// revocation check switched off.
func trusting(t *testing.T, c attestationtest.Chain) attestation.Options {
	t.Helper()
	roots, err := attestation.ParseRootsPEM(c.RootPEM)
	require.NoError(t, err)
	return attestation.Options{Roots: roots, Now: testNow, StatusOff: true}
}

var challenge = attestation.Challenge("fws_1", "fwu_1", "fwd_1")

func TestEvaluate_NoChainIsNone(t *testing.T) {
	t.Parallel()
	s := attestation.Evaluate(nil, &newKey(t).PublicKey, nil, attestation.Options{})
	assert.Equal(t, attestation.None(), s)
	assert.False(t, s.MeetsRequirement())
	assert.Equal(t, "no attestation was sent", s.Shortfall())
}

func TestEvaluate_StrongBiometricKeyInTrustedChainMeetsTheRequirement(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	chain := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	require.Len(t, chain.Base64, 4, "root, intermediate, attestation key, leaf: the real shape")
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
	assert.Equal(t, attestation.FormatAndroidKey, s.Format)
	assert.Equal(t, attestation.VerdictTrusted, s.Verdict, s.Problem)
	assert.Equal(t, attestation.LevelTEE, s.SecurityLevel)
	assert.Equal(t, attestation.LevelTEE, s.AttestationSecurityLevel)
	assert.Equal(t, 200, s.AttestationVersion)
	assert.True(t, s.ChallengeMatches)
	assert.True(t, s.AuthPerUse)
	assert.True(t, s.BiometricOnly)
	assert.True(t, s.SignP256)
	assert.Equal(t, attestation.ProvisioningRemote, s.Provisioning)
	assert.Equal(t, attestation.DatesValid, s.Dates)
	assert.Equal(t, attestation.RevocationOff, s.Revocation)
	assert.Equal(t, []string{"3003", "2002", "1001"}, s.Serials, "the serials above the leaf, as the status list writes them")
	assert.Len(t, s.RootFingerprint, 64)
	assert.True(t, s.MeetsRequirement())
	assert.Empty(t, s.Shortfall())
}

func TestEvaluate_UntrustedRootIsParsedButDoesNotMeetTheRequirement(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	chain := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	other := attestationtest.Build(&newKey(t).PublicKey, attestationtest.StrongBiometric(challenge))
	o := trusting(t, other)
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.VerdictUntrusted, s.Verdict)
	assert.True(t, s.AuthPerUse)
	assert.False(t, s.MeetsRequirement())
	o.Roots = nil
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.VerdictUntrusted, s.Verdict, "no roots configured trusts nothing")
}

// The reviewer's forgery: an app key made in genuine hardware (purpose SIGN,
// no user authentication) has a genuine chain to a trusted root, and signs a
// fabricated leaf for a software key that claims TEE, biometric per use, P-256
// and the right challenge. A key description above the leaf refuses it, as
// Google's verifier does ("only the first occurrence of the extension in the
// chain can be trusted").
func TestEvaluate_ForgedLeafSignedByAnAttestedAppKeyIsRefused(t *testing.T) {
	t.Parallel()
	k1 := newKey(t)
	genuine := attestationtest.Build(&k1.PublicKey, attestationtest.Description{
		Version: 200, AttestationLevel: 1, KeyLevel: 1, Challenge: []byte("anything"),
		Hardware: attestationtest.AuthList{Purposes: []int{2}, Algorithm: new(3), ECCurve: new(1), NoAuthRequired: true},
	})
	o := trusting(t, genuine)
	k2 := newKey(t) // a software key the attacker holds
	donor := attestationtest.Build(&k2.PublicKey, attestationtest.StrongBiometric(challenge))
	var ext pkix.Extension
	for _, e := range donor.Certs[0].Extensions {
		if e.Id.Equal(asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}) {
			ext = e
		}
	}
	k1Cert := genuine.Certs[0]
	require.False(t, k1Cert.IsCA)
	fakeDER, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(9), Subject: pkix.Name{CommonName: "Android Keystore Key"},
		NotBefore: time.Unix(0, 0), NotAfter: time.Unix(4102444800, 0), ExtraExtensions: []pkix.Extension{ext},
	}, k1Cert, &k2.PublicKey, k1)
	require.NoError(t, err)
	chain := append([]string{base64.StdEncoding.EncodeToString(fakeDER)}, genuine.Base64...)

	s := attestation.Evaluate(chain, &k2.PublicKey, challenge, o)
	assert.False(t, s.MeetsRequirement(), "FORGERY ACCEPTED: %+v", s)
	assert.Equal(t, attestation.VerdictMalformed, s.Verdict)
	assert.Contains(t, s.Problem, "key description")
}

func TestEvaluate_TheAppsOwnAttestationKeyMayCarryAKeyDescription(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	attestKey := attestationtest.Description{
		Version: 200, AttestationLevel: 1, KeyLevel: 1,
		Hardware: attestationtest.AuthList{Purposes: []int{7}, Algorithm: new(3), ECCurve: new(1), NoAuthRequired: true},
	}
	chain := attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge),
		attestationtest.Options{AttestKey: &attestKey, InterPathLen: -1})
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
	assert.Equal(t, attestation.VerdictTrusted, s.Verdict, s.Problem)
	assert.True(t, s.MeetsRequirement())

	strongBox := attestKey
	strongBox.AttestationLevel, strongBox.KeyLevel = 2, 2
	chain = attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge),
		attestationtest.Options{AttestKey: &strongBox, InterPathLen: -1})
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
	assert.Equal(t, attestation.VerdictTrusted, s.Verdict, "a StrongBox attestation key: %s", s.Problem)

	// The attestation key must itself be in, and attested by, the secure
	// hardware: a software one could sign any leaf.
	for name, levels := range map[string][2]int{
		"attested in software": {0, 1},
		"key in software":      {1, 0},
		"both in software":     {0, 0},
	} {
		soft := attestKey
		soft.AttestationLevel, soft.KeyLevel = levels[0], levels[1]
		chain = attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge),
			attestationtest.Options{AttestKey: &soft, InterPathLen: -1})
		s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
		assert.Equal(t, attestation.VerdictMalformed, s.Verdict, name)
		assert.Contains(t, s.Problem, "TEE or StrongBox", name)
		assert.False(t, s.MeetsRequirement(), name)
	}

	attestKey.Hardware.Purposes = []int{7, 2} // may also sign: not an attestation key only
	chain = attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge),
		attestationtest.Options{AttestKey: &attestKey, InterPathLen: -1})
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
	assert.Equal(t, attestation.VerdictMalformed, s.Verdict)
}

func TestEvaluate_IssuerRules(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		o    attestationtest.Options
		want attestation.Verdict
	}{
		// Pixel 2 and Sony Xperia 10 III factory chains: the batch certificate
		// that issues the leaf has no CA flag and digitalSignature only.
		"leaf's issuer not a CA, as real factory chains": {attestationtest.Options{AttestNotCA: true, Factory: true, InterPathLen: -1}, attestation.VerdictTrusted},
		"a higher issuer not a CA":                       {attestationtest.Options{InterNotCA: true, InterPathLen: -1}, attestation.VerdictMalformed},
		"path length 1 holds":                            {attestationtest.Options{InterPathLen: 1}, attestation.VerdictTrusted},
		"path length 0 exceeded":                         {attestationtest.Options{InterPathLen: 0}, attestation.VerdictMalformed},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := newKey(t)
			chain := attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge), tc.o)
			s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
			assert.Equal(t, tc.want, s.Verdict, s.Problem)
		})
	}
}

func TestEvaluate_Dates(t *testing.T) {
	t.Parallel()
	past, future := testNow.Add(-24*time.Hour), testNow.Add(24*time.Hour)
	cases := map[string]struct {
		o       attestationtest.Options
		want    attestation.DatesState
		problem string
	}{
		"remote, expired":         {attestationtest.Options{AttestNotAfter: past, InterPathLen: -1}, attestation.DatesInvalid, "expired"},
		"factory, expired":        {attestationtest.Options{AttestNotAfter: past, Factory: true, InterPathLen: -1}, attestation.DatesValid, ""},
		"remote, not yet valid":   {attestationtest.Options{AttestNotBefore: future, InterPathLen: -1}, attestation.DatesInvalid, "not yet valid"},
		"factory, not yet valid":  {attestationtest.Options{AttestNotBefore: future, Factory: true, InterPathLen: -1}, attestation.DatesInvalid, "not yet valid"},
		"within a short validity": {attestationtest.Options{AttestNotBefore: past, AttestNotAfter: future, InterPathLen: -1}, attestation.DatesValid, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := newKey(t)
			chain := attestationtest.BuildWith(&key.PublicKey, attestationtest.StrongBiometric(challenge), tc.o)
			s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
			require.Equal(t, attestation.VerdictTrusted, s.Verdict, s.Problem)
			assert.Equal(t, tc.want, s.Dates)
			assert.Contains(t, s.DatesProblem, tc.problem)
			assert.Equal(t, tc.want == attestation.DatesValid, s.MeetsRequirement())
			if tc.want == attestation.DatesInvalid {
				assert.Contains(t, s.Shortfall(), "dates")
			}
		})
	}
}

func TestEvaluate_RevocationList(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	chain := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	list := func(doc string) *attestation.StatusList {
		l, err := attestation.ParseStatusList([]byte(doc))
		require.NoError(t, err)
		return l
	}
	o := trusting(t, chain)
	o.StatusOff = false

	o.Status = list(`{"entries":{"abc":{"status":"REVOKED"}}}`)
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.RevocationClear, s.Revocation)
	assert.True(t, s.MeetsRequirement())

	o.Status = list(`{"entries":{"3003":{"status":"REVOKED","reason":"KEY_COMPROMISE"}}}`)
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.RevocationRevoked, s.Revocation)
	assert.Equal(t, "3003", s.RevokedSerial)
	assert.False(t, s.MeetsRequirement())
	assert.Contains(t, s.Shortfall(), "status list")

	o.Status = list(`{"entries":{"1001":{"status":"SUSPENDED"}}}`)
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.RevocationSuspended, s.Revocation, "the root counts too")
	assert.False(t, s.MeetsRequirement())

	o.Status = nil
	s = attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.RevocationUnavailable, s.Revocation)
	assert.False(t, s.MeetsRequirement(), "no usable list: not met")
	assert.Equal(t, "the attestation key status list is not available", s.Shortfall())
}

func TestEvaluate_AttestationMustComeFromSecureHardware(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	d := attestationtest.StrongBiometric(challenge)
	d.AttestationLevel = 0 // software attestation of a key that claims TEE
	chain := attestationtest.Build(&key.PublicKey, d)
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
	require.Equal(t, attestation.VerdictTrusted, s.Verdict)
	assert.Equal(t, attestation.LevelTEE, s.SecurityLevel)
	assert.False(t, s.MeetsRequirement())
	assert.Equal(t, "the attestation was not made by secure hardware", s.Shortfall())
}

func TestEvaluate_PropertiesThatFailTheRequirement(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		mutate func(*attestationtest.Description)
		check  func(*testing.T, attestation.Summary)
		meets  bool
	}{
		"software key": {
			mutate: func(d *attestationtest.Description) { d.KeyLevel = 0 },
			check:  func(t *testing.T, s attestation.Summary) { assert.Equal(t, attestation.LevelSoftware, s.SecurityLevel) },
		},
		"strongbox is fine": {
			mutate: func(d *attestationtest.Description) { d.KeyLevel, d.AttestationLevel = 2, 2 },
			check: func(t *testing.T, s attestation.Summary) {
				assert.Equal(t, attestation.LevelStrongBox, s.SecurityLevel)
			}, meets: true,
		},
		"another challenge": {
			mutate: func(d *attestationtest.Description) { d.Challenge = []byte("other") },
			check:  func(t *testing.T, s attestation.Summary) { assert.False(t, s.ChallengeMatches) },
		},
		"auth timeout": {
			mutate: func(d *attestationtest.Description) { d.Hardware.AuthTimeout = new(30) },
			check:  func(t *testing.T, s attestation.Summary) { assert.False(t, s.AuthPerUse) },
		},
		"timeout zero is per use": {
			mutate: func(d *attestationtest.Description) { d.Hardware.AuthTimeout = new(0) },
			check:  func(t *testing.T, s attestation.Summary) { assert.True(t, s.AuthPerUse) }, meets: true,
		},
		"no auth required in software": {
			mutate: func(d *attestationtest.Description) { d.Software.NoAuthRequired = true },
			check:  func(t *testing.T, s attestation.Summary) { assert.False(t, s.AuthPerUse) },
		},
		"device credential allowed": {
			mutate: func(d *attestationtest.Description) { d.Hardware.UserAuthType = new(3) },
			check: func(t *testing.T, s attestation.Summary) {
				assert.True(t, s.AuthPerUse)
				assert.False(t, s.BiometricOnly)
			},
		},
		"user auth only in software": {mutate: func(d *attestationtest.Description) {
			d.Software.UserAuthType, d.Hardware.UserAuthType = d.Hardware.UserAuthType, nil
		}, check: func(t *testing.T, s attestation.Summary) {
			assert.False(t, s.AuthPerUse)
			assert.False(t, s.BiometricOnly)
		}},
		"wrong curve": {
			mutate: func(d *attestationtest.Description) { d.Hardware.ECCurve = new(2) },
			check:  func(t *testing.T, s attestation.Summary) { assert.False(t, s.SignP256) },
		},
		"not a signing key": {
			mutate: func(d *attestationtest.Description) { d.Hardware.Purposes = []int{3} },
			check:  func(t *testing.T, s attestation.Summary) { assert.False(t, s.SignP256) },
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			key := newKey(t)
			d := attestationtest.StrongBiometric(challenge)
			tc.mutate(&d)
			chain := attestationtest.Build(&key.PublicKey, d)
			s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, trusting(t, chain))
			require.Equal(t, attestation.VerdictTrusted, s.Verdict)
			tc.check(t, s)
			assert.Equal(t, tc.meets, s.MeetsRequirement())
			assert.Equal(t, tc.meets, s.Shortfall() == "")
		})
	}
}

func TestEvaluate_MalformedChains(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	good := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	other := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	tooMany := make([]string, attestation.MaxCertificates+1)
	for i := range tooMany {
		tooMany[i] = good.Base64[0]
	}
	cases := map[string]struct {
		chain []string
		key   *ecdsa.PublicKey
	}{
		"not base64":                       {[]string{"%%%", good.Base64[1]}, &key.PublicKey},
		"a leaf alone":                     {good.Base64[:1], &key.PublicKey},
		"not a certificate":                {[]string{base64.StdEncoding.EncodeToString([]byte("nope")), good.Base64[1]}, &key.PublicKey},
		"broken link":                      {[]string{good.Base64[0], other.Base64[1], good.Base64[2], good.Base64[3]}, &key.PublicKey},
		"root not self-issued":             {good.Base64[:3], &key.PublicKey},
		"leaf for another key":             {good.Base64, &newKey(t).PublicKey},
		"too many":                         {tooMany, &key.PublicKey},
		"chain starts at the intermediate": {good.Base64[1:], &key.PublicKey},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := attestation.Evaluate(tc.chain, tc.key, challenge, trusting(t, good))
			assert.Equal(t, attestation.VerdictMalformed, s.Verdict)
			assert.NotEmpty(t, s.Problem)
			assert.False(t, s.MeetsRequirement())
		})
	}
}

func TestEvaluate_NoTimeMeansDatesNotValid(t *testing.T) {
	t.Parallel()
	key := newKey(t)
	chain := attestationtest.Build(&key.PublicKey, attestationtest.StrongBiometric(challenge))
	o := trusting(t, chain)
	o.Now = time.Time{}
	s := attestation.Evaluate(chain.Base64, &key.PublicKey, challenge, o)
	assert.Equal(t, attestation.DatesInvalid, s.Dates)
	assert.False(t, s.MeetsRequirement())
}

func TestParseRootsPEM_RefusesAFileWithoutCertificates(t *testing.T) {
	t.Parallel()
	_, err := attestation.ParseRootsPEM([]byte("not pem"))
	require.Error(t, err)
	var nilRoots *attestation.Roots
	assert.False(t, nilRoots.Configured())
}

func TestChallengeIsDomainSeparatedAndOrderSensitive(t *testing.T) {
	t.Parallel()
	a := attestation.Challenge("fws_1", "fwu_1", "fwd_1")
	assert.Len(t, a, 32)
	assert.NotEqual(t, a, attestation.Challenge("fwu_1", "fws_1", "fwd_1"))
	assert.Equal(t, a, attestation.Challenge("fws_1", "fwu_1", "fwd_1"))
}
