package bff

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// Sync Platform Events needs the event-type sync permission on top of anchor
// scope (docs/platform-bug-fixes.md, "who may sync"): anchor scope alone is
// reach, not authority, and any anchor principal (a read-only staff role, a
// provisioned service account) could rewrite the platform catalogue.
func TestSyncPlatformEventTypesNeedsTheSyncPermission(t *testing.T) {
	r := chi.NewRouter()
	RegisterEventTypes(r, &EventTypesState{})
	for _, tc := range []struct {
		name string
		ac   *auth.AuthContext
	}{
		{"anchor without a permission", &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeAnchor, Permissions: []string{"platform:messaging:event-type:view"}}},
		{"client user with the permission", &auth.AuthContext{PrincipalID: "p", Scope: auth.ScopeClient, Clients: []string{"clt_1"}, Permissions: []string{"platform:messaging:event-type:sync"}}},
	} {
		req := httptest.NewRequest(http.MethodPost, "/bff/event-types/sync-platform", nil)
		req = req.WithContext(auth.WithContext(req.Context(), tc.ac))
		rec := httptest.NewRecorder()
		func() {
			defer func() {
				if recover() != nil {
					t.Errorf("%s: admitted (reached the use case)", tc.name)
				}
			}()
			r.ServeHTTP(rec, req)
		}()
		if rec.Code != http.StatusForbidden && !t.Failed() {
			t.Errorf("%s: %d, want 403", tc.name, rec.Code)
		}
	}
}
