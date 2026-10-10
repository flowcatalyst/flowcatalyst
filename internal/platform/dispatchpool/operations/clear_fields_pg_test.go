//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// An update can clear a pool's rate limit (0) and description (blank)
// (docs/platform-bug-fixes.md, dispatch pools item): nil left them as they
// were and nothing cleared them, so an edit could never remove either.
func TestUpdateDispatchPool_ClearsRateLimitAndDescription(t *testing.T) {
	repo := dispatchpool.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	desc, rate := "Busy", int32(60)
	ev, err := runAuthorized(uow, operations.CreateDispatchPool(repo), operations.CreateCommand{Code: "clearpool", Name: "Clear", Description: &desc, RateLimit: &rate})
	require.NoError(t, err)

	blank, zero := "  ", int32(0)
	_, err = runAuthorized(uow, operations.UpdateDispatchPool(repo), operations.UpdateCommand{ID: ev.PoolID, Description: &blank, RateLimit: &zero})
	require.NoError(t, err)
	got, err := repo.FindByID(context.Background(), ev.PoolID)
	require.NoError(t, err)
	assert.Nil(t, got.Description, "a blank description clears it")
	assert.Nil(t, got.RateLimit, "a zero rate limit clears it (no limit)")

	_, err = runAuthorized(uow, operations.UpdateDispatchPool(repo), operations.UpdateCommand{ID: ev.PoolID, RateLimit: &rate})
	require.NoError(t, err)
	got, err = repo.FindByID(context.Background(), ev.PoolID)
	require.NoError(t, err)
	require.NotNil(t, got.RateLimit)
	assert.Equal(t, int32(60), *got.RateLimit)
}
