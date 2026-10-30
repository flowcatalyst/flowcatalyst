package sync_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	gosync "sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/client"
	"github.com/flowcatalyst/go-sdk/sync"
)

type call struct {
	path, query string
	body        map[string]any
}

// recordingPlatform answers every sync endpoint as the platform does and
// records each request. fail names paths to answer with 422.
func recordingPlatform(t *testing.T, fail map[string]bool) (*httptest.Server, func() []call) {
	t.Helper()
	var mu gosync.Mutex
	var calls []call
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		calls = append(calls, call{r.URL.Path, r.URL.RawQuery, body})
		mu.Unlock()
		if fail[r.URL.Path] {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"error":"VALIDATION","message":"nope"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/applications/orders/scheduled-jobs/sync":
			_, _ = w.Write([]byte(`{"applicationCode":"orders","created":["a"],"updated":["b"],"archived":["c"]}`))
		case "/api/applications/orders/openapi/sync":
			_, _ = w.Write([]byte(`{"applicationCode":"orders","specId":"s","version":"1.2.0","status":"CURRENT","hasBreaking":false,"unchanged":false,"archivedPriorVersion":"1.1.0"}`))
		case "/api/applications/orders/principals/sync":
			_, _ = w.Write([]byte(`{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["a@x.io"],"passwordHashIgnored":["b@x.io"],"skippedRoles":["orders:typo"]}`))
		default:
			_, _ = w.Write([]byte(`{"applicationCode":"orders","created":1,"updated":0,"deleted":0,"syncedCodes":["x"]}`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func() []call {
		mu.Lock()
		defer mu.Unlock()
		return append([]call(nil), calls...)
	}
}

func fullSet() *sync.DefinitionSet {
	return sync.ForApplication("orders").ForClient("acme").
		AddRole(sync.MakeRole("admin")).
		AddEventType(sync.MakeEventType("orders:sales:order:placed", "Order placed")).
		AddDispatchPool(sync.MakeDispatchPool("orders-pool", "Orders").WithConcurrency(5).WithRateLimit(600)).
		AddConnection(sync.MakeConnection("orders-hooks", "Orders hooks").WithExternalID("ext-1")).
		AddSubscription(sync.MakeSubscription("ship", "Ship", "https://orders.example.com/hooks/ship").
			WithConnectionCode("orders-hooks").Bind("orders:sales:order:placed", "")).
		AddSubscription(sync.MakeSubscription("audit", "Audit", "https://audit.example.com/in").
			WithSharedConnectionCode("audit-shared").Bind("orders:sales:order:placed", "")).
		AddProcess(sync.MakeProcess("orders:sales:checkout", "Checkout").
			WithBody("graph TD; A-->B").WithDiagramType("mermaid").WithTags("sales")).
		AddScheduledJob(sync.MakeScheduledJob("orders:nightly", "Nightly", "0 0 2 * * *").WithTimezone("Europe/London")).
		AddScheduledJob(sync.MakeScheduledJob("orders:acme-report", "Acme report", "0 0 6 * * MON").ForClient("acme")).
		AddScheduledJob(sync.MakeScheduledJob("orders:hourly", "Hourly", "0 0 * * * *")).
		AddPrincipal(sync.MakePrincipal("a@x.io").WithName("A").WithRoles("orders:admin")).
		WithOpenAPISpec(json.RawMessage(`{"openapi":"3.1.0","info":{"title":"Orders","version":"1.2.0"},"paths":{}}`)).
		AddDoc(sync.MakeDoc("getting-started", "# Getting started\n").WithTitle("Start"))
}

// Every category goes to its endpoint, in the dependency order, with the
// platform's request members.
func TestSynchronizerSyncsEveryCategoryInOrder(t *testing.T) {
	srv, calls := recordingPlatform(t, nil)
	out := sync.NewSynchronizer(client.New(srv.URL)).Sync(context.Background(), fullSet(), sync.WithRemoveUnlisted())
	require.False(t, out.HasErrors(), out.Errors())

	var paths []string
	for _, c := range calls() {
		paths = append(paths, c.path)
	}
	base := "/api/applications/orders/"
	assert.Equal(t, []string{
		base + "roles/sync", base + "event-types/sync", base + "dispatch-pools/sync",
		base + "connections/sync", base + "subscriptions/sync", base + "processes/sync",
		base + "scheduled-jobs/sync", base + "scheduled-jobs/sync",
		base + "principals/sync", base + "openapi/sync", base + "docs/sync",
	}, paths)

	byPath := map[string][]call{}
	for _, c := range calls() {
		byPath[c.path] = append(byPath[c.path], c)
	}
	conn := byPath[base+"connections/sync"][0]
	assert.Equal(t, "removeUnlisted=true", conn.query)
	assert.Equal(t, "acme", conn.body["clientId"])
	assert.Equal(t, []any{map[string]any{"code": "orders-hooks", "name": "Orders hooks", "externalId": "ext-1"}}, conn.body["connections"])

	subs := byPath[base+"subscriptions/sync"][0]
	assert.Equal(t, "acme", subs.body["clientId"])
	items := subs.body["subscriptions"].([]any)
	assert.Equal(t, "orders-hooks", items[0].(map[string]any)["connectionCode"])
	assert.NotContains(t, items[0].(map[string]any), "sharedConnection")
	assert.Equal(t, "audit-shared", items[1].(map[string]any)["connectionCode"])
	assert.Equal(t, true, items[1].(map[string]any)["sharedConnection"])

	pool := byPath[base+"dispatch-pools/sync"][0].body["pools"].([]any)[0].(map[string]any)
	assert.InDelta(t, 600, pool["rateLimit"], 0)
	assert.InDelta(t, 5, pool["concurrency"], 0)

	proc := byPath[base+"processes/sync"][0].body["processes"].([]any)[0].(map[string]any)
	assert.Equal(t, map[string]any{
		"code": "orders:sales:checkout", "name": "Checkout", "body": "graph TD; A-->B",
		"diagramType": "mermaid", "tags": []any{"sales"},
	}, proc)

	// Scheduled jobs: one request per job client, first-seen order; the
	// set's client does not apply to jobs; RemoveUnlisted → archiveUnlisted.
	jobs := byPath[base+"scheduled-jobs/sync"]
	require.Len(t, jobs, 2)
	assert.NotContains(t, jobs[0].body, "clientId")
	assert.Len(t, jobs[0].body["jobs"], 2)
	assert.Equal(t, true, jobs[0].body["archiveUnlisted"])
	assert.Equal(t, "acme", jobs[1].body["clientId"])
	assert.Equal(t, "Europe/London", jobs[0].body["jobs"].([]any)[0].(map[string]any)["timezone"])
	assert.Equal(t, uint32(2), out.ScheduledJobs.Created)
	assert.Equal(t, uint32(2), out.ScheduledJobs.Updated)
	assert.Equal(t, uint32(2), out.ScheduledJobs.Deleted)
	assert.Equal(t, []string{"a", "b", "a", "b"}, out.ScheduledJobs.SyncedCodes)

	spec := byPath[base+"openapi/sync"][0]
	assert.Empty(t, spec.query, "openapi has no removeUnlisted")
	assert.Equal(t, "3.1.0", spec.body["spec"].(map[string]any)["openapi"])
	assert.Equal(t, uint32(1), out.OpenAPI.Updated)
	assert.Equal(t, []string{"1.2.0"}, out.OpenAPI.SyncedCodes)

	docs := byPath[base+"docs/sync"][0]
	assert.Empty(t, docs.query, "docs is a full replace, no removeUnlisted")
	assert.Equal(t, []any{map[string]any{"slug": "getting-started", "title": "Start", "content": "# Getting started\n"}}, docs.body["docs"])

	// F13/F15: the principal sync's ignored hashes and skipped role names.
	assert.Equal(t, []string{"b@x.io"}, out.Principals.PasswordHashIgnored)
	assert.Equal(t, []string{"orders:typo"}, out.Principals.SkippedRoles)
}

// A failed connections sync leaves subscriptions unsent (their connection
// codes would not resolve); every other category still runs.
func TestSynchronizerSkipsSubscriptionsWhenConnectionsFail(t *testing.T) {
	srv, calls := recordingPlatform(t, map[string]bool{"/api/applications/orders/connections/sync": true})
	out := sync.NewSynchronizer(client.New(srv.URL, client.WithRetry(1, 0))).Sync(context.Background(), fullSet(), sync.DefaultOptions())
	require.True(t, out.Connections.IsError())
	require.NotNil(t, out.Subscriptions)
	assert.Contains(t, out.Subscriptions.Error, "connections sync failed")
	for _, c := range calls() {
		assert.NotEqual(t, "/api/applications/orders/subscriptions/sync", c.path)
	}
	require.NotNil(t, out.Docs)
	assert.False(t, out.Docs.IsError())
	assert.Equal(t, map[string]string{"connections": out.Connections.Error, "subscriptions": out.Subscriptions.Error}, out.Errors())
}

// A scheduled-jobs failure for one client is reported with the client and
// does not stop the other client's request.
func TestSynchronizerScheduledJobsReportsTheFailingClient(t *testing.T) {
	var n int
	var mu gosync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		first := n == 1
		mu.Unlock()
		if first {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"FORBIDDEN","message":"no"}`))
			return
		}
		_, _ = w.Write([]byte(`{"applicationCode":"orders","created":["x"],"updated":[],"archived":[]}`))
	}))
	defer srv.Close()
	set := sync.ForApplication("orders").
		AddScheduledJob(sync.MakeScheduledJob("orders:a", "A", "0 0 * * * *").ForClient("beta")).
		AddScheduledJob(sync.MakeScheduledJob("orders:b", "B", "0 0 * * * *"))
	out := sync.NewSynchronizer(client.New(srv.URL, client.WithRetry(1, 0))).Sync(context.Background(), set, sync.ScheduledJobsOnly())
	assert.Contains(t, out.ScheduledJobs.Error, "client beta:")
	assert.Equal(t, uint32(1), out.ScheduledJobs.Created)
}

func TestSynchronizerOpenAPIUnchanged(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"applicationCode":"orders","specId":"s","version":"1.0.0","status":"CURRENT","hasBreaking":false,"unchanged":true}`))
	}))
	defer srv.Close()
	set := sync.ForApplication("orders").WithOpenAPISpec(json.RawMessage(`{"openapi":"3.1.0"}`))
	out := sync.NewSynchronizer(client.New(srv.URL)).Sync(context.Background(), set, sync.OpenAPIOnly())
	assert.Equal(t, uint32(0), out.OpenAPI.Touched())
	assert.Equal(t, []string{"1.0.0"}, out.OpenAPI.SyncedCodes)
}
