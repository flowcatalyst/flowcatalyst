//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
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

// A client-scoped pool stores its client's identifier (docs/platform-bug-fixes.md,
// dispatch pools item): it never did, so the SPA showed every pool as
// "Anchor-level (no client)".
func TestCreateDispatchPool_StoresTheClientIdentifier(t *testing.T) {
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	clientID := seedClientIdentifier(t, "dpl-ident-acme")
	ev, err := runAuthorized(testpg.NewUoW(t), operations.CreateDispatchPool(repo), operations.CreateCommand{Code: "identpool", Name: "Ident", ClientID: &clientID})
	require.NoError(t, err)
	got, err := repo.FindByID(context.Background(), ev.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got.ClientIdentifier)
	require.Equal(t, "dpl-ident-acme", *got.ClientIdentifier)

	platform, err := runAuthorized(testpg.NewUoW(t), operations.CreateDispatchPool(repo), operations.CreateCommand{Code: "identpool-platform", Name: "Platform"})
	require.NoError(t, err)
	got, err = repo.FindByID(context.Background(), platform.PoolID)
	require.NoError(t, err)
	require.Nil(t, got.ClientIdentifier)
}
