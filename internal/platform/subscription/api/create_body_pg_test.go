//go:build integration

package api

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// The SPA's create body carries clientScoped and sequence; both are stored
// (bug 9 in docs/ui/README.md; docs/platform-bug-fixes.md). Omitted, they
// keep their defaults.
func TestCreateSubscription_StoresClientScopedAndSequence(t *testing.T) {
	pool := testpg.Pool(t)
	repo := subscription.NewRepository(pool)
	s := &State{Repo: repo, UoW: testpg.NewUoW(t)}
	create := func(body string) *subscription.Subscription {
		t.Helper()
		var req CreateSubscriptionRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		out, err := s.create(testpg.AnchorCtx(), &apicommon.In[CreateSubscriptionRequest]{Body: req})
		require.NoError(t, err)
		got, err := repo.FindByID(context.Background(), out.Body.ID)
		require.NoError(t, err)
		require.NotNil(t, got)
		return got
	}

	spa := create(`{"code":"sub-body-spa","name":"SPA","endpoint":"https://hooks.example.com/a",
		"eventTypes":[{"eventTypeCode":"subbody:orders:order:created"}],
		"queue":"DEFAULT","mode":"NEXT_ON_ERROR","clientScoped":true,"sequence":5}`)
	assert.True(t, spa.ClientScoped)
	assert.Equal(t, int32(5), spa.Sequence)

	plain := create(`{"code":"sub-body-plain","name":"Plain","endpoint":"https://hooks.example.com/b",
		"eventTypes":[{"eventTypeCode":"subbody:orders:order:created"}]}`)
	assert.False(t, plain.ClientScoped)
	assert.Equal(t, int32(99), plain.Sequence)
}
