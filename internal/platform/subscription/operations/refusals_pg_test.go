//go:build integration

package operations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

func seedPool(t *testing.T, code, status string) string {
	t.Helper()
	id := tsid.Generate(tsid.DispatchPool)
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO msg_dispatch_pools (id, code, name, status) VALUES ($1, $2, $2, $3)`, id, code, status)
	require.NoError(t, err)
	return id
}

func requireCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var ue *usecase.Error
	require.True(t, errors.As(err, &ue), "a usecase.Error, got %T: %v", err, err)
	assert.Equal(t, code, ue.Code)
}

// Owner rulings 2026-11-09: a non-blank binding filter is refused (no format
// was ever defined and none was ever applied); an ARCHIVED pool takes no new
// subscription; a subscription created without dataOnly gets the envelope.
func TestSubscriptionRefusals(t *testing.T) {
	t.Parallel()
	repo := subscription.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	binding := func(filter *string) []subscription.EventTypeBinding {
		b := subscription.NewEventTypeBinding("refusal:orders:order:created")
		b.Filter = filter
		return []subscription.EventTypeBinding{b}
	}
	f := "$.data.kind == 'x'"
	blank := " "

	t.Run("create refuses a filter", func(t *testing.T) {
		_, err := runAuthorized(uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: "refusal-filter", Name: "F", Endpoint: "https://refusal.example.test/hook", EventTypes: binding(&f),
		})
		requireCode(t, err, "FILTER_NOT_SUPPORTED")
	})

	t.Run("a blank filter is accepted, never stored, and dataOnly defaults to the envelope", func(t *testing.T) {
		ev, err := runAuthorized(uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: "refusal-blank", Name: "B", Endpoint: "https://refusal.example.test/hook", EventTypes: binding(&blank),
		})
		require.NoError(t, err)
		s, err := repo.FindByID(context.Background(), ev.SubscriptionID)
		require.NoError(t, err)
		require.NotNil(t, s)
		assert.False(t, s.DataOnly, "the documented default: the envelope")
		require.Len(t, s.EventTypes, 1)
		assert.Nil(t, s.EventTypes[0].Filter)

		// Update refuses a filter too.
		_, err = runAuthorized(uow, operations.UpdateSubscription(repo, credRepos(t)), operations.UpdateCommand{
			ID: ev.SubscriptionID, EventTypes: binding(&f),
		})
		requireCode(t, err, "FILTER_NOT_SUPPORTED")
	})

	t.Run("an archived pool is refused on create and on update; a suspended one is not", func(t *testing.T) {
		archived := seedPool(t, "refusal-archived-"+tsid.GenerateUntyped(), "ARCHIVED")
		suspended := seedPool(t, "refusal-suspended-"+tsid.GenerateUntyped(), "SUSPENDED")
		_, err := runAuthorized(uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: "refusal-pool", Name: "P", Endpoint: "https://refusal.example.test/hook",
			EventTypes: binding(nil), DispatchPoolID: &archived,
		})
		requireCode(t, err, "POOL_ARCHIVED")

		ev, err := runAuthorized(uow, operations.CreateSubscription(repo, credRepos(t)), operations.CreateCommand{
			Code: "refusal-pool", Name: "P", Endpoint: "https://refusal.example.test/hook",
			EventTypes: binding(nil), DispatchPoolID: &suspended,
		})
		require.NoError(t, err, "a suspended pool's jobs wait; binding to it is allowed")
		_, err = runAuthorized(uow, operations.UpdateSubscription(repo, credRepos(t)), operations.UpdateCommand{
			ID: ev.SubscriptionID, DispatchPoolID: &archived,
		})
		requireCode(t, err, "POOL_ARCHIVED")
	})

	t.Run("sync refuses a filter and a new binding to an archived pool", func(t *testing.T) {
		code := "refarch" + tsid.GenerateUntyped()
		seedPool(t, code, "ARCHIVED")
		sync := func(in operations.SyncSubscriptionInput) error {
			_, err := usecaseop.Run(appAccessCtx(), uow,
				operations.SyncSubscriptions(repo, connection.NewRepository(testpg.Pool(t)), dispatchpool.NewRepository(testpg.Pool(t))),
				operations.SyncSubscriptionsCommand{ApplicationCode: "refusalapp", Subscriptions: []operations.SyncSubscriptionInput{in}},
				testpg.TestEC())
			return err
		}
		err := sync(operations.SyncSubscriptionInput{
			Code: "refusal-sync-filter", Name: "S", Target: "https://refusal.example.test/hook",
			EventTypes: []operations.SyncEventTypeBindingInput{{EventTypeCode: "refusalapp:a:b:c", Filter: &f}},
		})
		requireCode(t, err, "FILTER_NOT_SUPPORTED")
		err = sync(operations.SyncSubscriptionInput{
			Code: "refusal-sync-pool", Name: "S", Target: "https://refusal.example.test/hook",
			EventTypes:       []operations.SyncEventTypeBindingInput{{EventTypeCode: "refusalapp:a:b:c"}},
			DispatchPoolCode: &code,
		})
		requireCode(t, err, "POOL_ARCHIVED")
	})
}
