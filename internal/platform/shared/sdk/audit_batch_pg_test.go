//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// newAuditBatchServer mounts RegisterAuditRoutes behind a middleware that
// injects the given AuthContext, mirroring newIngestServer in
// dispatch_job_create_pg_test.go (same package, shares its TestMain).
func newAuditBatchServer(t *testing.T, ac *auth.AuthContext) (*httptest.Server, *audit.Repository) {
	t.Helper()
	pool := testpg.Pool(t)
	repo := audit.NewRepository(pool)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), ac)))
		})
	})
	RegisterAuditRoutes(r, &AuditBatchState{
		Repo:    repo,
		Apps:    application.NewRepository(pool),
		Clients: client.NewRepository(pool),
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, repo
}

// TestAuditBatchIngest_MissingPrincipalIDIsRefused pins ruling 2026-09-06
// #10b: an audit entry without an actor is refused per item (BAD_REQUEST in
// its results[] slot), never stored with NULL and never attributed to the
// ingesting caller. The rest of the batch still lands.
func TestAuditBatchIngest_MissingPrincipalIDIsRefused(t *testing.T) {
	ac := &auth.AuthContext{PrincipalID: "prn_audit_batch_caller", Scope: auth.ScopeAnchor}
	srv, repo := newAuditBatchServer(t, ac)

	resp, err := http.Post(srv.URL+"/api/audit-logs/batch", "application/json", strings.NewReader(`{
		"items": [
			{"entityType": "widget", "entityId": "w_no_princ_1", "operation": "CREATE"},
			{"entityType": "widget", "entityId": "w_no_princ_2", "operation": "CREATE", "principalId": "prn_actor_2"}
		]
	}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var body BatchResponse
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	require.Len(t, body.Results, 2)
	assert.Equal(t, "BAD_REQUEST", body.Results[0].Status)
	assert.Equal(t, "principalId is required", body.Results[0].Error)
	assert.Equal(t, "SUCCESS", body.Results[1].Status)

	logs, err := repo.FindWithFilters(context.Background(), audit.FilterParams{
		EntityID: new("w_no_princ_1"),
		Limit:    10,
	})
	require.NoError(t, err)
	require.Empty(t, logs, "an item without principalId must not be stored")

	logs, err = repo.FindWithFilters(context.Background(), audit.FilterParams{
		EntityID: new("w_no_princ_2"),
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, logs, 1, "the valid sibling still lands")
}

// TestAuditBatchIngest_ExplicitPrincipalIDIsNotOverridden pins that the
// item's principalId is stored verbatim — never replaced by the caller.
func TestAuditBatchIngest_ExplicitPrincipalIDIsNotOverridden(t *testing.T) {
	ac := &auth.AuthContext{PrincipalID: "prn_audit_batch_caller_2", Scope: auth.ScopeAnchor}
	srv, repo := newAuditBatchServer(t, ac)

	resp, err := http.Post(srv.URL+"/api/audit-logs/batch", "application/json", strings.NewReader(`{
		"items": [{
			"entityType": "widget",
			"entityId": "w_expl_princ_1",
			"operation": "CREATE",
			"principalId": "prn_explicit_actor"
		}]
	}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	logs, err := repo.FindWithFilters(context.Background(), audit.FilterParams{
		EntityID: new("w_expl_princ_1"),
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].PrincipalID)
	assert.Equal(t, "prn_explicit_actor", *logs[0].PrincipalID)
}
