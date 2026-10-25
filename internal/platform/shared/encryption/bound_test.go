package encryption_test

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// A bound value opens only with the aad it was sealed with; a value sealed
// before binding is still read and reported unbound; a bound value survives a
// key rotation through the previous key.
func TestEncryptBound(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, err := encryption.New(key)
	if err != nil {
		t.Fatal(err)
	}
	aad := []byte("row-a")
	ct, err := svc.EncryptBound("s3cr3t", aad)
	if err != nil {
		t.Fatal(err)
	}
	pt, bound, err := svc.DecryptBound(ct, aad)
	if err != nil || pt != "s3cr3t" || !bound {
		t.Fatalf("round trip: %q bound=%v err=%v", pt, bound, err)
	}
	pt, bound, err = svc.DecryptBound("encrypted:"+ct, aad)
	if err != nil || pt != "s3cr3t" || !bound {
		t.Fatalf("with the marker: %q bound=%v err=%v", pt, bound, err)
	}
	if _, _, err := svc.DecryptBound(ct, []byte("row-b")); err == nil {
		t.Fatal("a bound value must not open with another row's aad")
	}
	if _, err := svc.Decrypt(ct); err == nil {
		t.Fatal("a bound value must not open without its aad")
	}

	legacy, err := svc.Encrypt("old")
	if err != nil {
		t.Fatal(err)
	}
	pt, bound, err = svc.DecryptBound(legacy, aad)
	if err != nil || pt != "old" || bound {
		t.Fatalf("legacy value: %q bound=%v err=%v", pt, bound, err)
	}

	next, _ := encryption.GenerateKey()
	rotated, err := encryption.WithPreviousKeys(next, []string{key})
	if err != nil {
		t.Fatal(err)
	}
	pt, bound, err = rotated.DecryptBound(ct, aad)
	if err != nil || pt != "s3cr3t" || !bound {
		t.Fatalf("after rotation: %q bound=%v err=%v", pt, bound, err)
	}
}
