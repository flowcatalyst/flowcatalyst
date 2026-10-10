//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
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

// A client-scoped subscription stores its client's identifier (the same flaw
// as pools and connections; docs/platform-bug-fixes.md): the SPA showed
// "Anchor-level" for it.
func TestCreateSubscription_StoresTheClientIdentifier(t *testing.T) {
	repo := subscription.NewRepository(testpg.Pool(t))
	clientID := seedClientIdentifier(t, "sub-ident-acme")
	ev, err := runAuthorized(testpg.NewUoW(t), operations.CreateSubscription(repo), operations.CreateCommand{
		Code: "identsub", Name: "Ident", Endpoint: "https://hooks.example.com/ident", ClientID: &clientID,
		EventTypes: []subscription.EventTypeBinding{subscription.NewEventTypeBinding("ident:orders:order:created")},
	})
	require.NoError(t, err)
	got, err := repo.FindByID(context.Background(), ev.SubscriptionID)
	require.NoError(t, err)
	require.NotNil(t, got.ClientIdentifier)
	require.Equal(t, "sub-ident-acme", *got.ClientIdentifier)
}
