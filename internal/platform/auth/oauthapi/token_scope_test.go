package oauthapi

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// fixedCeiling returns a State whose FlattenPermissions always yields the given
// permission set, isolating grantedScope's narrowing logic from role lookups.
func fixedCeiling(ceiling ...string) *State {
	return &State{
		FlattenPermissions: func(context.Context, []string) ([]string, error) {
			return ceiling, nil
		},
	}
}

func sortedEqual(a, b []string) bool {
	a = append([]string(nil), a...)
	b = append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	return reflect.DeepEqual(a, b)
}

func TestGrantedScope(t *testing.T) {
	ceiling := []string{
		"platform:messaging:event-type:view",
		"platform:messaging:event-type:create",
		"platform:billing:invoice:read",
	}
	clientUser := func() *principal.Principal { return principal.NewUser("u@example.com", principal.ScopeClient) }

	t.Run("unwired returns no scope", func(t *testing.T) {
		s := &State{} // FlattenPermissions nil
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(), "platform:messaging:event-type:view")
		if err != nil || explicit || granted != nil {
			t.Fatalf("got (%v, %v, %v), want (nil, false, nil)", granted, explicit, err)
		}
	})

	t.Run("omitted scope returns full ceiling", func(t *testing.T) {
		s := fixedCeiling(ceiling...)
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(), "")
		if err != nil {
			t.Fatal(err)
		}
		if explicit {
			t.Error("explicit should be false when no scope requested")
		}
		if !sortedEqual(granted, ceiling) {
			t.Errorf("granted = %v, want full ceiling %v", granted, ceiling)
		}
	})

	t.Run("requested subset is intersected", func(t *testing.T) {
		s := fixedCeiling(ceiling...)
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(),
			"platform:messaging:event-type:view platform:billing:invoice:read")
		if err != nil {
			t.Fatal(err)
		}
		if !explicit {
			t.Error("explicit should be true")
		}
		want := []string{"platform:messaging:event-type:view", "platform:billing:invoice:read"}
		if !sortedEqual(granted, want) {
			t.Errorf("granted = %v, want %v", granted, want)
		}
	})

	t.Run("unheld permission is dropped, never escalates", func(t *testing.T) {
		s := fixedCeiling(ceiling...)
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(),
			"platform:messaging:event-type:view platform:iam:role:delete")
		if err != nil {
			t.Fatal(err)
		}
		if !explicit {
			t.Error("explicit should be true")
		}
		// The unheld iam:role:delete must NOT appear.
		want := []string{"platform:messaging:event-type:view"}
		if !sortedEqual(granted, want) {
			t.Errorf("granted = %v, want %v (no escalation)", granted, want)
		}
	})

	t.Run("requesting only unheld permissions intersects to empty", func(t *testing.T) {
		s := fixedCeiling(ceiling...)
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(), "platform:iam:role:delete")
		if err != nil {
			t.Fatal(err)
		}
		if !explicit || len(granted) != 0 {
			t.Errorf("got (%v, explicit=%v), want (empty, explicit=true) so caller can reject", granted, explicit)
		}
	})

	t.Run("OIDC flow scopes are ignored for narrowing", func(t *testing.T) {
		s := fixedCeiling(ceiling...)
		granted, explicit, err := s.grantedScope(context.Background(), clientUser(), "openid profile offline_access")
		if err != nil {
			t.Fatal(err)
		}
		// No permission scopes requested → full ceiling, not a rejection.
		if explicit {
			t.Error("OIDC-only request must not count as an explicit permission request")
		}
		if !sortedEqual(granted, ceiling) {
			t.Errorf("granted = %v, want full ceiling", granted)
		}
	})

	t.Run("wildcard ceiling covers concrete request", func(t *testing.T) {
		s := fixedCeiling("platform:*:*:*")
		granted, _, err := s.grantedScope(context.Background(), clientUser(), "platform:messaging:event-type:view")
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"platform:messaging:event-type:view"}
		if !sortedEqual(granted, want) {
			t.Errorf("granted = %v, want %v", granted, want)
		}
	})

	t.Run("anchor passes requests through verbatim", func(t *testing.T) {
		// Anchor has an unbounded ceiling even with an empty role-derived set.
		s := fixedCeiling()
		anchor := principal.NewUser("a@example.com", principal.ScopeAnchor)
		granted, explicit, err := s.grantedScope(context.Background(), anchor, "platform:iam:role:delete")
		if err != nil {
			t.Fatal(err)
		}
		if !explicit || !sortedEqual(granted, []string{"platform:iam:role:delete"}) {
			t.Errorf("anchor granted = %v (explicit=%v), want the requested perm", granted, explicit)
		}
	})
}
