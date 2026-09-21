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

// TestSyncConnections_Endpoint_ClientIdentifierAndUnknownClient drives the
// actual sync-connections HANDLER end-to-end: a clientId given as the
// client's IDENTIFIER (not its id, per resolveClientRef — ruling 2026-09-21
// #5) resolves and scopes the synced connection to that client, and an
// unknown client reference 404s rather than silently syncing against no
// client (or the wrong one).
func TestSyncConnections_Endpoint_ClientIdentifierAndUnknownClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)

	appCode := "syncconnhttp1"
	appEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Sync Conn HTTP"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, name, active) VALUES ($1, 'SERVICE', 'HTTP SA', TRUE)`,
		"sva_syncconnhttp")
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `UPDATE app_applications SET service_account_id = $1 WHERE id = $2`,
		"sva_syncconnhttp", appEv.ApplicationID)
	require.NoError(t, err)

	clientEv, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(client.NewRepository(pool)),
		clientops.CreateCommand{Name: "Sync Conn HTTP Client", Identifier: "syncconnhttp-client"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{
		Apps:          application.NewRepository(pool),
		Connections:   connection.NewRepository(pool),
		Subscriptions: subscription.NewRepository(pool),
		Clients:       client.NewRepository(pool),
		UoW:           uow,
	}

	// An anchor caller with all-applications access clears CanSyncConnections
	// (admin sync/manage) + the use case's CanAccessApplication.
	authCtx := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID:     "prn_syncconnhttp",
		Scope:           auth.ScopeAnchor,
		Permissions:     []string{"platform:*:*:*"},
		AllApplications: true,
	})

	// The client named by its IDENTIFIER, not its id.
	clientIdentifier := "syncconnhttp-client"
	out, err := s.syncConnections(authCtx, &syncConnectionsInput{
		AppCode: appCode,
		Body: syncConnectionsRequest{
			ClientID: &clientIdentifier,
			Connections: []syncConnectionInputRequest{
				{Code: "syncconnhttp-a", Name: "A"},
			},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, uint32(1), out.Body.Created)

	got, err := connection.NewRepository(pool).FindByCode(ctx, "syncconnhttp-a", &appCode, &clientEv.ClientID)
	require.NoError(t, err)
	require.NotNil(t, got, "the connection must be scoped to the client resolved from its identifier")

	// An unknown client reference 404s rather than falling through.
	unknown := "no-such-client-identifier"
	_, err = s.syncConnections(authCtx, &syncConnectionsInput{
		AppCode: appCode,
		Body: syncConnectionsRequest{
			ClientID:    &unknown,
			Connections: []syncConnectionInputRequest{{Code: "syncconnhttp-b", Name: "B"}},
		},
	})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Client_NOT_FOUND")
}
