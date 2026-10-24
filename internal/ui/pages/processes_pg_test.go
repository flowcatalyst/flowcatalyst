//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The processes page end to end against Postgres: create (tags, source),
// the list's facets, edit (tags and description cleared), archive, delete,
// each through the API's use case and audited like it.
func TestProcessesPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	repo := process.NewRepository(testpg.Pool(t))
	page := &Processes{Store: repo, Ops: ProcessOps{Repo: repo, UoW: testpg.NewUoW(t)}, Shell: shellForTest()}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		if rec.Code != http.StatusOK && rec.Code != http.StatusSeeOther {
			t.Fatalf("%s %s: status %d: %s", q.method, q.target, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	body := do(req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, multipart: true, form: url.Values{
		"code": {"uipg:orders:shipment"}, "name": {"Shipment"}, "description": {"From the UI"}, "tags": {"orders, ui"},
		"body": {"graph TD\n  A --> B"},
	}})
	require.Contains(t, body, "Process created", body)
	p, err := repo.FindByCode(ctx, "uipg:orders:shipment")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, []string{"orders", "ui"}, p.Tags)
	assert.Equal(t, "graph TD\n  A --> B", p.Body)
	assert.Equal(t, "From the UI", deref(p.Description))
	assert.Equal(t, process.StatusCurrent, p.Status)
	assert.Equal(t, 1, auditCount(t, p.ID), "create is audited like the API")
	base := "/ui/processes/" + p.ID

	dup := do(req{method: http.MethodPost, target: "/ui/processes/new", datastar: true, form: url.Values{"code": {"uipg:orders:shipment"}, "name": {"Again"}}})
	assert.Contains(t, dup, `id="proc-form-code-err"`)
	assert.Contains(t, dup, "already exists")

	list := do(req{target: "/ui/processes?app=uipg", datastar: true})
	assert.Contains(t, list, "Shipment")
	assert.Contains(t, list, `<option value="orders">`, "the application's subdomains are the options")
	drawer := do(req{target: base, datastar: true})
	assert.Contains(t, drawer, "A --&gt; B")
	assert.Contains(t, drawer, "<fc-diagram", "the diagram is drawn in the browser; the source stays in the page")

	do(req{method: http.MethodPost, target: base + "/edit", datastar: true, multipart: true, form: url.Values{
		"name": {"Shipment v2"}, "description": {""}, "tags": {""}, "body": {"graph LR\n  X --> Y"}, "drawer": {p.ID},
	}})
	got, err := repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Shipment v2", got.Name)
	assert.Empty(t, got.Tags)
	assert.Nil(t, got.Description, "a blank description is stored as none (bug 25)")
	assert.Equal(t, "graph LR\n  X --> Y", got.Body)

	do(req{method: http.MethodPost, target: base + "/archive", datastar: true, form: url.Values{"drawer": {p.ID}}})
	got, _ = repo.FindByID(ctx, p.ID)
	assert.Equal(t, process.StatusArchived, got.Status)
	assert.Contains(t, do(req{target: base, datastar: true}), base+"/delete")

	deleted := do(req{method: http.MethodPost, target: base + "/delete", datastar: true, form: url.Values{"drawer": {p.ID}}})
	assert.Contains(t, deleted, "Process deleted")
	got, err = repo.FindByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Nil(t, got)
	assert.Equal(t, 4, auditCount(t, p.ID), "create, update, archive and delete are each audited")
}
