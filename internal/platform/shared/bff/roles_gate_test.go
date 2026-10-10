package bff

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The BFF role endpoints check the role permission for the operation
// (docs/platform-bug-fixes.md, found with bug 1): the reads checked nothing,
// and every write only anchor scope, so any anchor principal (a read-only
// role, a provisioned service account) could create roles and grant them any
// permission.
func TestBffRolesCheckTheRolePermissions(t *testing.T) {
	r := chi.NewRouter()
	RegisterRoles(r, &RolesState{})
	anchorWith := func(perms ...string) *auth.AuthContext {
		return &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Permissions: perms}
	}
	cases := []struct {
		method, path string
		ac           *auth.AuthContext
	}{
		{http.MethodGet, "/bff/roles", anchorWith("platform:messaging:event:view")},
		{http.MethodGet, "/bff/roles/filters/applications", anchorWith()},
		{http.MethodGet, "/bff/roles/permissions", anchorWith()},
		{http.MethodGet, "/bff/roles/permissions/x:y:z:w", anchorWith()},
		{http.MethodGet, "/bff/roles/app:r", anchorWith()},
		{http.MethodPost, "/bff/roles", anchorWith("platform:iam:role:view", "platform:iam:role:update")},
		{http.MethodPut, "/bff/roles/app:r", anchorWith("platform:iam:role:view", "platform:iam:role:create")},
		{http.MethodDelete, "/bff/roles/app:r", anchorWith("platform:iam:role:update")},
		{http.MethodPost, "/bff/roles/permissions", anchorWith("platform:iam:role:update")},
		{http.MethodPost, "/bff/roles/sync-platform", anchorWith("platform:iam:role:create", "platform:iam:role:update")},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader("{}"))
		req = req.WithContext(auth.WithContext(req.Context(), tc.ac))
		rec := httptest.NewRecorder()
		admitted := false
		func() {
			defer func() {
				if recover() != nil {
					admitted = true
				}
			}()
			r.ServeHTTP(rec, req)
		}()
		if admitted || rec.Code != http.StatusForbidden {
			t.Errorf("%s %s with %v: admitted=%v status=%d, want 403", tc.method, tc.path, tc.ac.Permissions, admitted, rec.Code)
		}
	}
}
