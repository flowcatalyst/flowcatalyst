package provider

import (
	"context"
	"testing"
)

func TestArgon2idHasher_RoundTrip(t *testing.T) {
	h := Argon2idHasher{}
	hash, err := h.Hash(context.Background(), []byte("secret-value"))
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := h.Compare(context.Background(), hash, []byte("secret-value")); err != nil {
		t.Fatalf("expected match, got %v", err)
	}
	if err := h.Compare(context.Background(), hash, []byte("wrong-value")); err == nil {
		t.Fatal("expected mismatch on wrong secret, got nil")
	}
}
