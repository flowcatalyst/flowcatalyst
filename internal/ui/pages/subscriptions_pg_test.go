//go:build integration

package pages

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	dpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The subscriptions page end to end against Postgres (the API's use cases).
func TestSubscriptionsPageAgainstPostgres(t *testing.T) {
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	subs := subscription.NewRepository(pool)
	pools := dispatchpool.NewRepository(pool)
	ets := eventtype.NewRepository(pool)

	created, err := usecaseop.Run(ctx, testpg.NewUoW(t), dpops.CreateDispatchPool(pools), dpops.CreateCommand{Code: "ui-sub-pool", Name: "UI sub pool", Concurrency: new(int32(2))}, testpg.TestEC())
	require.NoError(t, err)
	etEv, err := usecaseop.Run(ctx, testpg.NewUoW(t), etops.CreateEventType(ets), etops.CreateCommand{Code: "uisub:sales:order:placed", Name: "UI sub order", Schema: json.RawMessage(`{"type":"object"}`)}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(ctx, testpg.NewUoW(t), etops.FinaliseEventTypeSchema(ets), etops.FinaliseSchemaCommand{EventTypeID: etEv.EventTypeID, Version: "1.0"}, testpg.TestEC())
	require.NoError(t, err)

	page := &Subscriptions{
		Subs: subs, Ops: SubscriptionOps{Repo: subs, UoW: testpg.NewUoW(t)}, Pools: pools,
		Connections: connection.NewRepository(pool), EventTypes: ets, Apps: application.NewRepository(pool),
		Clients: client.NewRepository(pool), Shell: shellForTest(),
	}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		if q.datastar {
			require.NotContains(t, rec.Body.String(), "fc-flash-error", "%s %s", q.method, q.target)
		}
		return rec.Body.String()
	}

	dlg := do(req{target: "/ui/subscriptions/new", datastar: true})
	require.Contains(t, dlg, `value="`+etEv.EventTypeID+`"`, "the finalised event type is offered")

	body := do(req{method: http.MethodPost, target: "/ui/subscriptions/new", datastar: true, form: url.Values{
		"code": {"ui-pg-sub"}, "name": {"UI PG sub"}, "eventTypes": {etEv.EventTypeID}, "endpoint": {"https://hooks.example.com/ui"},
		"queue": {"DEFAULT"}, "dispatchPoolId": {created.PoolID}, "mode": {"IMMEDIATE"}, "maxAgeSeconds": {"600"}, "timeoutSeconds": {"15"}, "delaySeconds": {"0"},
	}})
	require.Contains(t, body, "Subscription created")
	all, err := subs.FindWithFilters(context.Background(), nil, nil)
	require.NoError(t, err)
	var s *subscription.Subscription
	for i := range all {
		if all[i].Code == "ui-pg-sub" {
			s = &all[i]
		}
	}
	require.NotNil(t, s)
	require.Len(t, s.EventTypes, 1)
	assert.Equal(t, "uisub:sales:order:placed", s.EventTypes[0].EventTypeCode)
	assert.Equal(t, "1.0", deref(s.EventTypes[0].SpecVersion))
	assert.Equal(t, created.PoolID, deref(s.DispatchPoolID))
	assert.Equal(t, "DEFAULT", deref(s.Queue))
	assert.Equal(t, int32(600), s.MaxAgeSeconds)
	assert.Contains(t, do(req{target: "/ui/subscriptions"}), "ui-sub-pool", "the pool code is shown for a pool referenced by id")

	base := "/ui/subscriptions/" + s.ID
	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, form: url.Values{
		"name": {"UI PG sub 2"}, "description": {""}, "endpoint": {"https://hooks.example.com/ui2"}, "connectionId": {""}, "queue": {"HIGH_PRIORITY"},
		"maxAgeSeconds": {"600"}, "timeoutSeconds": {"15"}, "delaySeconds": {"0"}, "mode": {"IMMEDIATE"},
	}})
	got, _ := subs.FindByID(context.Background(), s.ID)
	assert.Equal(t, "UI PG sub 2", got.Name)
	assert.Equal(t, "HIGH_PRIORITY", deref(got.Queue))
	do(req{method: http.MethodPost, target: base + "/pause", datastar: true, form: url.Values{}})
	got, _ = subs.FindByID(context.Background(), s.ID)
	assert.Equal(t, subscription.StatusPaused, got.Status)
	do(req{method: http.MethodPost, target: base + "/resume", datastar: true, form: url.Values{}})
	do(req{method: http.MethodPost, target: base + "/delete", datastar: true, form: url.Values{}})
	gone, err := subs.FindByID(context.Background(), s.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, 5, auditCount(t, s.ID), "create, update, pause, resume and delete are each audited")
}
