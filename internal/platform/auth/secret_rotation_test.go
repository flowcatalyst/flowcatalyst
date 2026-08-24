package auth

import (
	"testing"
	"time"
)

func confidentialWithSecret(ref string) *OAuthClient {
	c := &OAuthClient{ID: "oac_1", ClientType: OAuthClientConfidential}
	c.SetSecretRef(ref)
	return c
}

// TestRotateKeepsPreviousSecretUsable is the point of the overlap: immediately
// after a rotation both secrets authenticate, so a fleet holding the old one
// can be rolled gradually instead of being cut off the instant rotate returns.
func TestRotateKeepsPreviousSecretUsable(t *testing.T) {
	c := confidentialWithSecret("ref-old")

	expires := c.RotateSecretRef("ref-new", time.Hour)

	if expires == nil {
		t.Fatal("expected an overlap expiry")
	}
	if c.SecretRef == nil || *c.SecretRef != "ref-new" {
		t.Errorf("current secret = %v, want ref-new", c.SecretRef)
	}
	prev := c.UsablePreviousSecretRef()
	if prev == nil || *prev != "ref-old" {
		t.Fatalf("previous secret = %v, want ref-old still usable", prev)
	}
	if !expires.After(time.Now().UTC()) {
		t.Errorf("expiry %v is not in the future", expires)
	}
}

// TestPreviousSecretLapses: once the window closes the old secret stops being
// offered, even though the row still carries it until the purger clears it.
// UsablePreviousSecretRef is what enforces that, which is why verification must
// go through it rather than reading the field.
func TestPreviousSecretLapses(t *testing.T) {
	c := confidentialWithSecret("ref-old")
	c.RotateSecretRef("ref-new", time.Hour)

	// Wind the window shut without sleeping.
	past := time.Now().UTC().Add(-time.Second)
	c.PreviousSecretExpiresAt = &past

	if got := c.UsablePreviousSecretRef(); got != nil {
		t.Errorf("expired previous secret still offered: %v", *got)
	}
	if c.PreviousSecretRef == nil {
		t.Error("the ref should still be present on the row until purged")
	}
}

// TestRotateImmediateCutover: a non-positive grace keeps no overlap at all —
// the containment path for a secret believed compromised.
func TestRotateImmediateCutover(t *testing.T) {
	for _, grace := range []time.Duration{0, -time.Hour} {
		c := confidentialWithSecret("ref-old")

		expires := c.RotateSecretRef("ref-new", grace)

		if expires != nil {
			t.Errorf("grace %v: expected no overlap, got expiry %v", grace, expires)
		}
		if c.PreviousSecretRef != nil {
			t.Errorf("grace %v: old secret retained", grace)
		}
		if c.SecretRef == nil || *c.SecretRef != "ref-new" {
			t.Errorf("grace %v: current secret = %v", grace, c.SecretRef)
		}
	}
}

// TestRevokePreviousSecret: the explicit "close the second door now" action,
// and it must be idempotent so an operator can call it twice or after the timer
// has already fired.
func TestRevokePreviousSecret(t *testing.T) {
	c := confidentialWithSecret("ref-old")
	c.RotateSecretRef("ref-new", time.Hour)

	if !c.RevokePreviousSecret() {
		t.Fatal("expected the first revoke to report a dropped secret")
	}
	if c.UsablePreviousSecretRef() != nil || c.PreviousSecretRef != nil {
		t.Error("previous secret survived revoke")
	}
	if c.SecretRef == nil || *c.SecretRef != "ref-new" {
		t.Error("revoke must not disturb the current secret")
	}
	if c.RevokePreviousSecret() {
		t.Error("second revoke should report nothing to drop")
	}
}

// TestRotateTwiceRetiresTheOldestSecret: exactly one previous secret is
// honoured, so rotating again inside a window drops the secret from two
// rotations ago rather than accumulating a chain of valid credentials.
func TestRotateTwiceRetiresTheOldestSecret(t *testing.T) {
	c := confidentialWithSecret("ref-v1")
	c.RotateSecretRef("ref-v2", time.Hour)
	c.RotateSecretRef("ref-v3", time.Hour)

	if c.SecretRef == nil || *c.SecretRef != "ref-v3" {
		t.Errorf("current = %v, want ref-v3", c.SecretRef)
	}
	prev := c.UsablePreviousSecretRef()
	if prev == nil || *prev != "ref-v2" {
		t.Fatalf("previous = %v, want ref-v2", prev)
	}
}

// TestSetSecretRefClearsOverlap: provisioning a secret outright must not leave
// an older one acceptable.
func TestSetSecretRefClearsOverlap(t *testing.T) {
	c := confidentialWithSecret("ref-old")
	c.RotateSecretRef("ref-new", time.Hour)

	c.SetSecretRef("ref-provisioned")

	if c.PreviousSecretRef != nil || c.PreviousSecretExpiresAt != nil {
		t.Error("SetSecretRef left an overlap in flight")
	}
}

// TestRotateFromNoSecretKeepsNoOverlap: a client with nothing to demote gets a
// plain assignment, not an overlap pointing at nil.
func TestRotateFromNoSecretKeepsNoOverlap(t *testing.T) {
	c := &OAuthClient{ID: "oac_1", ClientType: OAuthClientConfidential}

	if expires := c.RotateSecretRef("ref-first", time.Hour); expires != nil {
		t.Errorf("expected no overlap, got %v", expires)
	}
	if c.PreviousSecretRef != nil {
		t.Error("overlap recorded with no prior secret")
	}
}
