package oauthapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// recordingSecretRewriter captures what acceptClientSecret writes back,
// without a database — so tests can assert on the PERSISTED VALUE (what a
// real repository would have stored) rather than merely "a rewrite method
// was called".
type recordingSecretRewriter struct {
	current  map[string]string
	previous map[string]string
	fail     error
}

func newRecordingSecretRewriter() *recordingSecretRewriter {
	return &recordingSecretRewriter{current: map[string]string{}, previous: map[string]string{}}
}

func (r *recordingSecretRewriter) RewriteSecretRef(_ context.Context, id, newRef string) error {
	if r.fail != nil {
		return r.fail
	}
	r.current[id] = newRef
	return nil
}

func (r *recordingSecretRewriter) RewritePreviousSecretRef(_ context.Context, id, newRef string) error {
	if r.fail != nil {
		return r.fail
	}
	r.previous[id] = newRef
	return nil
}

// TestAcceptClientSecretFreshHashedNeedsNoMigration: a client created after
// this change stores hashed:v1:… and authenticates with no rewrite at all —
// there is nothing to migrate.
func TestAcceptClientSecretFreshHashedNeedsNoMigration(t *testing.T) {
	s := overlapState(t)
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(s.Encryption.Hash("new-secret"))

	if !s.acceptClientSecret(context.Background(), c, "new-secret") {
		t.Fatal("a freshly-hashed secret must authenticate")
	}
	if len(rec.current) != 0 {
		t.Errorf("an already-hashed-under-the-current-key ref must not be rewritten; got %v", rec.current)
	}
}

// TestAcceptClientSecretLegacyEncryptedMigratesPersistedValue is the core
// transparent-migration behaviour: a client created before this change
// (an "encrypted:" ref) still authenticates, AND the rewritten ref that a
// real repository would now hold is hashed:v1:… and verifies against the
// same plaintext under the current key. This asserts the PERSISTED VALUE —
// not that RewriteSecretRef was merely invoked — so a rewrite that wrote
// garbage, or the wrong client's row, or nothing usable would still fail
// this test even though "a call happened".
func TestAcceptClientSecretLegacyEncryptedMigratesPersistedValue(t *testing.T) {
	s := overlapState(t)
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec
	c := &auth.OAuthClient{ID: "oac_legacy", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "legacy-secret"))

	if !s.acceptClientSecret(context.Background(), c, "legacy-secret") {
		t.Fatal("a legacy encrypted secret must still authenticate")
	}

	newRef, ok := rec.current["oac_legacy"]
	if !ok {
		t.Fatal("a successful legacy-shape verify must rewrite client_secret_ref")
	}
	if newRef == "" || newRef[:len("hashed:v1:")] != "hashed:v1:" {
		t.Fatalf("rewritten ref must be hashed:v1:…, got %q", newRef)
	}
	okVerify, rehash := s.Encryption.VerifySecret(newRef, "legacy-secret")
	if !okVerify || rehash {
		t.Fatalf("rewritten ref must verify under the current key with nothing left to migrate: ok=%v rehash=%v", okVerify, rehash)
	}
}

// TestAcceptClientSecretPreviousSecretMigratesPreviousRefOnly: a match on the
// rotation-overlap secret must rewrite PreviousSecretRef, never the current
// SecretRef, and must leave the secret + its rotation grace untouched.
func TestAcceptClientSecretPreviousSecretMigratesPreviousRefOnly(t *testing.T) {
	s := overlapState(t)
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec
	c := &auth.OAuthClient{ID: "oac_overlap", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	expires := c.RotateSecretRef(s.Encryption.Hash("new-secret"), time.Hour)
	if expires == nil {
		t.Fatal("rotation must keep a grace window")
	}

	if !s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Fatal("the outgoing secret must still authenticate inside its window")
	}

	if len(rec.current) != 0 {
		t.Errorf("a previous-secret match must not touch client_secret_ref; got %v", rec.current)
	}
	newPrevRef, ok := rec.previous["oac_overlap"]
	if !ok {
		t.Fatal("a successful legacy previous-secret verify must rewrite previous_secret_ref")
	}
	okVerify, rehash := s.Encryption.VerifySecret(newPrevRef, "old-secret")
	if !okVerify || rehash {
		t.Fatalf("rewritten previous ref must verify with nothing left to migrate: ok=%v rehash=%v", okVerify, rehash)
	}
	// The grace window itself (untouched) still governs UsablePreviousSecretRef.
	if got := c.PreviousSecretExpiresAt; got == nil || !got.Equal(*expires) {
		t.Errorf("rotation grace must be untouched by the migration, got %v want %v", got, expires)
	}
}

// TestAcceptClientSecretKeyRotationRehashesUnderCurrentKey: a ref already
// hashed, but under a superseded app key, must verify via the previous-key
// fallback and be rewritten so a current-key-only service (simulating the
// old key being retired entirely) can still verify it afterward.
func TestAcceptClientSecretKeyRotationRehashesUnderCurrentKey(t *testing.T) {
	oldKey, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	newKey, err := encryption.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	oldSvc, err := encryption.New(oldKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	rotatingSvc, err := encryption.WithPreviousKeys(newKey, []string{oldKey})
	if err != nil {
		t.Fatalf("WithPreviousKeys: %v", err)
	}

	s := testState(t)
	s.Encryption = rotatingSvc
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec
	c := &auth.OAuthClient{ID: "oac_rotated", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(oldSvc.Hash("still-good"))

	if !s.acceptClientSecret(context.Background(), c, "still-good") {
		t.Fatal("a secret hashed under a previous (still-configured) key must authenticate")
	}
	newRef, ok := rec.current["oac_rotated"]
	if !ok {
		t.Fatal("a previous-key hashed match must be rewritten under the current key")
	}
	currentOnly, err := encryption.New(newKey)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	okVerify, rehash := currentOnly.VerifySecret(newRef, "still-good")
	if !okVerify {
		t.Fatal("rewritten ref must verify under the current key ALONE (old key retired)")
	}
	if rehash {
		t.Fatal("a ref rewritten under the current key needs no further migration")
	}
}

// TestAcceptClientSecretWrongSecretFailsBothShapesNoRewrite: wrong secret
// fails against both a hashed and a legacy-encrypted ref, and never
// triggers a spurious rewrite.
func TestAcceptClientSecretWrongSecretFailsBothShapesNoRewrite(t *testing.T) {
	s := overlapState(t)
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec

	hashedClient := &auth.OAuthClient{ID: "oac_h", ClientType: auth.OAuthClientConfidential}
	hashedClient.SetSecretRef(s.Encryption.Hash("real-secret"))
	if s.acceptClientSecret(context.Background(), hashedClient, "wrong") {
		t.Error("wrong secret must not authenticate against a hashed ref")
	}

	legacyClient := &auth.OAuthClient{ID: "oac_l", ClientType: auth.OAuthClientConfidential}
	legacyClient.SetSecretRef(encrypted(t, s, "real-secret"))
	if s.acceptClientSecret(context.Background(), legacyClient, "wrong") {
		t.Error("wrong secret must not authenticate against a legacy encrypted ref")
	}

	if len(rec.current) != 0 || len(rec.previous) != 0 {
		t.Errorf("a failed verify must never trigger a rewrite; got current=%v previous=%v", rec.current, rec.previous)
	}
}

// TestAcceptClientSecretRewriteFailureIsNonFatal: authentication already
// succeeded by the time the migration write is attempted — a failure there
// must never turn a valid secret into a rejected one.
func TestAcceptClientSecretRewriteFailureIsNonFatal(t *testing.T) {
	s := overlapState(t)
	rec := newRecordingSecretRewriter()
	rec.fail = errors.New("boom")
	s.SecretRewrites = rec
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "legacy-secret"))

	if !s.acceptClientSecret(context.Background(), c, "legacy-secret") {
		t.Error("a failed migration write must not fail an otherwise-valid authentication")
	}
}

// TestAcceptClientSecretNilRewriterIsSafe: the migration is optional; a nil
// SecretRewrites must not panic and must not block authentication.
func TestAcceptClientSecretNilRewriterIsSafe(t *testing.T) {
	s := overlapState(t)
	s.SecretRewrites = nil
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "legacy-secret"))

	if !s.acceptClientSecret(context.Background(), c, "legacy-secret") {
		t.Error("authentication must work with no rewriter configured")
	}
}

// TestAcceptClientSecretEncryptionDisabledNoRewriteAttempted: with no
// encryption service configured, verification already fails closed
// (pre-existing behaviour); confirm the migration path isn't reached either.
func TestAcceptClientSecretEncryptionDisabledNoRewriteAttempted(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	rec := newRecordingSecretRewriter()
	s.SecretRewrites = rec
	s.Encryption = nil

	if s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Error("verification must fail closed without an encryption service")
	}
	if len(rec.current) != 0 || len(rec.previous) != 0 {
		t.Errorf("no rewrite may be attempted when encryption is disabled; got current=%v previous=%v", rec.current, rec.previous)
	}
}
