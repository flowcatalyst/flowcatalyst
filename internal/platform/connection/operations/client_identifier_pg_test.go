//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
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
