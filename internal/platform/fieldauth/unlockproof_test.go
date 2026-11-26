package fieldauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func p256(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return k
}

func seg(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(b)
}

// sign makes a compact JWS with a raw r||s P-256 signature, as a phone does.
func sign(t *testing.T, k *ecdsa.PrivateKey, header, claims map[string]any) string {
	t.Helper()
	input := seg(t, header) + "." + seg(t, claims)
	sum := sha256.Sum256([]byte(input))
	der, err := ecdsa.SignASN1(rand.Reader, k, sum[:])
	require.NoError(t, err)
	var rs struct{ R, S *big.Int }
	_, err = asn1.Unmarshal(der, &rs)
	require.NoError(t, err)
	return input + "." + base64.RawURLEncoding.EncodeToString(append(rs.R.FillBytes(make([]byte, 32)), rs.S.FillBytes(make([]byte, 32))...))
}

func assertionParts() (map[string]any, map[string]any, UnlockExpect) {
	h := map[string]any{"typ": UnlockAssertionType, "alg": "ES256", "kid": "fwk_1"}
	c := map[string]any{
		"aud": "https://fc.example.test/field/auth/unlock", "nonce": "ch-1", "sub": "fwu_1",
		"device_id": "fwd_1", "app": "warehouse-app", "iat": time.Now().Unix(),
	}
	want := UnlockExpect{
		Type: UnlockAssertionType, KeyID: "fwk_1", Aud: "https://fc.example.test/field/auth/unlock",
		Sub: "fwu_1", DeviceID: "fwd_1", App: "warehouse-app",
	}
	return h, c, want
}

func TestVerifyUnlockJWS_ValidAssertionAndKeyProof(t *testing.T) {
	t.Parallel()
	k := p256(t)
	h, c, want := assertionParts()
	got, err := VerifyUnlockJWS(sign(t, k, h, c), &k.PublicKey, want, time.Now())
	require.NoError(t, err)
	assert.Equal(t, "ch-1", got.Nonce, "an empty expected nonce hands the nonce to the caller")

	proof := sign(t, k, map[string]any{"typ": KeyProofType, "alg": "ES256"},
		map[string]any{
			"aud": "https://fc.example.test/field/auth/unlock-keys", "nonce": "n-1", "sub": "fwu_1",
			"device_id": "fwd_1", "iat": time.Now().Unix(),
		})
	_, err = VerifyUnlockJWS(proof, &k.PublicKey, UnlockExpect{
		Type: KeyProofType,
		Aud:  "https://fc.example.test/field/auth/unlock-keys", Nonce: "n-1", Sub: "fwu_1", DeviceID: "fwd_1",
	}, time.Now())
	require.NoError(t, err)
}

func TestVerifyUnlockJWS_Refusals(t *testing.T) {
	t.Parallel()
	k := p256(t)
	cases := map[string]struct {
		mutate func(h, c map[string]any)
		want   func(*UnlockExpect)
		key    *ecdsa.PublicKey
		raw    func(string) string
	}{
		"wrong typ":         {mutate: func(h, _ map[string]any) { h["typ"] = KeyProofType }},
		"dpop typ":          {mutate: func(h, _ map[string]any) { h["typ"] = "dpop+jwt" }},
		"wrong alg":         {mutate: func(h, _ map[string]any) { h["alg"] = "ES384" }},
		"no kid":            {mutate: func(h, _ map[string]any) { delete(h, "kid") }},
		"other kid":         {mutate: func(h, _ map[string]any) { h["kid"] = "fwk_2" }},
		"kid on key proof":  {want: func(w *UnlockExpect) { w.KeyID = "" }},
		"embedded key":      {mutate: func(h, _ map[string]any) { h["jwk"] = map[string]string{"kty": "EC"} }},
		"crit":              {mutate: func(h, _ map[string]any) { h["crit"] = []string{"b64"} }},
		"an unknown member": {mutate: func(h, _ map[string]any) { h["x5c"] = []string{"MII"} }},
		"aud":               {mutate: func(_, c map[string]any) { c["aud"] = "https://other.example.test/field/auth/unlock" }},
		"empty nonce":       {mutate: func(_, c map[string]any) { c["nonce"] = "" }},
		"expected nonce":    {want: func(w *UnlockExpect) { w.Nonce = "ch-2" }},
		"sub":               {mutate: func(_, c map[string]any) { c["sub"] = "fwu_2" }},
		"device":            {mutate: func(_, c map[string]any) { c["device_id"] = "fwd_2" }},
		"app":               {mutate: func(_, c map[string]any) { c["app"] = "other-app" }},
		"no iat":            {mutate: func(_, c map[string]any) { delete(c, "iat") }},
		"future iat":        {mutate: func(_, c map[string]any) { c["iat"] = time.Now().Add(25 * time.Hour).Unix() }},
		"another key":       {key: &p256(t).PublicKey},
		"no key":            {key: nil},
		"two segments":      {raw: func(s string) string { return s[:strings.LastIndex(s, ".")] }},
		"padded signature":  {raw: func(s string) string { return s + "=" }},
		"short signature":   {raw: func(s string) string { return s[:strings.LastIndex(s, ".")+1] + "AAAA" }},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h, c, want := assertionParts()
			if tc.mutate != nil {
				tc.mutate(h, c)
			}
			if tc.want != nil {
				tc.want(&want)
			}
			compact := sign(t, k, h, c)
			if tc.raw != nil {
				compact = tc.raw(compact)
			}
			pub := &k.PublicKey
			if tc.key != nil || name == "no key" {
				pub = tc.key
			}
			_, err := VerifyUnlockJWS(compact, pub, want, time.Now())
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnlockProof))
		})
	}
}

func TestVerifyUnlockJWS_ADPoPProofIsNotAnAssertion(t *testing.T) {
	t.Parallel()
	k := p256(t)
	pt, err := k.PublicKey.Bytes()
	require.NoError(t, err)
	jwk := map[string]string{
		"kty": "EC", "crv": "P-256",
		"x": base64.RawURLEncoding.EncodeToString(pt[1:33]), "y": base64.RawURLEncoding.EncodeToString(pt[33:]),
	}
	dpop := sign(t, k, map[string]any{"typ": "dpop+jwt", "alg": "ES256", "jwk": jwk},
		map[string]any{"htm": "POST", "htu": "https://fc.example.test/field/auth/unlock", "iat": time.Now().Unix(), "jti": "j", "nonce": "ch-1"})
	_, _, want := assertionParts()
	_, err = VerifyUnlockJWS(dpop, &k.PublicKey, want, time.Now())
	require.Error(t, err)
	// And an assertion is no DPoP proof.
	h, c, _ := assertionParts()
	_, err = ParseProof(sign(t, k, h, c), time.Now())
	require.ErrorIs(t, err, ErrProof)
}
