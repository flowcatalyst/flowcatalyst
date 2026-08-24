package oauthapi

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// TestUserinfoAcceptsIdentityToken pins the contract that /oauth/userinfo's
// expected input is the identity-only access token an ordinary (non-APIAccess)
// OIDC client receives from the authorization_code grant — token_use=identity,
// stripped of all authority.
//
// This is the token the platform's bearer middleware deliberately rejects, so
// the endpoint only works mounted outside it. See
// TestOAuthProviderRoutesMountedOutsideAuthGroup in internal/server, which
// guards the wiring half of the same invariant.
func TestUserinfoAcceptsIdentityToken(t *testing.T) {
	s := testState(t)
	p := principal.NewUser("u@example.com", principal.ScopeAnchor)
	tok, err := s.Auth.GenerateIdentityAccessToken(p)
	if err != nil {
		t.Fatalf("identity mint: %v", err)
	}

	r := chi.NewRouter()
	s.RegisterUserinfoRoutes(r)

	req := httptest.NewRequest("GET", "/oauth/userinfo", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp userInfoResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if resp.Sub != p.ID {
		t.Errorf("sub = %q, want %q", resp.Sub, p.ID)
	}
	if resp.Email == nil || *resp.Email != "u@example.com" {
		t.Errorf("email = %v, want u@example.com", resp.Email)
	}
	// The identity token carries no authority; userinfo must still render the
	// array claims as empty arrays rather than null.
	if resp.Roles == nil || resp.Clients == nil || resp.Applications == nil {
		t.Errorf("authority arrays must be non-nil: %+v", resp)
	}
}

// TestUserinfoRejectsMissingBearer covers the endpoint's own auth: it
// authenticates its caller, which is why it needs no middleware in front.
func TestUserinfoRejectsMissingBearer(t *testing.T) {
	s := testState(t)
	r := chi.NewRouter()
	s.RegisterUserinfoRoutes(r)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/oauth/userinfo", nil))

	if rec.Code != 401 {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}
