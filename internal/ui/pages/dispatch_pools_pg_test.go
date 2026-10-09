//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// auditCount is the number of aud_logs rows the use cases wrote for entity id.
func auditCount(t *testing.T, id string) int {
	t.Helper()
	var n int
	require.NoError(t, testpg.Pool(t).QueryRow(context.Background(), `SELECT count(*) FROM aud_logs WHERE entity_id = $1`, id).Scan(&n))
	return n
}

// The pools page end to end against Postgres: the UI handlers run the real
// use cases (PoolOps over the unit of work), so the rows, the domain events
// and the audit records are the API's.
func TestDispatchPoolsPageAgainstPostgres(t *testing.T) {
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	repo := dispatchpool.NewRepository(pool)
	clients := client.NewRepository(pool)
	_, err := usecaseop.Run(ctx, testpg.NewUoW(t), clientops.CreateClient(clients), clientops.CreateCommand{Name: "UI Acme", Identifier: "ui-acme"}, testpg.TestEC())
	require.NoError(t, err)

	page := &DispatchPools{Pools: repo, Ops: PoolOps{Repo: repo, UoW: testpg.NewUoW(t)}, Clients: clients, Shell: shellForTest()}
	do := func(q req) string {
		t.Helper()
		return doPage(t, page, clients, q)
	}

	// Create through the dialog, client chosen by identifier.
	body := do(req{
		method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true,
		form: createForm("code", "ui-pg-orders", "name", "UI PG Orders", "rateLimit", "90", "concurrency", "4", "client", "ui-acme"),
	})
	require.Contains(t, body, "Dispatch pool created", body)
	created, err := repo.FindByCode(context.Background(), "ui-pg-orders", clientIDOf(t, clients, "ui-acme"))
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, "UI PG Orders", created.Name)
	assert.Equal(t, int32(4), created.Concurrency)
	require.NotNil(t, created.RateLimit)
	assert.Equal(t, int32(90), *created.RateLimit)
	require.NotNil(t, created.ClientID)
	assert.Equal(t, deref(clientIDOf(t, clients, "ui-acme")), string(*created.ClientID), "the identifier was resolved to the client")
	assert.Contains(t, body, `"selPool":"`+created.ID+`"`, "the new pool opens in the drawer")
	assert.Equal(t, 1, auditCount(t, created.ID), "create is audited like the API")

	// The same code again: the use case's conflict, shown on the code field.
	dup := do(req{
		method: http.MethodPost, target: "/ui/dispatch-pools/new", datastar: true,
		form: createForm("code", "ui-pg-orders", "name", "Again", "client", "ui-acme"),
	})
	assert.Contains(t, dup, `id="pool-form-code-err"`)
	assert.Contains(t, dup, "already exists")

	// Edit: name and concurrency; a blank rate limit keeps the stored one (the SPA's PUT).
	do(req{
		method: http.MethodPost, target: "/ui/dispatch-pools/" + created.ID + "/edit", datastar: true,
		form: url.Values{"name": {"UI PG Orders 2"}, "description": {""}, "rateLimit": {""}, "concurrency": {"6"}, "drawer": {created.ID}},
	})
	edited, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Equal(t, "UI PG Orders 2", edited.Name)
	assert.Equal(t, int32(6), edited.Concurrency)
	require.NotNil(t, edited.RateLimit)
	assert.Equal(t, int32(90), *edited.RateLimit)

	// Suspend, then activate.
	do(req{method: http.MethodPost, target: "/ui/dispatch-pools/" + created.ID + "/suspend", datastar: true, form: url.Values{}})
	got, _ := repo.FindByID(context.Background(), created.ID)
	assert.Equal(t, dispatchpool.StatusSuspended, got.Status)
	do(req{method: http.MethodPost, target: "/ui/dispatch-pools/" + created.ID + "/activate", datastar: true, form: url.Values{}})
	got, _ = repo.FindByID(context.Background(), created.ID)
	assert.Equal(t, dispatchpool.StatusActive, got.Status)

	// Delete from the drawer: the pool is gone and the drawer closes.
	del := do(req{method: http.MethodPost, target: "/ui/dispatch-pools/" + created.ID + "/delete", datastar: true, form: url.Values{"drawer": {created.ID}}})
	assert.Contains(t, del, `"selPool":""`)
	gone, err := repo.FindByID(context.Background(), created.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, 5, auditCount(t, created.ID), "create, update, suspend, activate and delete are each audited")
}

func clientIDOf(t *testing.T, repo *client.Repository, identifier string) *string {
	t.Helper()
	c, err := repo.FindByIdentifier(context.Background(), identifier)
	require.NoError(t, err)
	require.NotNil(t, c)
	id := string(c.ID)
	return &id
}

// doPage serves one request through the page as the anchor admin.
func doPage(t *testing.T, page *DispatchPools, clients ClientDirectory, q req) string {
	t.Helper()
	h := &harness{clients: clients}
	rec := h.serveWith(t, page, anchorAdmin(), q)
	if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
		t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "fc-flash-error") {
		t.Logf("%s %s answered an error: %s", q.method, q.target, rec.Body.String())
	}
	return rec.Body.String()
}
