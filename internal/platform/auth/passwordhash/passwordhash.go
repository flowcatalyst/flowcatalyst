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
// Callers are principal/user passwords (create, reset, seed, login
// verify) — they go through `Hash` + `Verify`, there is no other path.
// OAuth client secrets do NOT use this; they're reversibly encrypted via
// shared/encryption (decrypt + compare), matching Rust.
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
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
//
// Both argon2id (the native scheme) and argon2i are accepted — the argon2i
// variant is what an upstream Laravel app produces, and existing users carry
// those hashes. The algorithm is read from the PHC envelope, matching the
// Rust/TS implementations (which verify the whole argon2 family). Migrate an
// argon2i row to argon2id on next login by pairing Verify with NeedsRehash.
func Verify(plaintext, encoded string) error {
	// bcrypt ($2a$/$2b$/$2y$) — the Laravel default. bcrypt only considers the
	// first 72 bytes; truncate to match PHP's password_verify so long passwords
	// compare identically.
	if isBcrypt(encoded) {
		pw := []byte(plaintext)
		if len(pw) > 72 {
			pw = pw[:72]
		}
		switch err := bcrypt.CompareHashAndPassword([]byte(encoded), pw); {
		case err == nil:
			return nil
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return ErrMismatch
		default:
			return ErrInvalidHash
		}
	}

	variant, p, salt, want, err := decode(encoded)
	if err != nil {
		return err
	}
	got := deriveKey(variant, plaintext, p, salt)
	if got == nil {
		return ErrInvalidHash
	}
	if subtle.ConstantTimeCompare(got, want) != 1 {
		return ErrMismatch
	}
	return nil
}

// isBcrypt reports whether the encoded hash is a bcrypt string (the Laravel
// default password algorithm).
func isBcrypt(s string) bool {
	return strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")
}

// deriveKey runs the argon2 variant named in the PHC envelope. Returns nil for
// an unsupported variant.
func deriveKey(variant, plaintext string, p Params, salt []byte) []byte {
	switch variant {
	case "argon2id":
		return argon2.IDKey([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	case "argon2i":
		return argon2.Key([]byte(plaintext), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	default:
		return nil
	}
}

// NeedsRehash reports whether a stored hash should be re-encoded to the native
// scheme after a successful verify. True for any non-native variant (e.g.
// Laravel's argon2i), for argon2id rows whose params differ from DefaultParams,
// and for anything that doesn't parse. Mirrors the Rust/TS needs_rehash so a
// login transparently upgrades a legacy hash without disrupting the user.
func NeedsRehash(encoded string) bool {
	variant, p, _, _, err := decode(encoded)
	if err != nil || variant != "argon2id" {
		return true
	}
	return p.Memory != DefaultParams.Memory ||
		p.Iterations != DefaultParams.Iterations ||
		p.Parallelism != DefaultParams.Parallelism ||
		p.KeyLength != DefaultParams.KeyLength
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

// decode parses a PHC argon2 envelope and returns the variant ("argon2id" or
// "argon2i") alongside its params/salt/hash.
func decode(s string) (string, Params, []byte, []byte, error) {
	// Expected: $argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>  (or $argon2i$…)
	parts := strings.Split(s, "$")
	// parts: ["", "argon2id", "v=19", "m=...,t=...,p=...", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[0] != "" {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	variant := parts[1]
	if variant != "argon2id" && variant != "argon2i" {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	var m, t uint32
	var par uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &par); err != nil {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return "", Params{}, nil, nil, ErrInvalidHash
	}
	p := Params{
		Memory:      m,
		Iterations:  t,
		Parallelism: par,
		KeyLength:   uint32(len(hash)),
		SaltLength:  uint32(len(salt)),
	}
	return variant, p, salt, hash, nil
}
