//go:build integration

package bff

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	etops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// bffGet serves one GET through the BFF event-type routes as ac and returns
// the status and decoded body.
func bffGet(t *testing.T, h http.Handler, ac *auth.AuthContext, target string) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(auth.WithContext(req.Context(), ac))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	return rec.Code, body
}

func toStrings(v any) []string {
	var out []string
	for _, x := range v.([]any) {
		out = append(out, x.(string))
	}
	return out
}

func itemCodes(body map[string]any) []string {
	var out []string
	for _, it := range body["items"].([]any) {
		out = append(out, it.(map[string]any)["code"].(string))
	}
	return out
}

// The BFF event-type reads make GET /api/event-types's checks (bug 1 in
// docs/ui/README.md, docs/platform-bug-fixes.md): the read permission, client
// scoping, application confinement; and the SPA's repeated facet parameters
// match any of their values (bug 2).
func TestBffEventTypeReads_AuthorizedScopedAndMultiValued(t *testing.T) {
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	apps := application.NewRepository(pool)
	clients := client.NewRepository(pool)
	repo := eventtype.NewRepository(pool)

	appA, err := usecaseop.Run(ctx, uow, appops.CreateApplication(apps), appops.CreateCommand{Code: "bffeta", Name: "A"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(ctx, uow, appops.CreateApplication(apps), appops.CreateCommand{Code: "bffetb", Name: "B"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(ctx, uow, appops.CreateApplication(apps), appops.CreateCommand{Code: "bffetc", Name: "C"}, testpg.TestEC())
	require.NoError(t, err)
	// Event types carry no client (msg_event_types has no client column), so
	// client scoping has nothing to hide here; the checks below are the read
	// permission and the application confinement.
	mk := func(code string) string {
		t.Helper()
		ev, err := usecaseop.Run(ctx, uow, etops.CreateEventType(repo), etops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.EventTypeID
	}
	mk("bffeta:orders:order:created")
	mk("bffeta:billing:invoice:sent")
	mk("bffetb:orders:order:created")
	foreign := mk("bffetc:orders:order:shipped")

	r := chi.NewRouter()
	RegisterEventTypes(r, &EventTypesState{Repo: repo, UoW: uow, Apps: apps})
	RegisterFilterOptions(r, &FilterOptionsState{Clients: clients, EventTypes: repo, Apps: apps})

	viewer := &auth.AuthContext{PrincipalID: "prn_bffviewer", Scope: auth.ScopeClient, Clients: []string{"clt_bffmine"}, AllApplications: true,
		Permissions: []string{"platform:messaging:event-type:view"}}
	nobody := &auth.AuthContext{PrincipalID: "prn_bffnobody", Scope: auth.ScopeAnchor, Permissions: []string{"platform:messaging:event:view"}}
	appSA := &auth.AuthContext{PrincipalID: "prn_bffsa", Scope: auth.ScopeAnchor, Applications: []string{appA.ApplicationID},
		Permissions: []string{"platform:application-service:event-type:view"}}

	t.Run("no read permission: every read refused", func(t *testing.T) {
		for _, u := range []string{
			"/bff/event-types", "/bff/event-types/" + foreign,
			"/bff/event-types/filters/applications", "/bff/event-types/filters/subdomains", "/bff/event-types/filters/aggregates",
		} {
			code, _ := bffGet(t, r, nobody, u)
			assert.Equal(t, http.StatusForbidden, code, u)
		}
	})

	t.Run("application confinement for a service account", func(t *testing.T) {
		code, _ := bffGet(t, r, appSA, "/bff/event-types/"+foreign)
		assert.Equal(t, http.StatusForbidden, code)
		code, body := bffGet(t, r, appSA, "/bff/event-types")
		require.Equal(t, http.StatusOK, code)
		assert.ElementsMatch(t, []string{"bffeta:orders:order:created", "bffeta:billing:invoice:sent"}, itemCodes(body))
		code, body = bffGet(t, r, appSA, "/bff/event-types/filters/applications")
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, []string{"bffeta"}, toStrings(body["options"]))
	})

	t.Run("repeated facets match any value", func(t *testing.T) {
		code, body := bffGet(t, r, viewer, "/bff/event-types?application=bffeta&application=bffetb")
		require.Equal(t, http.StatusOK, code)
		assert.ElementsMatch(t, []string{"bffeta:orders:order:created", "bffeta:billing:invoice:sent", "bffetb:orders:order:created"}, itemCodes(body))
		code, body = bffGet(t, r, viewer, "/bff/event-types?application=bffeta&application=bffetb&subdomain=billing&subdomain=nothing")
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, []string{"bffeta:billing:invoice:sent"}, itemCodes(body))
		code, body = bffGet(t, r, viewer, "/bff/event-types/filters/subdomains?application=bffeta&application=bffetb")
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, []string{"billing", "orders"}, toStrings(body["options"]))
		code, body = bffGet(t, r, viewer, "/bff/event-types/filters/aggregates?application=bffeta&application=bffetb&subdomain=orders")
		require.Equal(t, http.StatusOK, code)
		assert.Equal(t, []string{"order"}, toStrings(body["options"]))
	})

}

// The SPA's add-schema body (the textarea's text as a JSON string, with its
// MIME type and schema type) stores the document with those types (bug 3).
func TestBffAddSchema_StoresTheSPAsSchemaAsTheDocument(t *testing.T) {
	ctx := testpg.AnchorCtx()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	repo := eventtype.NewRepository(pool)
	ev, err := usecaseop.Run(ctx, uow, etops.CreateEventType(repo), etops.CreateCommand{Code: "bffschema:orders:order:created", Name: "S"}, testpg.TestEC())
	require.NoError(t, err)

	r := chi.NewRouter()
	RegisterEventTypes(r, &EventTypesState{Repo: repo, UoW: uow, Apps: application.NewRepository(pool)})
	body := `{"version":"1.0","mimeType":"application/json","schemaType":"JSON_SCHEMA","schema":"{\n  \"type\": \"object\"\n}"}`
	req := httptest.NewRequest(http.MethodPost, "/bff/event-types/"+ev.EventTypeID+"/schemas", stringsReader(body))
	req = req.WithContext(auth.WithContext(req.Context(), &auth.AuthContext{
		PrincipalID: "prn_bffschema", Scope: auth.ScopeAnchor, Permissions: []string{"platform:messaging:event-type:update"},
	}))
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	got, err := repo.FindByID(context.Background(), ev.EventTypeID)
	require.NoError(t, err)
	require.Len(t, got.SpecVersions, 1)
	assert.JSONEq(t, `{"type":"object"}`, string(got.SpecVersions[0].SchemaContent))
	assert.Equal(t, "application/json", got.SpecVersions[0].MimeType)
}

func stringsReader(s string) io.Reader { return strings.NewReader(s) }
