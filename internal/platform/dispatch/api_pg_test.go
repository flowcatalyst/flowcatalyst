//go:build integration

package dispatch_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
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

// F29: the router polls the document every few seconds, so an unchanged one
// answers 304 to its validator, and any change to what the document lists
// (a new client's queue) is a 200 with a new validator.
func TestRouterConfigEndpoint_ConditionalGet(t *testing.T) {
	s := routerConfigState(t)
	pool := testpg.Pool(t)
	ctx := auth.WithContext(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_rc_etag", Scope: auth.ScopeAnchor, PrincipalType: "SERVICE",
		Permissions: []string{"platform:messaging:dispatch-pool:view"},
	})

	status, etag, doc, err := s.RouterConfigConditionalForTest(ctx, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.NotEmpty(t, etag)
	require.NotNil(t, doc)

	status, again, doc, err := s.RouterConfigConditionalForTest(ctx, etag)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotModified, status, "an unchanged document is a 304")
	require.Equal(t, etag, again)
	require.Nil(t, doc)

	status, _, _, err = s.RouterConfigConditionalForTest(ctx, `"stale", W/`+etag)
	require.NoError(t, err)
	require.Equal(t, http.StatusNotModified, status, "a list and weak validators are understood")
	status, _, _, err = s.RouterConfigConditionalForTest(ctx, `"somethingelse"`)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)

	// A new client adds its two lanes to the document.
	_, err = pool.Exec(context.Background(),
		`INSERT INTO tnt_clients (id, name, identifier) VALUES ('clt_f29etag01', 'F29 etag', 'f29-etag-client')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	status, changed, doc, err := s.RouterConfigConditionalForTest(ctx, etag)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status, "the document changed")
	require.NotEqual(t, etag, changed)
	require.NotNil(t, doc)
}
