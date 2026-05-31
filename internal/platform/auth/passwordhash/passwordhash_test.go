package passwordhash

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

// bcrypt is the Laravel default — a $2y$/$2a$ hash must verify and be flagged
// for migration to the native argon2id scheme.
func TestVerifyBcryptAndNeedsRehash(t *testing.T) {
	raw, err := bcrypt.GenerateFromPassword([]byte("S3cret!"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt gen: %v", err)
	}
	// Laravel writes the $2y$ prefix; Go's bcrypt emits $2a$. Both must verify.
	for _, h := range []string{string(raw), strings.Replace(string(raw), "$2a$", "$2y$", 1)} {
		if err := Verify("S3cret!", h); err != nil {
			t.Fatalf("bcrypt verify (correct) %q: %v", h[:4], err)
		}
		if err := Verify("wrong", h); err != ErrMismatch {
			t.Fatalf("bcrypt verify (wrong) %q: got %v, want ErrMismatch", h[:4], err)
		}
		if !NeedsRehash(h) {
			t.Fatalf("bcrypt hash %q should need rehash to argon2id", h[:4])
		}
	}
}

func TestHashRoundTrip(t *testing.T) {
	encoded, err := Hash("correct horse battery staple")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("unexpected PHC prefix: %s", encoded)
	}
	if err := Verify("correct horse battery staple", encoded); err != nil {
		t.Errorf("Verify on correct password: %v", err)
	}
	if err := Verify("wrong", encoded); err != ErrMismatch {
		t.Errorf("Verify wrong: got %v, want ErrMismatch", err)
	}
}

func TestHashSaltUniqueness(t *testing.T) {
	a, _ := Hash("same plaintext")
	b, _ := Hash("same plaintext")
	if a == b {
		t.Fatalf("two hashes of the same input should differ (per-row salt): %s", a)
	}
}

func TestVerifyInvalidEnvelope(t *testing.T) {
	for _, bad := range []string{
		"",
		"not-a-phc-string",
		"$argon2id$v=19$m=64,t=1,p=4$short",     // missing hash
		"$argon2d$v=19$m=64,t=1,p=4$AAAA$AAAA",  // unsupported variant (argon2d)
		"$2y$10$abcdefghijklmnopqrstuv",         // bcrypt — not an argon2 envelope
		"$argon2id$v=18$m=64,t=1,p=4$AAAA$AAAA", // wrong version
		"$argon2id$v=19$m=64,t=1,p=4$!!!!$AAAA", // bad base64 salt
	} {
		if err := Verify("x", bad); err != ErrInvalidHash {
			t.Errorf("Verify(%q): got %v, want ErrInvalidHash", bad, err)
		}
	}
}

// argon2i hashes (what an upstream Laravel app produces) must verify, and be
// flagged for migration to the native argon2id scheme.
func TestVerifyArgon2iAndNeedsRehash(t *testing.T) {
	// A real argon2i PHC hash of "S3cret!" (produced by golang.org/x/crypto
	// argon2.Key with m=65536,t=3,p=4, 16-byte salt, 32-byte key).
	salt := make([]byte, 16)
	key := argon2.Key([]byte("S3cret!"), salt, 3, 65536, 4, 32)
	argon2iHash := fmt.Sprintf("$argon2i$v=%d$m=65536,t=3,p=4$%s$%s",
		argon2.Version,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key))

	if err := Verify("S3cret!", argon2iHash); err != nil {
		t.Fatalf("argon2i verify (correct password): %v", err)
	}
	if err := Verify("wrong", argon2iHash); err != ErrMismatch {
		t.Fatalf("argon2i verify (wrong password): got %v, want ErrMismatch", err)
	}
	if !NeedsRehash(argon2iHash) {
		t.Fatal("argon2i hash should need rehash to native argon2id")
	}

	// A native argon2id hash with default params does NOT need a rehash.
	native, err := Hash("S3cret!")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if NeedsRehash(native) {
		t.Fatal("native argon2id default-params hash should not need rehash")
	}
	// argon2id with non-default params DOES need a rehash.
	weak, _ := HashWithParams("S3cret!", Params{Memory: 4096, Iterations: 1, Parallelism: 1, KeyLength: 32, SaltLength: 16})
	if !NeedsRehash(weak) {
		t.Fatal("argon2id with non-default params should need rehash")
	}
}
