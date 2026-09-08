package encryption_test

import (
	"crypto/sha256"
	"encoding/base64"
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

// ── Hash / VerifySecret (keyed-hash form for verify-only secrets) ─────────

// TestGoldenHashVector pins the exact wire value for a fixed key/plaintext
// pair so the Java port can be checked byte-for-byte against it: key = bytes
// 0x00..0x1f, plaintext = "client-secret-golden". Breaking Hash's key
// derivation, the HMAC construction, or the "hashed:v1:" prefix in any way
// changes this literal and fails the test.
func TestGoldenHashVector(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	keyB64 := base64.StdEncoding.EncodeToString(key)
	if keyB64 != "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8=" {
		t.Fatalf("golden key base64 changed: %s", keyB64)
	}
	svc, err := encryption.New(keyB64)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	const want = "hashed:v1:HhInGB9kwvg6VsfBL0oHER0eslXRAg6GBwoTsRa2D4E="
	got := svc.Hash("client-secret-golden")
	if got != want {
		t.Fatalf("golden hash vector mismatch:\n got:  %s\n want: %s", got, want)
	}
	ok, rehash := svc.VerifySecret(got, "client-secret-golden")
	if !ok || rehash {
		t.Fatalf("golden vector must verify against its own plaintext under the current key: ok=%v rehash=%v", ok, rehash)
	}
}

// TestHashIsKeyedNotPlainSHA256 pins that Hash is a KEYED MAC, not an
// unkeyed digest of the plaintext: two different keys over the same
// plaintext must produce different stored values, and neither may equal the
// plaintext's bare SHA-256. This is what mutant (c) in the task ("unkeyed
// SHA-256") breaks: an unkeyed implementation would make every key produce
// the same hash, and that hash would match a bare SHA-256 of the plaintext.
func TestHashIsKeyedNotPlainSHA256(t *testing.T) {
	keyA, _ := encryption.GenerateKey()
	keyB, _ := encryption.GenerateKey()
	svcA, err := encryption.New(keyA)
	if err != nil {
		t.Fatalf("New(A): %v", err)
	}
	svcB, err := encryption.New(keyB)
	if err != nil {
		t.Fatalf("New(B): %v", err)
	}

	const plaintext = "same-plaintext-different-keys"
	hashA := svcA.Hash(plaintext)
	hashB := svcB.Hash(plaintext)
	if hashA == hashB {
		t.Fatalf("Hash must be keyed: two different keys produced the same hash: %s", hashA)
	}

	sum := sha256.Sum256([]byte(plaintext))
	unkeyed := "hashed:v1:" + base64.StdEncoding.EncodeToString(sum[:])
	if hashA == unkeyed || hashB == unkeyed {
		t.Fatalf("Hash must not equal a bare (unkeyed) SHA-256 of the plaintext")
	}
}

// TestVerifySecretHashedCurrentKeyMatchNoRehash: a secret hashed under the
// current key verifies and needs no migration.
func TestVerifySecretHashedCurrentKeyMatchNoRehash(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	stored := svc.Hash("s3cr3t")

	ok, rehash := svc.VerifySecret(stored, "s3cr3t")
	if !ok {
		t.Fatal("correct secret must verify against its own hash")
	}
	if rehash {
		t.Fatal("a ref already hashed under the current key must not be flagged for rehash")
	}
}

// TestVerifySecretHashedWrongSecretFails: a hashed ref never falls back to
// any other check on mismatch — wrong input simply fails.
func TestVerifySecretHashedWrongSecretFails(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	stored := svc.Hash("s3cr3t")

	ok, rehash := svc.VerifySecret(stored, "not-the-secret")
	if ok || rehash {
		t.Fatalf("wrong secret must fail cleanly, got ok=%v rehash=%v", ok, rehash)
	}
}

// TestVerifySecretHashedMalformedMACFails: hashed: is a closed claim — bad
// base64 after the prefix must fail closed, not panic or fall through to
// decrypt.
func TestVerifySecretHashedMalformedMACFails(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)

	ok, rehash := svc.VerifySecret("hashed:v1:not-valid-base64!!!", "anything")
	if ok || rehash {
		t.Fatalf("malformed hashed: ref must fail closed, got ok=%v rehash=%v", ok, rehash)
	}
}

// TestVerifySecretHashedPreviousKeyRehashes: rotation catch-up. A ref hashed
// under a superseded key still verifies (previous key tried after current),
// and is flagged for rehash so the caller can rewrite it under the new key.
func TestVerifySecretHashedPreviousKeyRehashes(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	oldSvc, _ := encryption.New(oldKey)
	stored := oldSvc.Hash("rotate-me")

	rotatingSvc, err := encryption.WithPreviousKeys(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("WithPreviousKeys: %v", err)
	}
	ok, rehash := rotatingSvc.VerifySecret(stored, "rotate-me")
	if !ok {
		t.Fatal("a ref hashed under a previous key must still verify")
	}
	if !rehash {
		t.Fatal("a previous-key match must be flagged for rehash under the current key")
	}

	// The pin that matters: rehashing actually produces a ref that a
	// current-key-only service (no previous keys at all) can verify.
	rehashed := rotatingSvc.Hash("rotate-me")
	currentOnly, _ := encryption.New(newKey)
	ok2, rehash2 := currentOnly.VerifySecret(rehashed, "rotate-me")
	if !ok2 || rehash2 {
		t.Fatalf("rehashed ref must verify under the current key alone with nothing left to migrate: ok=%v rehash=%v", ok2, rehash2)
	}
}

// TestVerifySecretHashedNeitherKeyMatchesFails: current AND previous both
// miss → clean failure, not a panic or a false positive.
func TestVerifySecretHashedNeitherKeyMatchesFails(t *testing.T) {
	oldKey, _ := encryption.GenerateKey()
	newKey, _ := encryption.GenerateKey()
	unrelatedKey, _ := encryption.GenerateKey()
	unrelatedSvc, _ := encryption.New(unrelatedKey)
	stored := unrelatedSvc.Hash("secret")

	svc, err := encryption.WithPreviousKeys(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("WithPreviousKeys: %v", err)
	}
	ok, rehash := svc.VerifySecret(stored, "secret")
	if ok || rehash {
		t.Fatalf("a ref hashed under a key not held at all must fail, got ok=%v rehash=%v", ok, rehash)
	}
}

// TestVerifySecretLegacyEncryptedMatchMigrates: the transparent-migration
// contract — an older reversibly-encrypted ref still verifies (decrypt +
// constant-time compare, exactly as before this type existed), and is always
// flagged for rehash so the caller lazily upgrades it to hashed:v1:.
func TestVerifySecretLegacyEncryptedMatchMigrates(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	ct, err := svc.Encrypt("legacy-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	stored := "encrypted:" + ct

	ok, rehash := svc.VerifySecret(stored, "legacy-secret")
	if !ok {
		t.Fatal("a legacy encrypted: ref must still verify")
	}
	if !rehash {
		t.Fatal("a legacy match must always be flagged for migration to hashed:v1:")
	}
}

// TestVerifySecretLegacyBareEnvelopeMatchMigrates: some rows in the wild
// hold a bare v1 envelope with no "encrypted:" prefix (see docs/spec/
// encryption.md §3) — VerifySecret must accept that shape too.
func TestVerifySecretLegacyBareEnvelopeMatchMigrates(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	bare, err := svc.Encrypt("bare-legacy-secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	ok, rehash := svc.VerifySecret(bare, "bare-legacy-secret")
	if !ok || !rehash {
		t.Fatalf("bare legacy envelope must verify and be flagged for migration, got ok=%v rehash=%v", ok, rehash)
	}
}

// TestVerifySecretLegacyWrongSecretFails: wrong input against a legacy ref
// fails, and is never (mis)reported as needing rehash.
func TestVerifySecretLegacyWrongSecretFails(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)
	ct, _ := svc.Encrypt("legacy-secret")
	stored := "encrypted:" + ct

	ok, rehash := svc.VerifySecret(stored, "not-it")
	if ok || rehash {
		t.Fatalf("wrong secret against a legacy ref must fail cleanly, got ok=%v rehash=%v", ok, rehash)
	}
}

// TestVerifySecretEmptyStoredFailsClosed: an absent/empty stored value
// behaves exactly as Decrypt("") does today — a clean failure, not a panic,
// and no migration to try.
func TestVerifySecretEmptyStoredFailsClosed(t *testing.T) {
	key, _ := encryption.GenerateKey()
	svc, _ := encryption.New(key)

	ok, rehash := svc.VerifySecret("", "anything")
	if ok || rehash {
		t.Fatalf("empty stored value must fail closed, got ok=%v rehash=%v", ok, rehash)
	}
}
