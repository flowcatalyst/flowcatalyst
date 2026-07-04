package oauthapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

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
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil)
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
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil)
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
		tok, err := s.mintIDToken(context.Background(), p, client.ClientID, client, nil)
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
		tok, err := s.mintIDToken(context.Background(), p, "clt_rp", nil, nil)
		if err != nil {
			t.Fatalf("mintIDToken: %v", err)
		}
		roles := decodeRoles(t, tok)
		if len(roles) != 2 {
			t.Errorf("roles = %v, want both roles (nil client)", roles)
		}
	})
}
