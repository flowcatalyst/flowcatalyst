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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The event types page end to end against Postgres: create, edit, add a
// schema, finalise, deprecate, archive and delete through the BFF's use
// cases, each audited; the export reads the stored schema.
func TestEventTypesPageAgainstPostgres(t *testing.T) {
	pool := testpg.Pool(t)
	repo := eventtype.NewRepository(pool)
	page := &EventTypes{Store: repo, Ops: EventTypeOps{Repo: repo, UoW: testpg.NewUoW(t)}, Apps: application.NewRepository(pool), Shell: shellForTest()}
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

	body := do(req{method: http.MethodPost, target: "/ui/event-types/new", datastar: true, form: url.Values{
		"application": {"uipg"}, "subdomain": {"sales"}, "aggregate": {"order"}, "event": {"placed"},
		"name": {"UI PG order placed"}, "description": {"From the UI"}, "clientScoped": {"true"},
	}})
	require.Contains(t, body, "Event type created")
	et, err := repo.FindByCode(context.Background(), "uipg:sales:order:placed")
	require.NoError(t, err)
	require.NotNil(t, et)
	assert.True(t, et.ClientScoped)
	assert.Equal(t, "From the UI", deref(et.Description))
	base := "/ui/event-types/" + et.ID

	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, form: url.Values{"name": {"UI PG order placed 2"}, "description": {""}}})
	schema := `{"type":"object","properties":{"id":{"type":"string"}},"required":["id"]}`
	do(req{method: http.MethodPost, target: base + "/add-schema", datastar: true, form: url.Values{"version": {"1.0"}, "mimeType": {"application/schema+json"}, "schema": {schema}}})
	got, err := repo.FindByID(context.Background(), et.ID)
	require.NoError(t, err)
	assert.Equal(t, "UI PG order placed 2", got.Name)
	require.Len(t, got.SpecVersions, 1)
	assert.JSONEq(t, schema, string(got.SpecVersions[0].SchemaContent), "the document is stored, not a JSON string")
	assert.Equal(t, eventtype.SpecFinalising, got.SpecVersions[0].Status)

	do(req{method: http.MethodPost, target: base + "/schemas/1.0/finalise", datastar: true, form: url.Values{}})
	export := serveMounted(t, anchorAdmin(), req{target: "/ui/event-types/export?app=uipg"}, page)
	assert.Equal(t, "application/zip", export.Header().Get("Content-Type"))
	viewer := do(req{target: base + "/schemas/1.0", datastar: true})
	assert.Contains(t, viewer, "OrderPlacedData", "generated types are named from the code")

	do(req{method: http.MethodPost, target: base + "/schemas/1.0/deprecate", datastar: true, form: url.Values{}})
	do(req{method: http.MethodPost, target: base + "/archive", datastar: true, form: url.Values{}})
	got, _ = repo.FindByID(context.Background(), et.ID)
	assert.Equal(t, eventtype.StatusArchived, got.Status)
	do(req{method: http.MethodPost, target: base + "/delete", datastar: true, form: url.Values{}})
	gone, err := repo.FindByID(context.Background(), et.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, 7, auditCount(t, et.ID), "create, update, add schema, finalise, deprecate, archive and delete are each audited")

	list := do(req{target: "/ui/event-types?app=uipg"})
	assert.True(t, strings.Contains(list, "No event types found"))
}
