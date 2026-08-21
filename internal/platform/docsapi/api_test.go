package docsapi

import (
	"context"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

func anchorCtx() context.Context {
	return auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_docstest", Scope: auth.ScopeAnchor,
	})
}

// The embedded corpus always contains the architecture doc; list and get
// must agree on it, and get must return real Markdown.
func TestDocsListAndGet(t *testing.T) {
	s := &State{}
	out, err := s.list(anchorCtx(), &apicommon.Empty{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Body.Docs) == 0 {
		t.Fatal("expected embedded docs, got none")
	}
	found := false
	for _, d := range out.Body.Docs {
		if d.Slug == "portal-users-architecture" {
			found = true
			if d.Title != "Portal Users Architecture" {
				t.Errorf("title from first heading: got %q", d.Title)
			}
		}
	}
	if !found {
		t.Fatal("portal-users-architecture missing from list")
	}

	doc, err := s.get(anchorCtx(), &getDocInput{Slug: "portal-users-architecture"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if doc.Body.Content == "" || doc.Body.Title != "Portal Users Architecture" {
		t.Fatalf("unexpected doc body: title=%q len=%d", doc.Body.Title, len(doc.Body.Content))
	}
}

// Unknown slugs and traversal-shaped slugs are NotFound; embed.go itself
// (a .go file, not .md) must never be served.
func TestDocsGetRejectsBadSlugs(t *testing.T) {
	s := &State{}
	for _, slug := range []string{"nope", "../go.mod", "adr/0001", "embed.go", "a.b"} {
		if _, err := s.get(anchorCtx(), &getDocInput{Slug: slug}); err == nil {
			t.Errorf("slug %q: expected error, got doc", slug)
		}
	}
}

// Unauthenticated and non-anchor-without-permission callers are refused;
// a client-scoped caller holding the permission passes.
func TestDocsAuthorization(t *testing.T) {
	s := &State{}
	if _, err := s.list(context.Background(), &apicommon.Empty{}); err == nil {
		t.Error("unauthenticated list should fail")
	}
	clientCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_docsclient", Scope: auth.ScopeClient,
	})
	if _, err := s.list(clientCtx, &apicommon.Empty{}); err == nil {
		t.Error("client-scoped caller without permission should fail")
	}
	grantedCtx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_docsclient", Scope: auth.ScopeClient,
		Permissions: []string{"platform:admin:docs:view"},
	})
	if _, err := s.list(grantedCtx, &apicommon.Empty{}); err != nil {
		t.Errorf("client-scoped caller WITH permission refused: %v", err)
	}
}
