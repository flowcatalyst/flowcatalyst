//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// PUT /api/config/...: a caller without write access to the application gets
// the same 403 for a client that exists and one that does not (the
// application is authorised before the client is resolved); an authorised
// caller is told an unknown client is not found.
func TestSetProperty_NoClientExistenceOracle(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-oracle")
	_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, 'Real', 'pcapi-real')`, tsid.Generate(tsid.Client))
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
	}}
	put := func(ac *auth.AuthContext, clientRef string) error {
		_, err := s.setProperty(testpg.WithAuth(ctx, ac), &setPropertyInput{
			App: "pcapi-oracle", Section: "s", Property: "p", ClientID: clientRef, Body: SetPropertyRequest{Value: "v"},
		})
		return err
	}
	outsider := &auth.AuthContext{PrincipalID: "prn_pcapioutside", Scope: auth.ScopeClient, Roles: []string{"none"}}
	for _, ref := range []string{"pcapi-real", "pcapi-no-such"} {
		testpg.RequireUsecaseError(t, put(outsider, ref), usecase.KindAuthorization, "FORBIDDEN")
	}
	admin := &auth.AuthContext{PrincipalID: "prn_pcapiadmin01", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:update"}}
	testpg.RequireUsecaseError(t, put(admin, "pcapi-no-such"), usecase.KindNotFound, "Client_NOT_FOUND")
	require.NoError(t, put(admin, "pcapi-real"))
}
