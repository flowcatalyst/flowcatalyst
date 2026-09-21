//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// TestSyncSubscriptions_Endpoint_ClientIdentifierAndUnknownClient drives the
// actual sync-subscriptions HANDLER end-to-end: a clientId given as the
// client's IDENTIFIER (not its id, per resolveClientRef) resolves and scopes
// the synced subscription to that client, and an unknown client reference
// 404s rather than silently syncing against no client (or the wrong one).
// Mirrors TestSyncConnections_Endpoint_ClientIdentifierAndUnknownClient.
func TestSyncSubscriptions_Endpoint_ClientIdentifierAndUnknownClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	appCode := "syncsubhttp1"
	_, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Sync Sub HTTP"}, testpg.TestEC())
	require.NoError(t, err)

	clientEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(client.NewRepository(pool)),
		clientops.CreateCommand{Name: "Sync Sub HTTP Client", Identifier: "syncsubhttp-client"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{
		Apps:          application.NewRepository(pool),
		Connections:   connection.NewRepository(pool),
		Subscriptions: subscription.NewRepository(pool),
		Clients:       client.NewRepository(pool),
		UoW:           uow,
	}

	// An anchor caller with all-applications access clears CanSyncSubscriptions
	// (admin sync/manage) + the use case's CanAccessApplication.
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID:     "prn_syncsubhttp",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})

	bindings := []syncSubscriptionEventTypeRequest{{EventTypeCode: "syncsubhttp:a:b:c"}}

	// The client named by its IDENTIFIER, not its id.
	clientIdentifier := "syncsubhttp-client"
	out, err := s.syncSubscriptions(authCtx, &syncSubscriptionsInput{
		AppCode: appCode,
		Body: syncSubscriptionsRequest{
			ClientID: &clientIdentifier,
			Subscriptions: []syncSubscriptionInputRequest{
				{Code: "syncsubhttp-a", Name: "A", Target: "https://a.example.test/hook", EventTypes: bindings},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), out.Body.Created)

	got, err := subscription.NewRepository(pool).FindByCode(ctx, "syncsubhttp-a", &appCode, &clientEv.ClientID)
	require.NoError(t, err)
	require.NotNil(t, got, "the subscription must be scoped to the client resolved from its identifier")

	// An unknown client reference 404s rather than falling through.
	unknown := "no-such-client-identifier"
	_, err = s.syncSubscriptions(authCtx, &syncSubscriptionsInput{
		AppCode: appCode,
		Body: syncSubscriptionsRequest{
			ClientID: &unknown,
			Subscriptions: []syncSubscriptionInputRequest{
				{Code: "syncsubhttp-b", Name: "B", Target: "https://b.example.test/hook", EventTypes: bindings},
			},
		},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
}
