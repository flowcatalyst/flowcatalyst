//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// The CORS origins page end to end against Postgres: add runs the API's use
// case (lower-cased, unique, audited), a duplicate is refused on the field,
// remove deletes and is audited, and a forged post without the permission
// changes nothing.
func TestCorsOriginsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	repo := cors.NewRepository(testpg.Pool(t))
	page := &CorsOrigins{Store: repo, Ops: CorsOps{Repo: repo, UoW: testpg.NewUoW(t)}, Shell: shellForTest()}
	do := func(q req) string {
		t.Helper()
		rec := serveMounted(t, anchorAdmin(), q, page)
		require.Contains(t, []int{http.StatusOK, http.StatusSeeOther}, rec.Code, "%s %s: %s", q.method, q.target, rec.Body.String())
		return rec.Body.String()
	}

	body := do(req{method: http.MethodPost, target: "/ui/cors-origins/new", datastar: true, multipart: true, form: url.Values{
		"origin": {"HTTPS://UiPg.Example.com:8443"}, "description": {"From the UI"},
	}})
	assert.Contains(t, body, "added successfully")
	o, err := repo.FindByOrigin(ctx, "https://uipg.example.com:8443")
	require.NoError(t, err)
	require.NotNil(t, o, "stored lower-cased")
	assert.Equal(t, "From the UI", deref(o.Description))
	assert.Equal(t, 1, auditCount(t, o.ID), "add is audited like the API")

	dup := do(req{method: http.MethodPost, target: "/ui/cors-origins/new", datastar: true, form: url.Values{"origin": {"https://uipg.example.com:8443"}}})
	assert.Contains(t, dup, `id="cors-add-origin-err"`)
	assert.Contains(t, dup, "already exists")
	invalid := do(req{method: http.MethodPost, target: "/ui/cors-origins/new", datastar: true, form: url.Values{"origin": {"https://exa mple.com"}}})
	assert.Contains(t, invalid, "must be a valid URL", "the use case's format rule answers on the field")

	assert.Contains(t, do(req{target: "/ui/cors-origins?q=uipg"}), "uipg.example.com:8443")
	assert.Contains(t, do(req{target: "/ui/cors-origins/" + o.ID + "/delete", datastar: true}), "https://uipg.example.com:8443")

	forged := serveMounted(t, connUser(corsViewPermTest), req{method: http.MethodPost, target: "/ui/cors-origins/" + o.ID + "/delete", datastar: true, form: url.Values{}}, page)
	assert.Contains(t, forged.Body.String(), "fc-flash-error")
	still, err := repo.FindByID(ctx, o.ID)
	require.NoError(t, err)
	assert.NotNil(t, still, "a forged remove deleted nothing")

	assert.Contains(t, do(req{method: http.MethodPost, target: "/ui/cors-origins/" + o.ID + "/delete", datastar: true, form: url.Values{}}), "removed")
	gone, err := repo.FindByID(ctx, o.ID)
	require.NoError(t, err)
	assert.Nil(t, gone)
	assert.Equal(t, 2, auditCount(t, o.ID), "add and remove are each audited")
}
