package pin

import (
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
)

// Hash schemes stored in field_identities.pin_scheme.
const (
	// SchemePepperV1 is the native scheme: argon2id over an HMAC of the PIN.
	SchemePepperV1 = "PEPPER_V1"
	// SchemeLegacy is an imported unpeppered hash (bcrypt or argon2i from an
	// app's own PIN table), verified on the raw PIN and rehashed to
	// PEPPER_V1 on the first successful verify.
	SchemeLegacy = "LEGACY"
)

// PepperVersion is the version of the HKDF pepper recorded with each hash.
const PepperVersion = 1

// ErrUnconfigured is returned by every Hasher operation when no
// FLOWCATALYST_APP_KEY is available. The field plane fails closed: it never
// hashes or verifies a PIN without the pepper.
var ErrUnconfigured = errors.New("field identity: FLOWCATALYST_APP_KEY is not set")

// Params are the argon2id parameters for PIN hashes (OWASP minimum m=19456,
// t=2, p=1). They are deliberately lighter than the password defaults: PINs
// are verified in bursts at shift start, and the pepper does the real work
// against an offline attacker.
var Params = passwordhash.Params{
	Memory: 19456, Iterations: 2, Parallelism: 1, KeyLength: 32, SaltLength: 16,
}

// Hasher hashes and verifies PINs. A nil *Hasher is the unconfigured state:
// every method fails closed with [ErrUnconfigured].
type Hasher struct{ pepper []byte }

// NewHasher derives the pepper from the app key (HKDF-SHA256, info
// "fc-field-pin"). An empty key yields (nil, ErrUnconfigured); the nil
// *Hasher is still safe to hold and call.
func NewHasher(appKey string) (*Hasher, error) {
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return nil, ErrUnconfigured
	}
	pepper, err := hkdf.Key(sha256.New, []byte(appKey), nil, "fc-field-pin", 32)
	if err != nil {
		return nil, err
	}
	return &Hasher{pepper: pepper}, nil
}

// Configured reports whether the hasher can hash and verify.
func (h *Hasher) Configured() bool { return h != nil && len(h.pepper) > 0 }

// input is the value actually fed to argon2id.
func (h *Hasher) input(clientID, identityID, pin string) string {
	m := hmac.New(sha256.New, h.pepper)
	m.Write([]byte(clientID))
	m.Write([]byte{0})
	m.Write([]byte(identityID))
	m.Write([]byte{0})
	m.Write([]byte(pin))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Hash returns the PEPPER_V1 hash of pin for (client, identity).
func (h *Hasher) Hash(clientID, identityID, pin string) (string, error) {
	if !h.Configured() {
		return "", ErrUnconfigured
	}
	return passwordhash.HashWithParams(h.input(clientID, identityID, pin), Params)
}

// Verify checks pin against a stored hash of the given scheme. needsRehash is
// true for a LEGACY hash that matched (the caller re-hashes with [Hasher.Hash]
// and stores scheme PEPPER_V1). A mismatch is (false, false, nil); err is only
// ErrUnconfigured or a malformed stored hash.
func (h *Hasher) Verify(clientID, identityID, pin, encoded, scheme string) (ok, needsRehash bool, err error) {
	if !h.Configured() {
		return false, false, ErrUnconfigured
	}
	plain := h.input(clientID, identityID, pin)
	if scheme == SchemeLegacy {
		plain = pin
	}
	switch verr := passwordhash.Verify(plain, encoded); {
	case verr == nil:
		return true, scheme == SchemeLegacy, nil
	case errors.Is(verr, passwordhash.ErrMismatch):
		return false, false, nil
	default:
		return false, false, verr
	}
}

var dummy = sync.OnceValue(func() string {
	h, err := passwordhash.HashWithParams("fc-field-pin-timing-sentinel", Params)
	if err != nil {
		return ""
	}
	return h
})

// Dummy spends one verify with the PIN parameters and discards the result, so
// a login for a worker that does not exist (or cannot sign in) costs what a
// real one does. A no-op when unconfigured.
func (h *Hasher) Dummy(pin string) {
	if !h.Configured() {
		return
	}
	if d := dummy(); d != "" {
		_ = passwordhash.Verify(h.input("-", "-", pin), d)
	}
}
