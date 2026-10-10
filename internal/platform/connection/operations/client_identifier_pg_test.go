//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// seedClientIdentifier creates a client and returns its id.
func seedClientIdentifier(t *testing.T, identifier string) string {
	t.Helper()
	ev, err := runAuthorized(testpg.NewUoW(t), clientops.CreateClient(client.NewRepository(testpg.Pool(t))),
		clientops.CreateCommand{Name: identifier, Identifier: identifier})
	require.NoError(t, err)
	return ev.ClientID
}

// A client-scoped connection stores its client's identifier
// (docs/platform-bug-fixes.md, dispatch pools/connections item): the SPA
// showed every connection as "Anchor-level".
func TestCreateConnection_StoresTheClientIdentifier(t *testing.T) {
	repo := connection.NewRepository(testpg.Pool(t))
	clientID := seedClientIdentifier(t, "con-ident-acme")
	ev, err := runAuthorized(testpg.NewUoW(t), operations.CreateConnection(repo, application.NewRepository(testpg.Pool(t))), operations.CreateCommand{
		Code: "identconn", Name: "Ident", ServiceAccountID: "sac_identconn0001", ClientID: &clientID,
	})
	require.NoError(t, err)
	got, err := repo.FindByID(context.Background(), ev.ConnectionID)
	require.NoError(t, err)
	require.NotNil(t, got.ClientIdentifier)
	require.Equal(t, "con-ident-acme", *got.ClientIdentifier)
}

// The code-first sync stores it too: a client-scoped sync creates its
// connections with the client's identifier, as create does.
func TestSyncConnections_StoresTheClientIdentifier(t *testing.T) {
	pool := testpg.Pool(t)
	repo := connection.NewRepository(pool)
	uow := testpg.NewUoW(t)
	clientID := seedClientIdentifier(t, "con-sync-ident")
	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "consyncident", "Con Sync Ident", "sva_consyncid01")
	_, err := runConnSync(t, uow, repo, application.NewRepository(pool), subscription.NewRepository(pool),
		operations.SyncConnectionsCommand{
			ApplicationID: app.ApplicationID, ApplicationCode: app.Code, ClientID: &clientID,
			Connections: []operations.SyncConnectionEntry{{Code: "syncident", Name: "Ident"}},
		})
	require.NoError(t, err)
	got, err := repo.FindByCode(context.Background(), "syncident", &app.Code, &clientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ClientIdentifier)
	require.Equal(t, "con-sync-ident", *got.ClientIdentifier)
}
