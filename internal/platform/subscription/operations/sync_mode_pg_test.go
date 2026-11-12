//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// Code is the source of truth for a subscription's dispatch mode: a sync that
// carries one applies it on create and on update, one that omits it leaves the
// stored mode alone, and an unknown value is refused.
func TestSyncSubscriptions_AppliesMode(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)
	appCode := "modesubapp"

	run := func(in operations.SyncSubscriptionInput) (operations.SubscriptionsSynced, error) {
		t.Helper()
		return usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{ApplicationCode: appCode, Subscriptions: []operations.SyncSubscriptionInput{in}},
			testpg.TestEC())
	}
	stored := func(code string) common.DispatchMode {
		t.Helper()
		s, err := subRepo.FindByCode(ctx, code, &appCode, nil)
		require.NoError(t, err)
		require.NotNil(t, s)
		return s.Mode
	}
	def := func(code string, mode *string) operations.SyncSubscriptionInput {
		return operations.SyncSubscriptionInput{
			Code: code, Name: "Mode", Target: "https://mode.example.test/hook", Mode: mode,
			EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "modesubapp:orders:order:created"}},
		}
	}

	// Create with a mode.
	_, err := run(def("mode-create", new("BLOCK_ON_ERROR")))
	require.NoError(t, err)
	assert.Equal(t, common.DispatchBlockOnError, stored("mode-create"))

	// Create without one takes the default.
	_, err = run(def("mode-default", nil))
	require.NoError(t, err)
	assert.Equal(t, common.DefaultDispatchMode, stored("mode-default"))

	// Update changes it, and counts as an update.
	out, err := run(def("mode-create", new("IMMEDIATE")))
	require.NoError(t, err)
	assert.Equal(t, uint32(1), out.Updated)
	assert.Equal(t, common.DispatchImmediate, stored("mode-create"))

	// Update without a mode keeps the stored one.
	out, err = run(def("mode-create", nil))
	require.NoError(t, err)
	assert.Equal(t, uint32(0), out.Updated, "nothing carried, nothing changed")
	assert.Equal(t, common.DispatchImmediate, stored("mode-create"))

	// An invalid mode is refused, naming the allowed values; nothing changes.
	_, err = run(def("mode-create", new("SOMETIMES")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "IMMEDIATE, NEXT_ON_ERROR, BLOCK_ON_ERROR")
	assert.Equal(t, common.DispatchImmediate, stored("mode-create"))
}
