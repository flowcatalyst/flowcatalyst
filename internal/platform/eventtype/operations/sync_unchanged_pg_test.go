//go:build integration

package operations_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// A sync that changes nothing reports nothing updated and writes nothing
// (bug 6 in docs/ui/README.md; docs/platform-bug-fixes.md): Sync Platform
// Events said "73 updated" on every run.
func TestSyncEventTypes_UnchangedTypesAreNotUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := eventtype.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	desc := "Placed"
	inputs := []operations.SyncEventTypeInput{
		{Code: "etsame:orders:order:placed", Name: "Order placed", Description: &desc, Schema: json.RawMessage(`{"type":"object"}`)},
		{Code: "etsame:orders:order:shipped", Name: "Order shipped"},
	}
	cmd := operations.SyncEventTypesCommand{ApplicationCode: "etsame", EventTypes: inputs}

	first, err := runAuthorized(uow, operations.SyncEventTypes(repo), cmd)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), first.Created)
	before, err := repo.FindByCode(ctx, "etsame:orders:order:placed")
	require.NoError(t, err)

	again, err := runAuthorized(uow, operations.SyncEventTypes(repo), cmd)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), again.Created)
	assert.Equal(t, uint32(0), again.Updated, "nothing changed")
	assert.Equal(t, uint32(2), again.SchemasUnchanged)
	after, err := repo.FindByCode(ctx, "etsame:orders:order:placed")
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "an unchanged type is not rewritten")

	renamed := cmd
	renamed.EventTypes = []operations.SyncEventTypeInput{inputs[0], {Code: inputs[1].Code, Name: "Order dispatched"}}
	third, err := runAuthorized(uow, operations.SyncEventTypes(repo), renamed)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), third.Updated, "only the renamed type")

	reschema := cmd
	reschema.EventTypes = []operations.SyncEventTypeInput{{Code: inputs[0].Code, Name: inputs[0].Name, Description: &desc, Schema: json.RawMessage(`{"type":"object","title":"v2"}`)}, renamed.EventTypes[1]}
	fourth, err := runAuthorized(uow, operations.SyncEventTypes(repo), reschema)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), fourth.Updated, "a changed schema updates its type")
	assert.Equal(t, uint32(1), fourth.SchemasUpdated)
}
