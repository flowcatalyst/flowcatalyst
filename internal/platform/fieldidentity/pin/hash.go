package pin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
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

// ErrPepperRetired is returned by Verify for a hash made under a pepper whose
// app key is no longer configured (neither FLOWCATALYST_APP_KEY nor listed in
// FLOWCATALYST_APP_KEY_PREVIOUS). The PIN cannot be checked at all — it is
// not a wrong PIN, so it must not count towards a lockout — and the way out
// is an admin PIN reset (or putting the old key back in the previous list).
var ErrPepperRetired = errors.New("field identity: PIN was hashed under an app key that is no longer configured")

// Hasher hashes and verifies PINs. A nil *Hasher is the unconfigured state:
// every method fails closed with [ErrUnconfigured].
//
// Key rotation: the pepper derives from the app key, so a rotated key means a
// new pepper. Hash always uses the current pepper and returns its id (a
// fingerprint of the pepper, stored beside the hash as pin_pepper_id); Verify
// picks the pepper by that id, so a wrong PIN costs one argon2id whatever
// keys are configured. A hash stored before ids existed has none: Verify
// tries the current pepper, then each previous one. A match under anything
// but the current pepper (or with no id) reports needsRehash, and the
// sign-in re-hashes the PIN under the current pepper in the same transaction
// — the PINs migrate as workers sign in.
type Hasher struct {
	pepper   []byte
	id       string
	previous []pepperKey
}

type pepperKey struct {
	pepper []byte
	id     string
}

// Digest is a freshly hashed PIN: the PHC string and the id of the pepper it
// was made under (both are stored).
type Digest struct {
	Hash     string
	PepperID string
}

// Stored is a PIN hash as stored: the PHC string, its scheme, and the id of
// its pepper ("" for a hash stored before pepper ids existed, and for LEGACY
// hashes, which have no pepper).
type Stored struct {
	Hash     string
	Scheme   string
	PepperID string
}

// NewHasher derives the pepper from the app key (HKDF-SHA256, info
// "fc-field-pin"), and one from each previous app key so hashes made before a
// rotation still verify. An empty key yields (nil, ErrUnconfigured); the nil
// *Hasher is still safe to hold and call.
func NewHasher(appKey string, previous ...string) (*Hasher, error) {
	appKey = strings.TrimSpace(appKey)
	if appKey == "" {
		return nil, ErrUnconfigured
	}
	var prevKeys []string
	for _, p := range previous {
		if p = strings.TrimSpace(p); p != "" && p != appKey {
			prevKeys = append(prevKeys, p)
		}
	}
	cur, prev, err := encryption.DeriveKeys(appKey, prevKeys, "fc-field-pin")
	if err != nil {
		return nil, err
	}
	h := &Hasher{pepper: cur, id: pepperID(cur)}
	for _, p := range prev {
		h.previous = append(h.previous, pepperKey{pepper: p, id: pepperID(p)})
	}
	return h, nil
}

// pepperID is the stored id of a pepper: a truncated SHA-256 of the pepper
// itself under a fixed label — a fingerprint of the derived pepper, never of
// the app key, so it reveals nothing usable about either.
func pepperID(pepper []byte) string {
	sum := sha256.Sum256(append([]byte("fc-field-pin-id\x00"), pepper...))
	return hex.EncodeToString(sum[:6])
}

// Configured reports whether the hasher can hash and verify.
func (h *Hasher) Configured() bool { return h != nil && len(h.pepper) > 0 }

// PepperID is the id of the current pepper ("" when unconfigured).
func (h *Hasher) PepperID() string {
	if !h.Configured() {
		return ""
	}
	return h.id
}

// input is the value actually fed to argon2id.
func input(pepper []byte, clientID, identityID, pin string) string {
	m := hmac.New(sha256.New, pepper)
	m.Write([]byte(clientID))
	m.Write([]byte{0})
	m.Write([]byte(identityID))
	m.Write([]byte{0})
	m.Write([]byte(pin))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Hash returns the PEPPER_V1 hash of pin for (client, identity) under the
// current pepper, with that pepper's id.
func (h *Hasher) Hash(clientID, identityID, pin string) (Digest, error) {
	if !h.Configured() {
		return Digest{}, ErrUnconfigured
	}
	enc, err := passwordhash.HashWithParams(input(h.pepper, clientID, identityID, pin), Params)
	if err != nil {
		return Digest{}, err
	}
	return Digest{Hash: enc, PepperID: h.id}, nil
}

// Verify checks pin against a stored hash. needsRehash is true when the PIN
// matched but the stored hash is not a PEPPER_V1 hash under the current
// pepper with its id: a LEGACY hash, a hash under a previous pepper, or one
// without a pepper id (the caller re-hashes with [Hasher.Hash] and stores the
// digest). A mismatch is (false, false, nil); err is only ErrUnconfigured,
// ErrPepperRetired or a malformed stored hash.
func (h *Hasher) Verify(clientID, identityID, pin string, stored Stored) (ok, needsRehash bool, err error) {
	if !h.Configured() {
		return false, false, ErrUnconfigured
	}
	if stored.Scheme == SchemeLegacy {
		ok, err := verify(pin, stored.Hash)
		return ok, ok, err
	}
	if stored.PepperID != "" {
		pepper := h.pepperFor(stored.PepperID)
		if pepper == nil {
			return false, false, ErrPepperRetired
		}
		ok, err := verify(input(pepper, clientID, identityID, pin), stored.Hash)
		return ok, ok && stored.PepperID != h.id, err
	}
	// No id: stored before ids existed. Current pepper first (the common case
	// costs what it always did), then each previous one.
	ok, err = verify(input(h.pepper, clientID, identityID, pin), stored.Hash)
	if ok || err != nil {
		return ok, ok, err
	}
	for _, p := range h.previous {
		ok, err = verify(input(p.pepper, clientID, identityID, pin), stored.Hash)
		if ok || err != nil {
			return ok, ok, err
		}
	}
	return false, false, nil
}

func (h *Hasher) pepperFor(id string) []byte {
	if id == h.id {
		return h.pepper
	}
	for _, p := range h.previous {
		if p.id == id {
			return p.pepper
		}
	}
	return nil
}

// verify is passwordhash.Verify with a mismatch as (false, nil).
func verify(plain, encoded string) (bool, error) {
	switch verr := passwordhash.Verify(plain, encoded); {
	case verr == nil:
		return true, nil
	case errors.Is(verr, passwordhash.ErrMismatch):
		return false, nil
	default:
		return false, verr
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
		_ = passwordhash.Verify(input(h.pepper, "-", "-", pin), d)
	}
}
