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
	"github.com/flowcatalyst/go-sdk/usecase"
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

// GET /api/platform-config/{app}: a client row names the client both ways
// (its stored id and its identifier), so a caller holding either can match it.
func TestListProperties_NamesTheClientIdentifier(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	testpg.EnsureApplication(t, "pcapi-list")
	cid := tsid.Generate(tsid.Client)
	_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, 'Listed', 'pcapi-listed')`, cid)
	require.NoError(t, err)
	s := &State{Svc: &platformconfig.Service{
		Repo: platformconfig.NewRepository(pool), UoW: testpg.NewUoW(t), Clients: client.NewRepository(pool), Audit: audit.NewRepository(pool),
	}}
	admin := &auth.AuthContext{PrincipalID: "prn_pcapiadmin02", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{"platform:admin:config:update", "platform:admin:config:view"}}
	actx := testpg.WithAuth(ctx, admin)
	for _, ref := range []string{"", "pcapi-listed"} {
		_, err := s.setProperty(actx, &setPropertyInput{App: "pcapi-list", Section: "s", Property: "p", ClientID: ref, Body: SetPropertyRequest{Value: "v"}})
		require.NoError(t, err)
	}
	out, err := s.listProperties(actx, &listPropsInput{App: "pcapi-list"})
	require.NoError(t, err)
	require.Len(t, out.Body.Items, 2)
	for _, r := range out.Body.Items {
		if r.ClientID == nil {
			require.Nil(t, r.ClientIdentifier, "the global row names no client")
			continue
		}
		require.Equal(t, cid, *r.ClientID)
		require.NotNil(t, r.ClientIdentifier)
		require.Equal(t, "pcapi-listed", *r.ClientIdentifier)
	}
}
