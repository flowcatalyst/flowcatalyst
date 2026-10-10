//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// POST /api/clients/{id}/deactivate sets the client INACTIVE with the reason
// and keeps the row, so it can be reactivated (bug 15 in docs/ui/README.md;
// docs/platform-bug-fixes.md); it ran DeleteClient, which removed the row.
func TestDeactivateClient_SetsInactiveAndKeepsTheRow(t *testing.T) {
	ctx := testpg.AnchorCtx()
	repo := client.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ev, err := usecaseop.Run(ctx, uow, clientops.CreateClient(repo), clientops.CreateCommand{Name: "Deactivate me", Identifier: "deact-me"}, testpg.TestEC())
	require.NoError(t, err)
	s := &State{Repo: repo, UoW: uow}

	out, err := s.deactivate(ctx, &deactivateInput{ID: ids.ClientID(ev.ClientID), Body: StatusChangeRequest{Reason: "Manual deactivation"}})
	require.NoError(t, err)
	assert.Equal(t, "Client deactivated", out.Body.Message)

	got, err := repo.FindByID(context.Background(), ev.ClientID)
	require.NoError(t, err)
	require.NotNil(t, got, "the client row is kept")
	assert.Equal(t, client.StatusInactive, got.Status)
	require.NotNil(t, got.StatusReason)
	assert.Equal(t, "Manual deactivation", *got.StatusReason)

	_, err = s.activate(ctx, &apicommon.IDInput{ID: ev.ClientID})
	require.NoError(t, err)
	got, err = repo.FindByID(context.Background(), ev.ClientID)
	require.NoError(t, err)
	assert.Equal(t, client.StatusActive, got.Status, "a deactivated client can be reactivated")
}
