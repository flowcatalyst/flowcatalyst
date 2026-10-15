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

// Confinement (F12) on the SPA's role editor routes: create and update through
// /bff/roles answer 400 PERMISSION_OUTSIDE_APPLICATION for a permission of
// another application, whoever holds it; a super-admin may cross.
func TestBffRoles_PermissionsConfinedToTheirApplication(t *testing.T) {
	testpg.EnsureApplication(t, "platform")
	testpg.EnsureApplication(t, "bffconf")
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	r := chi.NewRouter()
	RegisterRoles(r, &RolesState{Roles: repo, UoW: testpg.NewUoW(t)})

	admin := &auth.AuthContext{PrincipalID: "prn_bffconf_adm1", Scope: auth.ScopeAnchor, Permissions: []string{
		"platform:iam:role:create", "platform:iam:role:update", "platform:iam:role:delete", "platform:iam:user:view",
	}}
	superAdmin := &auth.AuthContext{PrincipalID: "prn_bffconf_sup1", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}}
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
	refused := func(label string, status int, body map[string]any, permission, app string) {
		t.Helper()
		assert.Equal(t, http.StatusBadRequest, status, label)
		assert.Equal(t, "PERMISSION_OUTSIDE_APPLICATION", body["error"], label)
		assert.Equal(t, "Permission '"+permission+"' does not belong to application '"+app+
			"'; a role may only hold its own application's permissions", body["message"], label)
	}

	status, body := send(admin, http.MethodPost, "/bff/roles",
		`{"applicationCode":"bffconf","roleName":"escalated","displayName":"Escalated","permissions":["platform:iam:user:view"]}`)
	refused("create with a held platform permission", status, body, "platform:iam:user:view", "bffconf")

	status, _ = send(admin, http.MethodPost, "/bff/roles",
		`{"applicationCode":"bffconf","roleName":"clerk","displayName":"Clerk","permissions":["bffconf:order:view"]}`)
	require.Equal(t, http.StatusCreated, status)
	status, _ = send(admin, http.MethodPost, "/bff/roles",
		`{"applicationCode":"platform","roleName":"bffconf-viewer","displayName":"Viewer","permissions":["platform:iam:user:view"]}`)
	require.Equal(t, http.StatusCreated, status, "a platform role with platform permissions")

	status, body = send(admin, http.MethodPut, "/bff/roles/bffconf:clerk",
		`{"permissions":["bffconf:order:view","other:order:view"]}`)
	refused("update adding another application's", status, body, "other:order:view", "bffconf")

	status, _ = send(superAdmin, http.MethodPut, "/bff/roles/bffconf:clerk",
		`{"permissions":["bffconf:order:view","other:order:view"]}`)
	require.Equal(t, http.StatusNoContent, status, "a super-admin may cross applications")

	// What the editor sends back for a role already holding one: accepted.
	status, _ = send(admin, http.MethodPut, "/bff/roles/bffconf:clerk",
		`{"permissions":["bffconf:order:view","bffconf:order:create","other:order:view"]}`)
	require.Equal(t, http.StatusNoContent, status, "a kept foreign permission is not checked")

	clerk, err := repo.FindByName(t.Context(), "bffconf:clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.ElementsMatch(t, []string{"bffconf:order:view", "bffconf:order:create", "other:order:view"}, clerk.Permissions)
	none, err := repo.FindByName(t.Context(), "bffconf:escalated")
	require.NoError(t, err)
	assert.Nil(t, none)
}
