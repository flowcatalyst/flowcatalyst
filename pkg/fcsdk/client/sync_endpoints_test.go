package client_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
)

func TestConnectionsSync(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["hooks"]}`)
	res, err := client.New(srv.URL).Connections().Sync(context.Background(), "orders", &client.SyncConnectionsRequest{
		ClientID:    "acme",
		Connections: []client.SyncConnectionItem{{Code: "hooks", Name: "Hooks"}},
	}, true)
	require.NoError(t, err)
	assert.Equal(t, "POST", seen.method)
	assert.Equal(t, "/api/applications/orders/connections/sync", seen.path)
	assert.Equal(t, "true", seen.query.Get("removeUnlisted"))
	assert.JSONEq(t, `{"clientId":"acme","connections":[{"code":"hooks","name":"Hooks"}]}`, seen.body)
	assert.Equal(t, []string{"hooks"}, res.SyncedCodes)
}

func TestDocsSync(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applicationCode":"orders","created":2,"updated":0,"deleted":1,"syncedCodes":["a","b"]}`)
	res, err := client.New(srv.URL).Docs().Sync(context.Background(), "orders", &client.SyncDocsRequest{
		Docs: []client.SyncDocInput{{Slug: "a", Content: "# A"}, {Slug: "b", Title: "B", Content: "b"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "/api/applications/orders/docs/sync", seen.path)
	assert.Empty(t, seen.query, "a full replace: no removeUnlisted")
	assert.JSONEq(t, `{"docs":[{"slug":"a","content":"# A"},{"slug":"b","title":"B","content":"b"}]}`, seen.body)
	assert.Equal(t, uint32(1), res.Deleted)
}

// F13/F15: the principal sync result carries the ignored password hashes
// and the skipped role names.
func TestPrincipalsSyncResultCarriesIgnoredHashesAndSkippedRoles(t *testing.T) {
	srv, _ := newMockSrv(t, `{"applicationCode":"orders","created":1,"updated":1,"deleted":0,"syncedCodes":["a@x.io","b@x.io"],
		"passwordHashIgnored":["b@x.io"],"skippedRoles":["orders:typo","ghost:admin"]}`)
	res, err := client.New(srv.URL).Principals().Sync(context.Background(), "orders", &client.SyncPrincipalsRequest{
		Principals: []client.SyncPrincipalItem{{Email: "a@x.io", Name: "A", Active: true}},
	}, false)
	require.NoError(t, err)
	assert.Equal(t, []string{"b@x.io"}, res.PasswordHashIgnored)
	assert.Equal(t, []string{"orders:typo", "ghost:admin"}, res.SkippedRoles)
}

func TestSubscriptionsSyncSendsConnectionCodeAndClient(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["s"]}`)
	_, err := client.New(srv.URL).Subscriptions().Sync(context.Background(), "orders", &client.SyncSubscriptionsRequest{
		ClientID: "acme",
		Subscriptions: []client.SyncSubscriptionItem{{
			Code: "s", Name: "S", Target: "https://x.example.com/h", ConnectionCode: "shared-hooks", SharedConnection: true,
			EventTypes: []client.EventTypeBinding{{EventTypeCode: "orders:a:b:c"}},
		}},
	}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"clientId":"acme","subscriptions":[{"code":"s","name":"S","target":"https://x.example.com/h",
		"connectionCode":"shared-hooks","sharedConnection":true,"eventTypes":[{"eventTypeCode":"orders:a:b:c"}]}]}`, seen.body)
}

// The process DTOs are the platform's (body/diagramType/tags), not the old
// steps model the platform never had.
func TestProcessesUseThePlatformShape(t *testing.T) {
	srv, seen := newMockSrv(t, `{"id":"prc_1","code":"orders:sales:checkout","name":"Checkout","status":"CURRENT","source":"API",
		"application":"orders","subdomain":"sales","processName":"checkout","body":"graph TD","diagramType":"mermaid","tags":["t"],
		"createdAt":"2026-10-13T00:00:00Z","updatedAt":"2026-10-13T00:00:00Z"}`)
	created, err := client.New(srv.URL).Processes().Create(context.Background(), &client.CreateProcessRequest{
		Code: "orders:sales:checkout", Name: "Checkout", Body: "graph TD", Tags: []string{"t"},
	})
	require.NoError(t, err)
	assert.JSONEq(t, `{"code":"orders:sales:checkout","name":"Checkout","body":"graph TD","tags":["t"]}`, seen.body)
	assert.Equal(t, "prc_1", created.ID)

	p, err := client.New(srv.URL).Processes().Get(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "graph TD", p.Body)
	assert.Equal(t, "mermaid", p.DiagramType)
	assert.Equal(t, "checkout", p.ProcessName)
	assert.Equal(t, []string{"t"}, p.Tags)

	_, err = client.New(srv.URL).Processes().Sync(context.Background(), "orders", &client.SyncProcessesRequest{
		Processes: []client.SyncProcessInput{{Code: "orders:sales:checkout", Name: "Checkout", Body: "graph TD", DiagramType: "mermaid"}},
	}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"processes":[{"code":"orders:sales:checkout","name":"Checkout","body":"graph TD","diagramType":"mermaid"}]}`, seen.body)
}

func TestDispatchPoolSyncSendsRateLimit(t *testing.T) {
	srv, seen := newMockSrv(t, `{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["p"]}`)
	rl := uint32(600)
	_, err := client.New(srv.URL).DispatchPools().Sync(context.Background(), "orders", &client.SyncDispatchPoolsRequest{
		Pools: []client.SyncDispatchPoolItem{{Code: "p", Name: "P", Concurrency: 4, RateLimit: &rl}},
	}, false)
	require.NoError(t, err)
	assert.JSONEq(t, `{"pools":[{"code":"p","name":"P","concurrency":4,"rateLimit":600}]}`, seen.body)
}
