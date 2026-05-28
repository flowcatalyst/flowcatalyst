// Package passwordhash is the single source of truth for FlowCatalyst's
// password / OAuth-client-secret hashing.
//
// Hashes are stored in the PHC string format
// (https://github.com/P-H-C/phc-string-format) using argon2id:
//
//	$argon2id$v=19$m=65536,t=3,p=4$<salt-b64>$<hash-b64>
//
// `m`/`t`/`p` mirror the Rust impl
// (crates/fc-platform/src/auth/password_service.rs):
//
//	memory      64 MiB (65536 KiB)
//	iterations  3
//	parallelism 4
//	key length  32 bytes
//	salt length 16 bytes
//
// `Verify` parses the stored PHC envelope so future param changes are
// backwards-compatible: each row carries the params it was hashed with.
//
// All callers (principal password, OAuth client secret, fosite-bound
// client hasher) go through `Hash` + `Verify`. There is no other path.
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Params is the Argon2id parameter set. The defaults match the Rust
// password_service.rs configuration.
type Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	KeyLength   uint32
	SaltLength  uint32
}

// DefaultParams is what new hashes use. Existing rows may carry different
// params in their PHC envelope; Verify reads those rather than these.
var DefaultParams = Params{
	Memory:      64 * 1024,
	Iterations:  3,
	Parallelism: 4,
	KeyLength:   32,
	SaltLength:  16,
}

// Hash returns a PHC-encoded argon2id hash of `plaintext` using a fresh
// random salt and the default params.
func Hash(plaintext string) (string, error) {
	return HashWithParams(plaintext, DefaultParams)
}

// HashWithParams is the parameter-explicit form. Use for tests that need
// reproducibility or for forcing weaker params on a slow machine.
func HashWithParams(plaintext string, p Params) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("passwordhash: read salt: %w", err)
	}
	hash := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return encode(p, salt, hash), nil
}

// Verify returns nil iff `plaintext` re-hashes to the same value as the
// stored PHC string. `ErrInvalidHash` for malformed input; `ErrMismatch`
// for a hash that parsed but didn't match.
func Verify(plaintext, encoded string) error {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return err
	}
	got := argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

// ErrInvalidHash signals a stored hash that doesn't parse as a PHC
// argon2id envelope.
var ErrInvalidHash = errors.New("passwordhash: invalid PHC string")

// ErrMismatch signals a correctly-formatted hash whose plaintext was
// wrong.
var ErrMismatch = errors.New("passwordhash: hash mismatch")

func encode(p Params, salt, hash []byte) string {
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash))
}

func decode(s string) (Params, []byte, []byte, error) {
	// Expected: $argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>
	if !strings.HasPrefix(s, "$argon2id$") {
		return Params{}, nil, nil, ErrInvalidHash
	}
	parts := strings.Split(s, "$")
	// parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Params{}, nil, nil, ErrInvalidHash
	}
	var m, t uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &par); err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, ErrInvalidHash
	}
	p := Params{
		Memory:      m,
		Iterations:  t,
		Parallelism: par,
		KeyLength:   uint32(len(hash)),
		SaltLength:  uint32(len(salt)),
	}
	return p, salt, hash, nil
}
