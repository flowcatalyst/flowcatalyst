package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// TestProfileOnlyWithoutRole pins the rule: a USER with no platform role
// reaches only the self-service surface; everything else — including
// ungated /bff lookups and anything a zero-role ANCHOR user would otherwise
// pass — answers 403 NO_PLATFORM_ROLE. Service accounts, unknown-type
// contexts, and anonymous requests are untouched.
func TestProfileOnlyWithoutRole(t *testing.T) {
	handler := ProfileOnlyWithoutRole(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	do := func(ac *auth.AuthContext, method, path string) int {
		req := httptest.NewRequest(method, path, nil)
		if ac != nil {
			req = req.WithContext(auth.WithContext(req.Context(), ac))
		}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	roleless := &auth.AuthContext{PrincipalID: "prn_1", Scope: auth.ScopeClient, PrincipalType: auth.PrincipalTypeUser}
	rolelessAnchor := &auth.AuthContext{PrincipalID: "prn_2", Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser}
	admin := &auth.AuthContext{
		PrincipalID: "prn_3", Scope: auth.ScopeAnchor, PrincipalType: auth.PrincipalTypeUser,
		Roles: []string{"platform:super-admin"}, Permissions: []string{"platform:*:*:*"},
	}
	appSA := &auth.AuthContext{
		PrincipalID: "prn_4", Scope: auth.ScopeAnchor, PrincipalType: "SERVICE",
		Applications: []string{"app_1"},
	}

	for _, ac := range []*auth.AuthContext{roleless, rolelessAnchor} {
		assert.Equal(t, http.StatusForbidden, do(ac, http.MethodGet, "/bff/roles"))
		assert.Equal(t, http.StatusForbidden, do(ac, http.MethodGet, "/api/clients"))
		assert.Equal(t, http.StatusForbidden, do(ac, http.MethodGet, "/api/me/clients"))
		assert.Equal(t, http.StatusForbidden, do(ac, http.MethodPost, "/api/audit-logs/batch"))
		// Self-service stays open.
		assert.Equal(t, http.StatusOK, do(ac, http.MethodGet, "/auth/me"))
		assert.Equal(t, http.StatusOK, do(ac, http.MethodPost, "/auth/change-password"))
		assert.Equal(t, http.StatusOK, do(ac, http.MethodGet, "/auth/2fa/status"))
		assert.Equal(t, http.StatusOK, do(ac, http.MethodGet, "/api/me"))
	}
	// The rejection uses the platform error envelope.
	req := httptest.NewRequest(http.MethodGet, "/bff/roles", nil)
	req = req.WithContext(auth.WithContext(req.Context(), roleless))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	assert.JSONEq(t, `{"error":"NO_PLATFORM_ROLE","message":"Your account has no platform access. Only your profile is available."}`, rr.Body.String())

	assert.Equal(t, http.StatusOK, do(admin, http.MethodGet, "/bff/roles"))
	assert.Equal(t, http.StatusOK, do(appSA, http.MethodPost, "/api/sdk/sync"), "service accounts are exempt")
	assert.Equal(t, http.StatusOK, do(&auth.AuthContext{PrincipalID: "t"}, http.MethodGet, "/bff/roles"), "unknown type exempt")
	assert.Equal(t, http.StatusOK, do(nil, http.MethodGet, "/bff/roles"), "anonymous passes to the handler's own check")
}
