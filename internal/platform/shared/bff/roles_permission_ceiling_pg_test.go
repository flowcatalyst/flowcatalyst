//go:build integration

package bff

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The permission ceiling (F10) on the SPA's role editor routes: create,
// update and delete through /bff/roles answer 403 PERMISSION_ABOVE_CALLER
// for a platform permission the caller does not hold, added or removed.
func TestBffRoles_PermissionsBoundedByCallersPermissions(t *testing.T) {
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	r := chi.NewRouter()
	RegisterRoles(r, &RolesState{Roles: repo, UoW: testpg.NewUoW(t)})

	admin := &auth.AuthContext{PrincipalID: "prn_bffceil_adm1", Scope: auth.ScopeAnchor, Permissions: []string{
		"platform:iam:role:create", "platform:iam:role:update", "platform:iam:role:delete", "platform:iam:user:view",
	}}
	superAdmin := &auth.AuthContext{PrincipalID: "prn_bffceil_sup1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}}
	send := func(ac *auth.AuthContext, method, path, body string) (int, map[string]any) {
		t.Helper()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req = req.WithContext(auth.WithContext(req.Context(), ac))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var out map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Code, out
	}
	refused := func(label string, status int, body map[string]any, named string) {
		t.Helper()
		assert.Equal(t, http.StatusForbidden, status, label)
		assert.Equal(t, "PERMISSION_ABOVE_CALLER", body["error"], label)
		assert.Equal(t, "You may only add or remove permissions you hold yourself: "+named, body["message"], label)
	}

	// Platform roles: confinement (F12) refuses a platform permission on
	// another application's role before the ceiling is reached.
	status, body := send(admin, http.MethodPost, "/bff/roles",
		`{"applicationCode":"platform","roleName":"bffceil-escalated","displayName":"Escalated","permissions":["platform:*:*:*"]}`)
	refused("create", status, body, "platform:*:*:*")

	status, _ = send(admin, http.MethodPost, "/bff/roles",
		`{"applicationCode":"platform","roleName":"bffceil-clerk","displayName":"Clerk","permissions":["platform:iam:user:view"]}`)
	require.Equal(t, http.StatusCreated, status)

	status, body = send(admin, http.MethodPut, "/bff/roles/platform:bffceil-clerk",
		`{"permissions":["platform:iam:user:view","platform:*:*:*"]}`)
	refused("update adding", status, body, "platform:*:*:*")

	status, _ = send(superAdmin, http.MethodPut, "/bff/roles/platform:bffceil-clerk",
		`{"permissions":["platform:iam:user:view","platform:iam:user:create"]}`)
	require.Equal(t, http.StatusNoContent, status, "the wildcard holder may add anything")

	status, body = send(admin, http.MethodPut, "/bff/roles/platform:bffceil-clerk",
		`{"permissions":["platform:iam:user:view"]}`)
	refused("update removing", status, body, "platform:iam:user:create")

	status, body = send(admin, http.MethodDelete, "/bff/roles/platform:bffceil-clerk", "")
	refused("delete", status, body, "platform:iam:user:create")

	clerk, err := repo.FindByName(t.Context(), "platform:bffceil-clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.ElementsMatch(t, []string{"platform:iam:user:view", "platform:iam:user:create"}, clerk.Permissions)
}
