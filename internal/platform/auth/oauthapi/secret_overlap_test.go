package oauthapi

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
)

// overlapState builds a State with a real encryption service so
// acceptClientSecret exercises the actual decrypt+compare path.
func overlapState(t *testing.T) *State {
	t.Helper()
	// 32 raw bytes, base64-encoded, as makeAEAD requires.
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	enc, err := encryption.New(key)
	if err != nil {
		t.Fatalf("encryption: %v", err)
	}
	s := testState(t)
	s.Encryption = enc
	return s
}

func encrypted(t *testing.T, s *State, plaintext string) string {
	t.Helper()
	ref, err := s.Encryption.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return ref
}

// TestAcceptClientSecretHonoursRotationOverlap: during the overlap both the new
// and the outgoing secret authenticate. Without this, rotation is a hard
// cutover — every service still holding the old secret 401s the moment rotate
// returns, so the fleet can't be rolled gradually.
func TestAcceptClientSecretHonoursRotationOverlap(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	if !s.acceptClientSecret(c, "new-secret") {
		t.Error("current secret rejected")
	}
	if !s.acceptClientSecret(c, "old-secret") {
		t.Error("outgoing secret rejected inside its overlap window")
	}
	if s.acceptClientSecret(c, "never-a-secret") {
		t.Error("an unrelated secret was accepted")
	}
}

// TestAcceptClientSecretRefusesLapsedSecret: once the window closes the old
// secret must stop working, even though the row still carries it until purged.
func TestAcceptClientSecretRefusesLapsedSecret(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	past := time.Now().UTC().Add(-time.Second)
	c.PreviousSecretExpiresAt = &past

	if s.acceptClientSecret(c, "old-secret") {
		t.Error("lapsed secret still authenticates")
	}
	if !s.acceptClientSecret(c, "new-secret") {
		t.Error("current secret rejected")
	}
}

// TestAcceptClientSecretAfterRevoke: the explicit revoke closes the overlap
// immediately.
func TestAcceptClientSecretAfterRevoke(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)
	c.RevokePreviousSecret()

	if s.acceptClientSecret(c, "old-secret") {
		t.Error("revoked secret still authenticates")
	}
	if !s.acceptClientSecret(c, "new-secret") {
		t.Error("current secret rejected")
	}
}

// TestAcceptClientSecretImmediateCutover: rotating with no grace leaves the old
// secret dead on arrival — the containment path.
func TestAcceptClientSecretImmediateCutover(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), 0)

	if s.acceptClientSecret(c, "old-secret") {
		t.Error("old secret survived an immediate cutover")
	}
	if !s.acceptClientSecret(c, "new-secret") {
		t.Error("current secret rejected")
	}
}

// TestAcceptClientSecretNoEncryptionFailsClosed keeps the existing fail-closed
// behaviour: with no encryption service configured nothing authenticates,
// including via the overlap branch.
func TestAcceptClientSecretNoEncryptionFailsClosed(t *testing.T) {
	s := overlapState(t)
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	s.Encryption = nil

	if s.acceptClientSecret(c, "new-secret") || s.acceptClientSecret(c, "old-secret") {
		t.Error("verification did not fail closed without an encryption service")
	}
}
