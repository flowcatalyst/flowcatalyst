package oauthapi

import (
	"context"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

// recordingToucher captures the signal without a database.
type recordingToucher struct {
	ids  []string
	fail error
}

func (r *recordingToucher) TouchPreviousSecretUsed(_ context.Context, id string, _, _ time.Time) error {
	r.ids = append(r.ids, id)
	return r.fail
}

// TestPreviousSecretUseIsRecorded: authenticating on the superseded secret must
// leave a trace. Without it the drawer can only show a countdown and revoking is
// a guess — you cannot answer "who still hasn't redeployed?" until the window
// closes and the 401s start.
func TestPreviousSecretUseIsRecorded(t *testing.T) {
	s := overlapState(t)
	rec := &recordingToucher{}
	s.OAuthClientWrites = rec
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	if !s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Fatal("the outgoing secret must still authenticate inside its window")
	}
	if len(rec.ids) != 1 || rec.ids[0] != "oac_1" {
		t.Fatalf("expected one recorded use for oac_1, got %v", rec.ids)
	}
}

// TestCurrentSecretUseIsNotRecorded: the signal must mean "someone is still on
// the old secret". Recording a current-secret match would make it meaningless.
func TestCurrentSecretUseIsNotRecorded(t *testing.T) {
	s := overlapState(t)
	rec := &recordingToucher{}
	s.OAuthClientWrites = rec
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	if !s.acceptClientSecret(context.Background(), c, "new-secret") {
		t.Fatal("the current secret must authenticate")
	}
	if len(rec.ids) != 0 {
		t.Errorf("current-secret use must not be recorded; got %v", rec.ids)
	}
}

// TestLapsedPreviousSecretIsNotRecorded: a secret past its window doesn't
// authenticate, so there is nothing to report.
func TestLapsedPreviousSecretIsNotRecorded(t *testing.T) {
	s := overlapState(t)
	rec := &recordingToucher{}
	s.OAuthClientWrites = rec
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)
	past := time.Now().UTC().Add(-time.Second)
	c.PreviousSecretExpiresAt = &past

	if s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Fatal("a lapsed secret must not authenticate")
	}
	if len(rec.ids) != 0 {
		t.Errorf("nothing to record for a lapsed secret; got %v", rec.ids)
	}
}

// TestRecordingFailureDoesNotFailAuthentication: the signal is observability.
// Losing it must never cost a client a valid authentication.
func TestRecordingFailureDoesNotFailAuthentication(t *testing.T) {
	s := overlapState(t)
	s.OAuthClientWrites = &recordingToucher{fail: context.DeadlineExceeded}
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	if !s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Error("a failed signal write must not reject a valid secret")
	}
}

// TestNilToucherIsSafe: the signal is optional; authentication is not.
func TestNilToucherIsSafe(t *testing.T) {
	s := overlapState(t)
	s.OAuthClientWrites = nil
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef(encrypted(t, s, "old-secret"))
	c.RotateSecretRef(encrypted(t, s, "new-secret"), time.Hour)

	if !s.acceptClientSecret(context.Background(), c, "old-secret") {
		t.Error("authentication must work with no toucher configured")
	}
}

// TestRotateAndRevokeClearTheSignal: a new overlap starts unused, and revoking
// clears the field — a stale "last used" would read as someone still being on a
// secret that no longer exists.
func TestRotateAndRevokeClearTheSignal(t *testing.T) {
	c := &auth.OAuthClient{ID: "oac_1", ClientType: auth.OAuthClientConfidential}
	c.SetSecretRef("ref-v1")
	c.RotateSecretRef("ref-v2", time.Hour)
	used := time.Now().UTC()
	c.PreviousSecretLastUsedAt = &used

	c.RotateSecretRef("ref-v3", time.Hour)
	if c.PreviousSecretLastUsedAt != nil {
		t.Error("a fresh overlap must start unused")
	}

	c.PreviousSecretLastUsedAt = &used
	c.RevokePreviousSecret()
	if c.PreviousSecretLastUsedAt != nil {
		t.Error("revoke must clear the last-used signal with the secret")
	}
}
