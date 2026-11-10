package scheduler

import "testing"

func TestDispatchAuthAcceptsPreviousSecretsButSignsWithTheCurrentOne(t *testing.T) {
	old := NewDispatchAuthService("old-secret")
	rotated := NewDispatchAuthService("new-secret", "older-secret", "old-secret")

	tok := old.Sign("job-1")
	if ok, prev := rotated.Verify("job-1", tok); !ok || !prev {
		t.Fatalf("a token signed before the rotation: ok=%v previous=%v; want accepted under a previous key", ok, prev)
	}
	if ok, _ := rotated.Verify("job-2", tok); ok {
		t.Fatal("a token for another job must not verify")
	}

	fresh := rotated.Sign("job-1")
	if fresh == tok {
		t.Fatal("Sign must use the current secret, not a previous one")
	}
	if ok, prev := rotated.Verify("job-1", fresh); !ok || prev {
		t.Fatalf("a current token: ok=%v previous=%v; want accepted under the current key", ok, prev)
	}
	// A process not yet carrying the new key rejects the new token — the
	// mismatch the rejected counter makes visible.
	if ok, _ := old.Verify("job-1", fresh); ok {
		t.Fatal("the old secret alone must not accept a token signed with the new one")
	}
	if ok, _ := rotated.Verify("job-1", ""); ok {
		t.Fatal("an empty token must not verify")
	}
}
