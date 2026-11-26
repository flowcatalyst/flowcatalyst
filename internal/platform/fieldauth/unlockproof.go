package fieldauth

import (
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws"
)

// The two JWS kinds an unlock key signs (docs/field-unlock-keys-spec.md
// section 3). Each names itself in typ and in aud, so neither can stand in for
// the other or for a DPoP proof (typ dpop+jwt, signed by the device key).
const (
	// KeyProofType is the typ of the registration's key proof.
	KeyProofType = "fc-unlock-key+jwt"
	// UnlockAssertionType is the typ of an unlock assertion.
	UnlockAssertionType = "fc-unlock+jwt"
	// unlockKeysPath and unlockPath are the aud paths (after the base URL).
	unlockKeysPath = Prefix + "/auth/unlock-keys"
	unlockPath     = Prefix + "/auth/unlock"
)

// ErrUnlockProof is a key proof or unlock assertion that does not verify
// (HTTP 401 INVALID_UNLOCK_PROOF).
var ErrUnlockProof = errors.New("invalid unlock proof")

func unlockErr(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrUnlockProof, fmt.Sprintf(format, a...))
}

// UnlockClaims are the claims of a key proof or an unlock assertion.
type UnlockClaims struct {
	Aud      string `json:"aud"`
	Nonce    string `json:"nonce"`
	Sub      string `json:"sub"`
	DeviceID string `json:"device_id"`
	// App is the application code (unlock assertions only).
	App string `json:"app,omitempty"`
	IAT *int64 `json:"iat"`
}

// UnlockExpect is what a verified JWS must say: every field is compared
// exactly (App is "" for a key proof). Nonce "" accepts any non-empty nonce,
// for the caller to judge (an assertion's nonce is the challenge it consumes).
type UnlockExpect struct {
	Type     string
	KeyID    string // "" = the header must have no kid
	Aud      string
	Nonce    string
	Sub      string
	DeviceID string
	App      string
}

// VerifyUnlockJWS verifies a compact ES256 JWS signed by an unlock key: the
// canonical spelling of every segment, the header (typ, alg, kid), the raw
// r||s signature with pub, and every claim of want; iat must be within
// [ProofClockSkew] of now. It never uses the device key.
func VerifyUnlockJWS(compact string, pub *ecdsa.PublicKey, want UnlockExpect, now time.Time) (*UnlockClaims, error) {
	parts := strings.Split(compact, ".")
	if len(parts) != 3 {
		return nil, unlockErr("not a compact JWS")
	}
	hdrBytes, err := compactjws.DecodeSegment(parts[0])
	if err != nil {
		return nil, unlockErr("header is not base64url")
	}
	// The header is exactly typ, alg and (an assertion) kid: any other member,
	// crit and jwk included, is refused, so nothing in it can change what the
	// signature means.
	var members map[string]json.RawMessage
	if err := json.Unmarshal(hdrBytes, &members); err != nil {
		return nil, unlockErr("header is not JSON")
	}
	allowed := map[string]bool{"typ": true, "alg": true, "kid": want.KeyID != ""}
	for name := range members {
		if !allowed[name] {
			return nil, unlockErr("the header carries %q", name)
		}
	}
	var hdr struct {
		Typ string  `json:"typ"`
		Alg string  `json:"alg"`
		Kid *string `json:"kid"`
	}
	if err := json.Unmarshal(hdrBytes, &hdr); err != nil {
		return nil, unlockErr("header is not JSON")
	}
	switch {
	case hdr.Typ != want.Type:
		return nil, unlockErr("typ must be %s", want.Type)
	case hdr.Alg != "ES256":
		return nil, unlockErr("alg must be ES256")
	case want.KeyID != "" && (hdr.Kid == nil || *hdr.Kid != want.KeyID):
		return nil, unlockErr("kid does not name the unlock key")
	}
	sig, err := compactjws.DecodeSegment(parts[2])
	if err != nil || len(sig) != 64 {
		return nil, unlockErr("signature is malformed")
	}
	if pub == nil {
		return nil, unlockErr("no key")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		return nil, unlockErr("signature does not verify")
	}
	claimBytes, err := compactjws.DecodeSegment(parts[1])
	if err != nil {
		return nil, unlockErr("claims are not base64url")
	}
	var c UnlockClaims
	if err := json.Unmarshal(claimBytes, &c); err != nil {
		return nil, unlockErr("claims are not JSON")
	}
	switch {
	case c.Aud != want.Aud:
		return nil, unlockErr("aud does not match")
	case c.Nonce == "" || (want.Nonce != "" && c.Nonce != want.Nonce):
		return nil, unlockErr("nonce does not match")
	case c.Sub != want.Sub:
		return nil, unlockErr("sub does not match")
	case c.DeviceID != want.DeviceID:
		return nil, unlockErr("device_id does not match")
	case c.App != want.App:
		return nil, unlockErr("app does not match")
	case c.IAT == nil:
		return nil, unlockErr("iat is missing")
	}
	if d := now.Sub(time.Unix(*c.IAT, 0)); d > ProofClockSkew || d < -ProofClockSkew {
		return nil, unlockErr("iat is more than 24h from the server clock")
	}
	return &c, nil
}
