package encryption_test

import (
	"strings"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

func TestRoundTrip(t *testing.T) {
	key, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	svc, err := encryption.New(key)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	pt := "super-secret-oauth-client-secret"
	ct, err := svc.Encrypt(pt)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if ct == pt {
		t.Fatalf("ciphertext equals plaintext")
	}
	got, err := svc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != pt {
		t.Fatalf("round-trip mismatch: got %q want %q", got, pt)
	}
}

func TestNonceUniqueness(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	a, _ := svc.Encrypt("same")
	b, _ := svc.Encrypt("same")
	if a == b {
		t.Fatalf("two encryptions of the same input must differ: %s", a)
	}
}

func TestInvalidKeyLength(t *testing.T) {
	// 16-byte key (AES-128 instead of AES-256)
	shortKey := "AAAAAAAAAAAAAAAAAAAAAA==" // base64 of 16 zero bytes
	if _, err := encryption.New(shortKey); err == nil {
		t.Fatalf("New must reject non-32-byte key")
	}
}

func TestKeyRotationDecryptWithPrevious(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()

	oldSvc, _ := encryption.New(oldKey)
	ct, _ := oldSvc.Encrypt("secret-data")

	newSvc, err := encryption.WithPreviousKeys(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("WithPreviousKeys: %v", err)
	}
	got, err := newSvc.Decrypt(ct)
	if err != nil {
		t.Fatalf("Decrypt with previous: %v", err)
	}
	if got != "secret-data" {
		t.Fatalf("got %q", got)
	}
}

func TestKeyRotationNewEncryptionsUseCurrent(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()

	newSvc, _ := encryption.WithPreviousKeys(newKey, []string{oldKey})
	ct, _ := newSvc.Encrypt("new-data")

	// current-only service: must decrypt
	currentOnly, _ := encryption.New(newKey)
	got, err := currentOnly.Decrypt(ct)
	if err != nil || got != "new-data" {
		t.Fatalf("current-only decrypt failed: %v / %q", err, got)
	}

	// old-only service: must fail
	oldOnly, _ := encryption.New(oldKey)
	if _, err := oldOnly.Decrypt(ct); err == nil {
		t.Fatalf("old-only must not decrypt new data")
	}
}

func TestReEncryptMigratesToCurrent(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()

	oldSvc, _ := encryption.New(oldKey)
	oldCT, _ := oldSvc.Encrypt("migrate-me")

	newSvc, _ := encryption.WithPreviousKeys(newKey, []string{oldKey})
	newCT, err := newSvc.ReEncrypt(oldCT)
	if err != nil {
		t.Fatalf("ReEncrypt: %v", err)
	}
	currentOnly, _ := encryption.New(newKey)
	got, err := currentOnly.Decrypt(newCT)
	if err != nil || got != "migrate-me" {
		t.Fatalf("migrated value not decryptable: %v / %q", err, got)
	}
}

func TestNeedsReEncryption(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()

	oldSvc, _ := encryption.New(oldKey)
	oldCT, _ := oldSvc.Encrypt("check-me")

	newSvc, _ := encryption.WithPreviousKeys(newKey, []string{oldKey})

	if !newSvc.NeedsReEncryption(oldCT) {
		t.Fatalf("old-key data should be flagged for re-encryption")
	}
	fresh, _ := newSvc.Encrypt("fresh")
	if newSvc.NeedsReEncryption(fresh) {
		t.Fatalf("fresh data must not need re-encryption")
	}
}

func TestEncryptedPrefixStripped(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	ct, _ := svc.Encrypt("hello")
	got, err := svc.Decrypt("encrypted:" + ct)
	if err != nil {
		t.Fatalf("Decrypt(encrypted: ...): %v", err)
	}
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestFromEnvAbsent(t *testing.T) {
	t.Setenv("FLOWCATALYST_APP_KEY", "")
	t.Setenv("FLOWCATALYST_APP_KEY_PREVIOUS", "")
	svc, err := encryption.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv with empty env: %v", err)
	}
	if svc != nil {
		t.Fatalf("FromEnv with empty env must return nil service")
	}
}

func TestFromEnvWithKey(t *testing.T) {
	key, _ := encryption.GenerateKey()
	t.Setenv("FLOWCATALYST_APP_KEY", key)
	t.Setenv("FLOWCATALYST_APP_KEY_PREVIOUS", "")
	svc, err := encryption.FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if svc == nil {
		t.Fatalf("FromEnv must return non-nil when key present")
	}
	ct, err := svc.Encrypt("hello")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, "") || ct == "" {
		t.Fatalf("Encrypt produced empty result")
	}
}
