//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
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

// The code-first sync stores it too: a client-scoped sync creates its
// subscriptions with the client's identifier, as create does.
func TestSyncSubscriptions_StoresTheClientIdentifier(t *testing.T) {
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	clientID := seedClientIdentifier(t, "sub-sync-ident")
	_, err := usecaseop.Run(appAccessCtx(), testpg.NewUoW(t),
		operations.SyncSubscriptions(repo, connection.NewRepository(pool), dispatchpool.NewRepository(pool)),
		operations.SyncSubscriptionsCommand{
			ApplicationCode: "subsyncident", ClientID: &clientID,
			Subscriptions: []operations.SyncSubscriptionInput{{
				Code: "syncident", Name: "Ident", Target: "https://hooks.example.com/syncident",
				EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "subsyncident:orders:order:created"}},
			}},
		}, testpg.TestEC())
	require.NoError(t, err)
	appCode := "subsyncident"
	got, err := repo.FindByCode(context.Background(), "syncident", &appCode, &clientID)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ClientIdentifier)
	require.Equal(t, "sub-sync-ident", *got.ClientIdentifier)
}
