package passwordhash

import (
	"strings"
	"testing"
)

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
		"$argon2i$v=19$m=64,t=1,p=4$AAAA$AAAA",  // wrong variant
		"$argon2id$v=18$m=64,t=1,p=4$AAAA$AAAA", // wrong version
		"$argon2id$v=19$m=64,t=1,p=4$!!!!$AAAA", // bad base64 salt
	} {
		if err := Verify("x", bad); err != ErrInvalidHash {
			t.Errorf("Verify(%q): got %v, want ErrInvalidHash", bad, err)
		}
	}
}
