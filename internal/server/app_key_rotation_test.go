package server

import (
	"context"
	"crypto/hkdf"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity/pin"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduler"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
)

// legacyDispatchSecret is the derivation dispatchAuthSecret always used:
// HKDF-SHA256 over the app key's text, info "fc-dispatch-auth", hex.
func legacyDispatchSecret(t *testing.T, appKey string) string {
	t.Helper()
	k, err := hkdf.Key(sha256.New, []byte(appKey), nil, "fc-dispatch-auth", 32)
	if err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(k)
}

// After a rotation the callbacks accept tokens signed under the old key (from
// FLOWCATALYST_APP_KEY_PREVIOUS, a list) and the scheduler signs with the new
// one, derived exactly as before so an upgrade breaks no in-flight token.
func TestDispatchAuthAcrossAnAppKeyRotation(t *testing.T) {
	older, _ := encryption.GenerateKey()
	old, _ := encryption.GenerateKey()
	cur, _ := encryption.GenerateKey()
	t.Setenv(encryption.EnvAppKey, cur)
	t.Setenv(encryption.EnvAppKeyPrevious, old+","+older)

	signing, _, err := dispatchAuthSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if signing != legacyDispatchSecret(t, cur) {
		t.Fatal("the signing secret must be derived from the current key exactly as before")
	}
	v, err := newDispatchAuthVerifier()
	if err != nil {
		t.Fatal(err)
	}
	for name, key := range map[string]string{"previous": old, "older": older} {
		tok := scheduler.NewDispatchAuthService(legacyDispatchSecret(t, key)).Sign("job-1")
		if ok, prev := v.Verify("job-1", tok); !ok || !prev {
			t.Errorf("token signed under the %s key: ok=%v previous=%v", name, ok, prev)
		}
	}
	stranger, _ := encryption.GenerateKey()
	if ok, _ := v.Verify("job-1", scheduler.NewDispatchAuthService(legacyDispatchSecret(t, stranger)).Sign("job-1")); ok {
		t.Error("a token under an unconfigured key must be rejected")
	}
}

// The field PIN hasher is built with the previous keys too, so PINs hashed
// before a rotation keep verifying.
func TestFieldOpsHasherAcceptsPINsFromBeforeARotation(t *testing.T) {
	old, _ := encryption.GenerateKey()
	cur, _ := encryption.GenerateKey()
	oldHasher, err := pin.NewHasher(old)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := oldHasher.Hash("clt_1", "fwu_1", "482915")

	t.Setenv(encryption.EnvAppKey, cur)
	t.Setenv(encryption.EnvAppKeyPrevious, old)
	ops := newFieldOps(&repoSet{})
	ok, rehash, err := ops.Hasher.Verify("clt_1", "fwu_1", "482915",
		pin.Stored{Hash: d.Hash, Scheme: pin.SchemePepperV1, PepperID: d.PepperID})
	if err != nil || !ok || !rehash {
		t.Fatalf("ok=%v rehash=%v err=%v; want verified under the previous key and flagged for rehash", ok, rehash, err)
	}
}

// A malformed previous key stops every tier at boot, before any subsystem
// starts — not only the tiers that happen to build the encryption service.
func TestRunRefusesAMalformedPreviousAppKey(t *testing.T) {
	// Run sets the process-wide session cookie setting; put it back so the
	// integration tests that follow see the default (not secure).
	t.Cleanup(func() { platformmw.SetSessionCookieSecure(false) })
	cur, _ := encryption.GenerateKey()
	t.Setenv(encryption.EnvAppKey, cur)
	t.Setenv(encryption.EnvAppKeyPrevious, "not-a-key")
	// Bounded: should the check regress, Run would otherwise serve forever.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	err := Run(ctx, nil, EnvCfg{}, RunOptions{})
	if err == nil || !strings.Contains(err.Error(), "app key") {
		t.Fatalf("Run = %v, want an app key error", err)
	}
}
