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

// The published corpus is curated and ordered by filename prefix; slugs
// drop the prefix, titles come from the first heading, and the reading
// order starts at the overview.
func TestPublishedListAndGet(t *testing.T) {
	s := &State{}
	out, err := s.list(anchorCtx(), &apicommon.Empty{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(out.Body.Platform) == 0 {
		t.Fatal("expected published platform docs, got none")
	}
	if out.Body.Platform[0].Slug != "platform-overview" {
		t.Errorf("first doc should be the overview, got %q", out.Body.Platform[0].Slug)
	}
	found := false
	for _, d := range out.Body.Platform {
		if d.Slug == "portal-users" && d.Title == "Portal Users Architecture" {
			found = true
		}
	}
	if !found {
		t.Fatal("portal-users missing from the published set")
	}
	if out.Body.Applications == nil {
		t.Error("applications must be [] (not null) when unwired")
	}

	doc, err := s.getPlatform(anchorCtx(), &getPlatformDocInput{Slug: "platform-overview"})
	if err != nil {
		t.Fatalf("getPlatform: %v", err)
	}
	if doc.Body.Title != "Platform Overview" || doc.Body.Content == "" {
		t.Fatalf("unexpected doc: title=%q len=%d", doc.Body.Title, len(doc.Body.Content))
	}
}

// Order-prefixed filenames must not leak into slugs, and unknown or
// traversal-shaped slugs are NotFound.
func TestPublishedGetRejectsBadSlugs(t *testing.T) {
	s := &State{}
	for _, slug := range []string{"10-platform-overview", "nope", "../embed.go", "published/10-platform-overview"} {
		if _, err := s.getPlatform(anchorCtx(), &getPlatformDocInput{Slug: slug}); err == nil {
			t.Errorf("slug %q: expected error, got doc", slug)
		}
	}
}

// Unauthenticated and permission-less client-scoped callers are refused;
// a client-scoped caller holding the docs permission passes.
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
