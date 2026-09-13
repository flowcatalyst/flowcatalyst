//go:build integration

package dispatch_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// routerConfigState builds the endpoint over a real document builder.
func routerConfigState(t *testing.T) *dispatch.State {
	t.Helper()
	settings, err := dispatch.ResolveSettings("postgres", "", "", "FC-test", "postgresql://x@localhost/fc")
	require.NoError(t, err)
	return &dispatch.State{Documents: dispatch.NewDocumentBuilder(testpg.Pool(t), settings)}
}

// The document lists every client's queue names and URLs beside the pool
// codes, so it is anchor-only AND permission-gated. The built-in
// platform:router role carries exactly the one permission asserted here.
func TestRouterConfigEndpoint_RequiresAnchorAndTheDispatchPoolViewPermission(t *testing.T) {
	s := routerConfigState(t)

	t.Run("no principal", func(t *testing.T) {
		_, err := s.RouterConfigForTest(context.Background())
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")
	})

	t.Run("anchor without the permission", func(t *testing.T) {
		// A provisioned application service account: anchor-scoped, and its
		// role grants it nothing here.
		ctx := auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID:   "prn_rc_sa",
			Scope:         auth.ScopeAnchor,
			PrincipalType: "SERVICE",
			Permissions:   []string{"platform:application-service:event-type:create"},
		})
		_, err := s.RouterConfigForTest(ctx)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
	})

	t.Run("client-scoped principal holding the permission", func(t *testing.T) {
		// Reach, not just authority: the document spans every tenant.
		ctx := auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID: "prn_rc_client",
			Scope:       auth.ScopeClient,
			Clients:     []string{"clt_rc"},
			Permissions: []string{"platform:messaging:dispatch-pool:view"},
		})
		_, err := s.RouterConfigForTest(ctx)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "ANCHOR_REQUIRED")
	})

	t.Run("the router's own role", func(t *testing.T) {
		ctx := auth.WithContext(context.Background(), &auth.AuthContext{
			PrincipalID:   "prn_rc_router",
			Scope:         auth.ScopeAnchor,
			PrincipalType: "SERVICE",
			// Exactly what platform:router grants.
			Permissions: []string{"platform:messaging:dispatch-pool:view"},
		})
		doc, err := s.RouterConfigForTest(ctx)
		require.NoError(t, err)
		// The platform tenant always has a lane, so the document is never
		// empty — proving a real build, not an early return.
		require.NotEmpty(t, doc.Queues)
	})
}
