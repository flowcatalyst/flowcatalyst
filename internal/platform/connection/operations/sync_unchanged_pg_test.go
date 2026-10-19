//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// F30: a connection sync that changes nothing reports nothing updated and
// writes nothing; a changed field is still an update.
func TestSyncConnections_UnchangedConnectionsAreNotUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	connRepo := connection.NewRepository(pool)
	apps := application.NewRepository(pool)
	subRepo := subscription.NewRepository(pool)
	uow := testpg.NewUoW(t)
	app := mustCreateApplicationWithServiceAccount(t, pool, uow, "f30connapp", "F30 Conn App", "sva_f30conn1")

	mk := func(e operations.SyncConnectionEntry) operations.SyncConnectionsCommand {
		return operations.SyncConnectionsCommand{
			ApplicationID: app.ApplicationID, ApplicationCode: app.Code,
			Connections: []operations.SyncConnectionEntry{e},
		}
	}
	desc, ext := "main", "ext-1"
	base := operations.SyncConnectionEntry{Code: "f30-main", Name: "Main", Description: &desc, ExternalID: &ext}

	first, err := runConnSync(t, uow, connRepo, apps, subRepo, mk(base))
	require.NoError(t, err)
	assert.Equal(t, uint32(1), first.Created)
	before, err := connRepo.FindByCode(ctx, "f30-main", &app.Code, nil)
	require.NoError(t, err)
	require.NotNil(t, before)

	again, err := runConnSync(t, uow, connRepo, apps, subRepo, mk(base))
	require.NoError(t, err)
	assert.Equal(t, uint32(0), again.Created)
	assert.Equal(t, uint32(0), again.Updated, "nothing changed")
	after, err := connRepo.FindByCode(ctx, "f30-main", &app.Code, nil)
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "an unchanged connection is not rewritten")

	for name, mutate := range map[string]func(e *operations.SyncConnectionEntry){
		"name":        func(e *operations.SyncConnectionEntry) { e.Name = "Main v2" },
		"description": func(e *operations.SyncConnectionEntry) { e.Description = nil },
		"external id": func(e *operations.SyncConnectionEntry) { e.ExternalID = new("ext-2") },
	} {
		e := base
		mutate(&e)
		changed, err := runConnSync(t, uow, connRepo, apps, subRepo, mk(e))
		require.NoError(t, err)
		assert.Equal(t, uint32(1), changed.Updated, "a changed %s updates its connection", name)
		_, err = runConnSync(t, uow, connRepo, apps, subRepo, mk(base))
		require.NoError(t, err)
	}
}
