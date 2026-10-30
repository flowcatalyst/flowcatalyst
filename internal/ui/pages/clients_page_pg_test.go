//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The clients page end to end against Postgres (the API's use cases).
func TestClientsPageAgainstPostgres(t *testing.T) {
	pool := testpg.Pool(t)
	clients := client.NewRepository(pool)
	apps := application.NewRepository(pool)
	configs := application.NewClientConfigRepo(pool)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), testpg.NewUoW(t), appops.CreateApplication(apps), appops.CreateCommand{Code: "ui-cli-app", Name: "UI cli app"}, testpg.TestEC())
	require.NoError(t, err)
	page := &ClientsPage{Clients: clients, Ops: ClientOps{Repo: clients, Apps: apps, Configs: configs, UoW: testpg.NewUoW(t)}, Apps: apps, Configs: configs, Shell: shellForTest()}
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

	require.Contains(t, do(req{method: http.MethodPost, target: "/ui/clients/new", datastar: true, form: url.Values{"name": {"UI PG client"}, "identifier": {"ui-pg-client"}}}), "Client created")
	c, err := clients.FindByIdentifier(context.Background(), "ui-pg-client")
	require.NoError(t, err)
	require.NotNil(t, c)
	base := "/ui/clients/" + string(c.ID)

	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, form: url.Values{"name": {"UI PG client 2"}}})
	do(req{method: http.MethodPost, target: base + "/suspend", datastar: true, form: url.Values{}})
	got, _ := clients.FindByID(context.Background(), string(c.ID))
	assert.Equal(t, "UI PG client 2", got.Name)
	assert.Equal(t, client.StatusSuspended, got.Status)
	assert.Equal(t, "Manual suspension", deref(got.StatusReason))
	do(req{method: http.MethodPost, target: base + "/activate", datastar: true, form: url.Values{}})

	do(req{method: http.MethodPost, target: base + "/applications", datastar: true, form: url.Values{"applicationIds": {ev.ApplicationID}}})
	cfgs, err := configs.FindByClient(context.Background(), string(c.ID))
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	assert.True(t, cfgs[0].Enabled)
	assert.Contains(t, do(req{target: base, datastar: true}), `value="`+ev.ApplicationID+`" checked`)

	do(req{method: http.MethodPost, target: base + "/deactivate", datastar: true, form: url.Values{}})
	inactive, err := clients.FindByID(context.Background(), string(c.ID))
	require.NoError(t, err)
	require.NotNil(t, inactive, "deactivate is a soft delete: the row is kept")
	assert.Equal(t, client.StatusInactive, inactive.Status)
	assert.Equal(t, "Manual deactivation", deref(inactive.StatusReason))
}
