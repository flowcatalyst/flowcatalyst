//go:build integration

package ingestion_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// An integration that owns an ingestion cannot be turned back into an
// application (the ingestion resolver only accepts integrations); once the
// ingestion is gone it can, and the upgrade to integration is always open.
func TestUpdateApplicationType_GuardedByOwnedIngestions(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	apps := application.NewRepository(e.pool)
	update := func(typ string) (appops.ApplicationUpdated, error) {
		return usecaseop.Run(testpg.AnchorCtx(), e.uow, appops.UpdateApplication(apps, e.repo),
			appops.UpdateCommand{ID: e.app.ID.String(), Type: &typ}, testpg.TestEC())
	}

	i := e.newIngestion("typed", sqsDef(t, "Typed", "typed"), true)
	e.mustSave(t, i)
	n, err := e.repo.CountByApplication(ctx, e.app.ID.String())
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	_, err = update("APPLICATION")
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "APPLICATION_TYPE_IN_USE")
	got, err := apps.FindByID(ctx, e.app.ID.String())
	require.NoError(t, err)
	assert.Equal(t, application.TypeIntegration, got.Type)

	ev, err := update("INTEGRATION")
	require.NoError(t, err)
	assert.Empty(t, ev.PreviousType, "already an integration")

	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Delete(ctx, i, tx) }))
	ev, err = update("APPLICATION")
	require.NoError(t, err)
	assert.Equal(t, "INTEGRATION", ev.PreviousType)

	ev, err = update("INTEGRATION")
	require.NoError(t, err)
	assert.Equal(t, "APPLICATION", ev.PreviousType)
	assert.Equal(t, "INTEGRATION", ev.Type)
}
