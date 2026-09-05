//go:build integration

package sdk

import (
	"context"
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

// TestAuditBatchIngest_MissingPrincipalIDFallsBackToCaller is the regression
// guard for the batch-ingest principal fallback: an item that omits
// principalId must be attributed to the ingesting principal (the auth
// context), not stored with a NULL principal_id — otherwise the by-principal
// filter can never find it.
func TestAuditBatchIngest_MissingPrincipalIDFallsBackToCaller(t *testing.T) {
	ac := &auth.AuthContext{PrincipalID: "prn_audit_batch_caller", Scope: auth.ScopeAnchor}
	srv, repo := newAuditBatchServer(t, ac)

	resp, err := http.Post(srv.URL+"/api/audit-logs/batch", "application/json", strings.NewReader(`{
		"items": [{
			"entityType": "widget",
			"entityId": "w_no_princ_1",
			"operation": "CREATE"
		}]
	}`))
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	logs, err := repo.FindWithFilters(context.Background(), audit.FilterParams{
		EntityID: new("w_no_princ_1"),
		Limit:    10,
	})
	require.NoError(t, err)
	require.Len(t, logs, 1)
	require.NotNil(t, logs[0].PrincipalID, "principal_id must not be NULL when the item omitted principalId")
	assert.Equal(t, ac.PrincipalID, *logs[0].PrincipalID)

	// The by-principal filter must find it too.
	byPrincipal, err := repo.FindWithFilters(context.Background(), audit.FilterParams{
		PrincipalID: &ac.PrincipalID,
		EntityID:    new("w_no_princ_1"),
		Limit:       10,
	})
	require.NoError(t, err)
	require.Len(t, byPrincipal, 1, "by-principal filter must find the entry attributed to the caller")
}

// TestAuditBatchIngest_ExplicitPrincipalIDIsNotOverridden pins that the
// fallback only fires when the item omits principalId — an item that
// explicitly names a different principal keeps that attribution.
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
