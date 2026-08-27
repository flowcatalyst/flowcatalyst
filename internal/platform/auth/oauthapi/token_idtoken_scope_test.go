package oauthapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

func testAuthService(t *testing.T) *authservice.AuthService {
	t.Helper()
	cfg := authservice.DefaultConfig()
	cfg.SecretKey = "test-secret-at-least-32-bytes-long!!"
	svc, err := authservice.New(cfg)
	if err != nil {
		t.Fatalf("authservice.New: %v", err)
	}
	return svc
}

func decodeRoles(t *testing.T, token string) []string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload struct {
		Roles []string `json:"roles"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload.Roles
}

func TestMintIDToken(t *testing.T) {
	p := principal.NewUser("u@example.com", principal.ScopeClient)
	p.Roles = []serviceaccount.RoleAssignment{
		{Role: "za-logistics:orders-admin"},
		{Role: "za-billing:viewer"},
	}

	// fixedFilter mimics provider.FilterRolesForApplications: keeps only the
	// role names present in `keep`, ignoring the appIDs argument (the test
	// stands in for role→application resolution).
	fixedFilter := func(keep ...string) func(context.Context, []string, []string) ([]string, error) {
		return func(_ context.Context, roleNames []string, appIDs []string) ([]string, error) {
			out := make([]string, 0, len(roleNames))
			for _, r := range roleNames {
				for _, k := range keep {
					if r == k {
						out = append(out, r)
					}
				}
			}
			return out, nil
		}
	}

	t.Run("client with no ApplicationIDs gets full unfiltered roles", func(t *testing.T) {
		s := &State{
			Auth:                       testAuthService(t),
			FilterRolesForApplications: fixedFilter("za-logistics:orders-admin"),
		}
		client := auth.NewOAuthClient("clt_rp", "RP", auth.OAuthClientConfidential)
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		roles := decodeRoles(t, tok)
		if len(roles) != 2 {
			t.Errorf("roles = %v, want both roles (unscoped client, no narrowing)", roles)
		}
	})

	t.Run("app-scoped client gets roles narrowed to its application", func(t *testing.T) {
		s := &State{
			Auth:                       testAuthService(t),
			FilterRolesForApplications: fixedFilter("za-logistics:orders-admin"),
		}
		client := auth.NewOAuthClient("clt_rp", "RP", auth.OAuthClientConfidential)
		client.ApplicationIDs = []string{"app_za_logistics"}
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		roles := decodeRoles(t, tok)
		if len(roles) != 1 || roles[0] != "za-logistics:orders-admin" {
			t.Errorf("roles = %v, want [za-logistics:orders-admin]", roles)
		}
	})

	t.Run("FilterRolesForApplications unwired falls back to full roles", func(t *testing.T) {
		s := &State{Auth: testAuthService(t)} // FilterRolesForApplications nil
		client := auth.NewOAuthClient("clt_rp", "RP", auth.OAuthClientConfidential)
		client.ApplicationIDs = []string{"app_za_logistics"}
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		roles := decodeRoles(t, tok)
		if len(roles) != 2 {
			t.Errorf("roles = %v, want both roles (narrowing not wired)", roles)
		}
	})

	t.Run("nil client falls back to full roles", func(t *testing.T) {
		s := &State{
			Auth:                       testAuthService(t),
			FilterRolesForApplications: fixedFilter("za-logistics:orders-admin"),
		}
		tok, err := s.mintIDToken(context.Background(), p, "clt_rp", nil, nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		roles := decodeRoles(t, tok)
		if len(roles) != 2 {
			t.Errorf("roles = %v, want both roles (nil client)", roles)
		}
	})
}

// TestMintIDTokenConfinesApplications pins the other half of the confinement:
// an ID token tells a relying party what its user holds inside ITS walls, so
// the applications claim must be the intersection with the client's own
// applications — not the principal's full platform-wide list. Roles were
// narrowed from the start; applications were emitted whole, so a user with
// access to three applications handed all three ids to a client scoped to one.
func TestMintIDTokenConfinesApplications(t *testing.T) {
	newPrincipal := func() *principal.Principal {
		p := principal.NewUser("u@example.com", principal.ScopeClient)
		p.Roles = []serviceaccount.RoleAssignment{{Role: "za-logistics:orders-admin"}}
		p.AllApplications = false
		p.AccessibleApplicationIDs = []string{"app_own", "app_other1", "app_other2"}
		return p
	}
	scopedClient := func() *auth.OAuthClient {
		c := auth.NewOAuthClient("clt_rp", "RP", auth.OAuthClientConfidential)
		c.ApplicationIDs = []string{"app_own"}
		return c
	}

	t.Run("app-scoped client sees only its own application", func(t *testing.T) {
		s := &State{Auth: testAuthService(t)}
		tok, err := s.mintIDToken(context.Background(), newPrincipal(), "clt_rp", scopedClient(), nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		apps, all := decodeApplications(t, tok)
		if !reflect.DeepEqual(apps, []string{"app_own"}) {
			t.Errorf("applications = %v, want [app_own]", apps)
		}
		if all {
			t.Error("all_applications must be forced off for an app-scoped client")
		}
	})

	t.Run("all-applications principal is confined to the client's set", func(t *testing.T) {
		p := newPrincipal()
		p.AllApplications = true
		p.AccessibleApplicationIDs = nil
		s := &State{Auth: testAuthService(t)}
		tok, err := s.mintIDToken(context.Background(), p, "clt_rp", scopedClient(), nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		apps, all := decodeApplications(t, tok)
		if !reflect.DeepEqual(apps, []string{"app_own"}) {
			t.Errorf("applications = %v, want [app_own]", apps)
		}
		if all {
			t.Error("all_applications must not survive confinement")
		}
	})

	t.Run("unscoped client keeps the full list", func(t *testing.T) {
		c := auth.NewOAuthClient("clt_rp", "RP", auth.OAuthClientConfidential)
		s := &State{Auth: testAuthService(t)}
		tok, err := s.mintIDToken(context.Background(), newPrincipal(), "clt_rp", c, nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		apps, _ := decodeApplications(t, tok)
		want := []string{"app_own", "app_other1", "app_other2"}
		if !reflect.DeepEqual(apps, want) {
			t.Errorf("applications = %v, want %v", apps, want)
		}
	})

	t.Run("applications narrow even when role filtering is unwired", func(t *testing.T) {
		// Application confinement is set intersection; it does not depend on
		// role→application resolution being available.
		s := &State{Auth: testAuthService(t)} // FilterRolesForApplications nil
		tok, err := s.mintIDToken(context.Background(), newPrincipal(), "clt_rp", scopedClient(), nil, time.Time{})
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		apps, _ := decodeApplications(t, tok)
		if !reflect.DeepEqual(apps, []string{"app_own"}) {
			t.Errorf("applications = %v, want [app_own]", apps)
		}
		if roles := decodeRoles(t, tok); !reflect.DeepEqual(roles, []string{"za-logistics:orders-admin"}) {
			t.Errorf("roles = %v, want the unfiltered list", roles)
		}
	})
}

func decodeApplications(t *testing.T, token string) ([]string, bool) {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %d parts", len(parts))
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	var payload struct {
		Applications    []string `json:"applications"`
		AllApplications bool     `json:"all_applications"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return payload.Applications, payload.AllApplications
}
