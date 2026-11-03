package fieldauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"
)

// Proof is a verified DPoP-style proof of possession: a JWS signed by the
// device key whose public half travels in the JOSE header (RFC 9449 shape).
// Verifying it proves the caller holds the key; it does not by itself say the
// key is enrolled, or that the nonce is fresh (the handler does both).
type Proof struct {
	// JKT is the RFC 7638 thumbprint of the proof key.
	JKT string
	// JWK is the canonical public-key JSON ({"crv","kty","x","y"}).
	JWK string

	HTM   string
	HTU   string
	IAT   int64
	JTI   string
	Nonce string
	ATH   string
}

// ProofClockSkew is how far a proof's iat may be from the server clock.
// Devices have bad clocks; the nonce, not iat, defends against replay.
const ProofClockSkew = 24 * time.Hour

// ErrProof is a malformed or unverifiable proof (HTTP 401 invalid_dpop_proof).
var ErrProof = errors.New("invalid DPoP proof")

func proofErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrProof, fmt.Sprintf(format, a...))
}

type jwkPublic struct {
	Kty string `json:"kty"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// ParseECJWK validates a public EC P-256 JWK (no private members) and returns
// the key, its canonical JSON and its RFC 7638 thumbprint.
func ParseECJWK(raw []byte) (*ecdsa.PublicKey, string, string, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, "", "", proofErr("jwk is not a JSON object")
	}
	for _, private := range []string{"d", "p", "q", "dp", "dq", "qi", "oth", "k"} {
		if _, ok := members[private]; ok {
			return nil, "", "", proofErr("jwk carries private key material")
		}
	}
	var j jwkPublic
	if err := json.Unmarshal(raw, &j); err != nil {
		return nil, "", "", proofErr("jwk is malformed")
	}
	if j.Kty != "EC" || j.Crv != "P-256" {
		return nil, "", "", proofErr("jwk must be an EC P-256 key")
	}
	x, err := base64.RawURLEncoding.DecodeString(j.X)
	if err != nil || len(x) != 32 {
		return nil, "", "", proofErr("jwk x is malformed")
	}
	y, err := base64.RawURLEncoding.DecodeString(j.Y)
	if err != nil || len(y) != 32 {
		return nil, "", "", proofErr("jwk y is malformed")
	}
	point := make([]byte, 0, 65)
	point = append(point, 4)
	point = append(point, x...)
	point = append(point, y...)
	pub, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), point)
	if err != nil {
		return nil, "", "", proofErr("jwk point is not on the curve")
	}
	// Re-encode from the decoded bytes so the thumbprint input is canonical.
	cx, cy := base64.RawURLEncoding.EncodeToString(x), base64.RawURLEncoding.EncodeToString(y)
	canonical := `{"crv":"P-256","kty":"EC","x":"` + cx + `","y":"` + cy + `"}`
	sum := sha256.Sum256([]byte(canonical))
	return pub, canonical, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}

// ParseProof verifies a DPoP proof's structure and signature. now bounds iat.
// It checks typ, alg, the embedded key, the ES256 signature, and that jti and
// iat are present and iat is within [ProofClockSkew]; htm, htu, nonce and ath
// are returned for the caller to compare.
func ParseProof(compact string, now time.Time) (*Proof, error) {
	parts := strings.Split(strings.TrimSpace(compact), ".")
	if len(parts) != 3 {
		return nil, proofErr("not a compact JWS")
	}
	hdrBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, proofErr("header is not base64url")
	}
	var hdr struct {
		Typ string          `json:"typ"`
		Alg string          `json:"alg"`
		JWK json.RawMessage `json:"jwk"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, proofErr("header is not JSON")
	}
	if hdr.Typ != "dpop+jwt" {
		return nil, proofErr("typ must be dpop+jwt")
	}
	if hdr.Alg != "ES256" {
		return nil, proofErr("alg must be ES256")
	}
	if len(hdr.JWK) == 0 {
		return nil, proofErr("jwk is missing")
	}
	pub, canonical, jkt, err := ParseECJWK(hdr.JWK)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, proofErr("signature is malformed")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, digest[:], r, s) {
		return nil, proofErr("signature does not verify")
	}
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, proofErr("claims are not base64url")
	}
	var c struct {
		HTM   string `json:"htm"`
		HTU   string `json:"htu"`
		IAT   *int64 `json:"iat"`
		JTI   string `json:"jti"`
		Nonce string `json:"nonce"`
		ATH   string `json:"ath"`
	}
	if err := json.Unmarshal(claimBytes, &c); err != nil {
		return nil, proofErr("claims are not JSON")
	}
	if c.JTI == "" {
		return nil, proofErr("jti is missing")
	}
	if c.IAT == nil {
		return nil, proofErr("iat is missing")
	}
	if d := now.Sub(time.Unix(*c.IAT, 0)); d > ProofClockSkew || d < -ProofClockSkew {
		return nil, proofErr("iat is more than 24h from the server clock")
	}
	return &Proof{JKT: jkt, JWK: canonical, HTM: c.HTM, HTU: c.HTU, IAT: *c.IAT, JTI: c.JTI, Nonce: c.Nonce, ATH: c.ATH}, nil
}

// AccessTokenHash is the proof's ath for an access token: base64url(SHA-256).
func AccessTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
