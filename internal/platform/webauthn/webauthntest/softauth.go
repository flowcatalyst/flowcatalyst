// Package webauthntest is a software WebAuthn authenticator for tests: it
// builds the registration (attestation "none") and assertion payloads a
// browser would send, so the ceremony paths can be exercised against the real
// go-webauthn verifier without a browser. Test support only.
package webauthntest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"math/big"
	"testing"
)

// Authenticator is one software credential (P-256 / ES256).
type Authenticator struct {
	t       testing.TB
	key     *ecdsa.PrivateKey
	CredID  []byte
	Counter uint32
}

// New creates an authenticator with a fresh key and credential ID.
func New(t testing.TB) *Authenticator {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	id := make([]byte, 32)
	_, _ = rand.Read(id)
	return &Authenticator{t: t, key: k, CredID: id}
}

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

// ── minimal CBOR (just the shapes WebAuthn needs) ────────────────────────

func cborHead(major byte, n uint64) []byte {
	switch {
	case n < 24:
		return []byte{major<<5 | byte(n)}
	case n < 256:
		return []byte{major<<5 | 24, byte(n)}
	default:
		return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
	}
}
func cborUint(n uint64) []byte { return cborHead(0, n) }
func cborNeg(n int64) []byte   { return cborHead(1, uint64(-1-n)) }
func cborBytes(b []byte) []byte {
	return append(cborHead(2, uint64(len(b))), b...)
}
func cborText(s string) []byte { return append(cborHead(3, uint64(len(s))), s...) }
func cborMap(n int, kv ...[]byte) []byte {
	out := cborHead(5, uint64(n))
	for _, b := range kv {
		out = append(out, b...)
	}
	return out
}

func (a *Authenticator) coseKey() []byte {
	// Uncompressed SEC1 point: 0x04 || X || Y.
	pt, err := a.key.PublicKey.Bytes()
	if err != nil {
		a.t.Fatal(err)
	}
	x, y := pt[1:33], pt[33:65]
	return cborMap(5,
		cborUint(1), cborUint(2), // kty EC2
		cborUint(3), cborNeg(-7), // alg ES256
		cborNeg(-1), cborUint(1), // crv P-256
		cborNeg(-2), cborBytes(x),
		cborNeg(-3), cborBytes(y),
	)
}

func rpHash(rpID string) []byte { h := sha256.Sum256([]byte(rpID)); return h[:] }

// Flags: UP=0x01 UV=0x04 AT=0x40.
func authData(rpID string, flags byte, counter uint32, attested []byte) []byte {
	out := append([]byte{}, rpHash(rpID)...)
	out = append(out, flags)
	c := make([]byte, 4)
	binary.BigEndian.PutUint32(c, counter)
	out = append(out, c...)
	return append(out, attested...)
}

func clientData(typ, challenge, origin string) []byte {
	b, _ := json.Marshal(map[string]any{"type": typ, "challenge": challenge, "origin": origin})
	return b
}

// Register builds the JSON body of navigator.credentials.create() for a
// ceremony with the given challenge (base64url), as if made on origin for
// rpID. Pass a different rpID/origin than the server's to simulate phishing.
func (a *Authenticator) Register(challenge, origin, rpID string) []byte {
	attested := append(make([]byte, 16), cborLen16(len(a.CredID))...) // aaguid + credIdLen
	attested = append(attested, a.CredID...)
	attested = append(attested, a.coseKey()...)
	ad := authData(rpID, 0x45, a.Counter, attested)
	attObj := cborMap(3,
		cborText("fmt"), cborText("none"),
		cborText("attStmt"), cborMap(0),
		cborText("authData"), cborBytes(ad),
	)
	body, _ := json.Marshal(map[string]any{
		"id": b64(a.CredID), "rawId": b64(a.CredID), "type": "public-key",
		"response": map[string]any{
			"clientDataJSON":    b64(clientData("webauthn.create", challenge, origin)),
			"attestationObject": b64(attObj),
		},
	})
	return body
}

func cborLen16(n int) []byte { return []byte{byte(n >> 8), byte(n)} }

// Assert builds the JSON body of navigator.credentials.get(). It advances the
// signature counter first unless the caller sets Counter itself afterwards.
func (a *Authenticator) Assert(challenge, origin, rpID string, userHandle []byte) []byte {
	a.Counter++
	return a.AssertWithCounter(challenge, origin, rpID, userHandle, a.Counter)
}

// AssertWithCounter is Assert with an explicit signature counter (use a
// non-advancing value to simulate a cloned authenticator).
func (a *Authenticator) AssertWithCounter(challenge, origin, rpID string, userHandle []byte, counter uint32) []byte {
	ad := authData(rpID, 0x05, counter, nil)
	cd := clientData("webauthn.get", challenge, origin)
	cdHash := sha256.Sum256(cd)
	digest := sha256.Sum256(append(append([]byte{}, ad...), cdHash[:]...))
	r, s, err := ecdsa.Sign(rand.Reader, a.key, digest[:])
	if err != nil {
		a.t.Fatal(err)
	}
	sig := derSig(r, s)
	resp := map[string]any{
		"clientDataJSON":    b64(cd),
		"authenticatorData": b64(ad),
		"signature":         b64(sig),
	}
	if userHandle != nil {
		resp["userHandle"] = b64(userHandle)
	}
	body, _ := json.Marshal(map[string]any{
		"id": b64(a.CredID), "rawId": b64(a.CredID), "type": "public-key", "response": resp,
	})
	return body
}

func derInt(n *big.Int) []byte {
	b := n.Bytes()
	if len(b) == 0 || b[0]&0x80 != 0 {
		b = append([]byte{0}, b...)
	}
	return append([]byte{0x02, byte(len(b))}, b...)
}

func derSig(r, s *big.Int) []byte {
	body := append(derInt(r), derInt(s)...)
	return append([]byte{0x30, byte(len(body))}, body...)
}
