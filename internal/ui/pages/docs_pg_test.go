//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/docsapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

// The Documentation page against the real docsapi.State: the embedded
// published pages and a synced application's pages, under the API's rules.
func TestDocsPageAgainstPostgres(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	apps := application.NewRepository(pool)
	_, err := usecaseop.Run(ctx, testpg.NewUoW(t), appops.CreateApplication(apps), appops.CreateCommand{Code: "uidocsapp", Name: "UI Docs App"}, testpg.TestEC())
	require.NoError(t, err)
	app, err := apps.FindByCode(ctx, "uidocsapp")
	require.NoError(t, err)
	require.NotNil(t, app)
	repo := appdocs.NewRepository(pool)
	_, err = repo.ReplaceForApplication(ctx, string(app.ID), []appdocs.Input{
		{Slug: "guide", Title: "Integration Guide", Content: "# Integration Guide\n\nBody <script>alert(1)</script> with [next](reference.md).\n\n```mermaid\ngraph LR\n  A --> B\n```\n"},
		{Slug: "reference", Title: "Reference", Content: "# Reference\n"},
	})
	require.NoError(t, err)

	page := &Docs{Source: &docsapi.State{AppDocs: repo, Apps: apps}, Shell: shellForTest()}
	do := func(q req) *httptest.ResponseRecorder {
		t.Helper()
		return serveMounted(t, anchorAdmin(), q, page)
	}

	idx := do(req{target: "/ui/platform/docs"})
	require.Equal(t, http.StatusFound, idx.Code)
	first := idx.Header().Get("Location")
	assert.Contains(t, first, "/ui/platform/docs/platform/", "the first published page")

	published := do(req{target: first})
	require.Equal(t, http.StatusOK, published.Code, published.Body.String())
	assert.Contains(t, published.Body.String(), "UI Docs App", "the application's group is in the navigation")
	assert.Contains(t, published.Body.String(), "/ui/platform/docs/uidocsapp/guide")
	assert.Contains(t, published.Body.String(), `<article class="fc-markdown">`)

	guide := do(req{target: "/ui/platform/docs/uidocsapp/guide"})
	require.Equal(t, http.StatusOK, guide.Code, guide.Body.String())
	assert.Contains(t, guide.Body.String(), "Integration Guide")
	assert.NotContains(t, guide.Body.String(), "alert(1)", "raw HTML in a synced page is dropped")
	assert.Contains(t, guide.Body.String(), `href="/ui/platform/docs/uidocsapp/reference"`)
	assert.Contains(t, guide.Body.String(), "<fc-diagram")

	assert.Equal(t, http.StatusNotFound, do(req{target: "/ui/platform/docs/uidocsapp/missing"}).Code)
	assert.Equal(t, http.StatusNotFound, do(req{target: "/ui/platform/docs/no-such-app/guide"}).Code)
	assert.Equal(t, http.StatusNotFound, do(req{target: "/ui/platform/docs/platform/no-such-page"}).Code)

	// The permission is the docs view permission, not anchor scope or config.
	forbidden := serveMounted(t, cfgUser(permCfgView, permCfgUpdate), req{target: "/ui/platform/docs/uidocsapp/guide"}, page)
	assert.Equal(t, http.StatusForbidden, forbidden.Code)
	assert.NotContains(t, forbidden.Body.String(), "Integration Guide")
}
