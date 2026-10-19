//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// F30: a subscription sync that changes nothing reports nothing updated and
// writes nothing; a changed field is still an update.
func TestSyncSubscriptions_UnchangedSubscriptionsAreNotUpdated(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	subRepo := subscription.NewRepository(pool)
	connRepo := connection.NewRepository(pool)
	poolRepo := dispatchpool.NewRepository(pool)
	uow := testpg.NewUoW(t)
	appCode := "f30subapp"

	run := func(in operations.SyncSubscriptionInput) operations.SubscriptionsSynced {
		t.Helper()
		out, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncSubscriptions(subRepo, connRepo, poolRepo),
			operations.SyncSubscriptionsCommand{ApplicationCode: appCode, Subscriptions: []operations.SyncSubscriptionInput{in}},
			testpg.TestEC())
		require.NoError(t, err)
		return out
	}
	// A binding filter is not persisted (no column), so it must not make a
	// subscription look changed on every sync.
	filter := "$.data.kind == 'x'"
	base := operations.SyncSubscriptionInput{
		Code: "f30-notify", Name: "Notify", Description: new("d"), Target: "https://f30.example.test/hook",
		EventTypes: []operations.SyncEventTypeBindingInput{
			{EventTypeCode: "f30subapp:orders:order:created"},
			{EventTypeCode: "f30subapp:orders:order:updated", Filter: &filter},
		},
		MaxRetries: new(int32(5)), TimeoutSeconds: new(int32(30)),
	}

	first := run(base)
	assert.Equal(t, uint32(1), first.Created)
	before, err := subRepo.FindByCode(ctx, "f30-notify", &appCode, nil)
	require.NoError(t, err)
	require.NotNil(t, before)

	again := run(base)
	assert.Equal(t, uint32(0), again.Created)
	assert.Equal(t, uint32(0), again.Updated, "nothing changed")
	after, err := subRepo.FindByCode(ctx, "f30-notify", &appCode, nil)
	require.NoError(t, err)
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "an unchanged subscription is not rewritten")

	// Omitted maxRetries / timeoutSeconds keep the stored values: unchanged.
	omitted := base
	omitted.MaxRetries, omitted.TimeoutSeconds = nil, nil
	assert.Equal(t, uint32(0), run(omitted).Updated)

	for name, mutate := range map[string]func(in *operations.SyncSubscriptionInput){
		"name":        func(in *operations.SyncSubscriptionInput) { in.Name = "Notify v2" },
		"description": func(in *operations.SyncSubscriptionInput) { in.Description = nil },
		"target":      func(in *operations.SyncSubscriptionInput) { in.Target = "https://f30.example.test/other" },
		"binding set": func(in *operations.SyncSubscriptionInput) { in.EventTypes = in.EventTypes[:1] },
		"data only":   func(in *operations.SyncSubscriptionInput) { in.DataOnly = true },
		"max retries": func(in *operations.SyncSubscriptionInput) { in.MaxRetries = new(int32(9)) },
		"timeout":     func(in *operations.SyncSubscriptionInput) { in.TimeoutSeconds = new(int32(45)) },
	} {
		in := base
		in.EventTypes = append([]operations.SyncEventTypeBindingInput(nil), base.EventTypes...)
		mutate(&in)
		assert.Equal(t, uint32(1), run(in).Updated, "a changed %s updates its subscription", name)
		run(base)
	}
}
