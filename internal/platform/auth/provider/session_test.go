package provider

import (
	"testing"
)

func TestApplyClaims(t *testing.T) {
	s := NewSession()
	s.applyClaims(&Claims{
		Issuer:       "https://issuer.example.com",
		Subject:      "prn_123",
		Scope:        "ANCHOR",
		Clients:      []string{"clt_a", "clt_b"},
		Roles:        []string{"admin"},
		Applications: []string{"app_x"},
		Email:        "user@example.com",
	})

	extras := s.GetExtraClaims()
	// scope lands as a top-level "scope" string via WithScopeField(String).
	if extras["scope"] != "ANCHOR" {
		t.Fatalf("scope: got %v", extras["scope"])
	}
	if extras["email"] != "user@example.com" {
		t.Fatalf("email: got %v", extras["email"])
	}
	if cs, ok := extras["clients"].([]string); !ok || len(cs) != 2 {
		t.Fatalf("clients: got %v", extras["clients"])
	}
	if extras["sub"] != "prn_123" {
		t.Fatalf("sub: got %v", extras["sub"])
	}
	if s.GetSubject() != "prn_123" {
		t.Fatalf("subject: got %q", s.GetSubject())
	}
}

func TestSessionClone(t *testing.T) {
	s := NewSession()
	s.applyClaims(&Claims{Subject: "prn_1", Scope: "PARTNER"})
	clone, ok := s.Clone().(*FCSession)
	if !ok {
		t.Fatal("Clone() did not return *FCSession")
	}
	if clone.GetSubject() != "prn_1" {
		t.Fatalf("clone subject: %q", clone.GetSubject())
	}
}
