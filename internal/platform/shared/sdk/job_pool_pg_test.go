//go:build integration

package sdk

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// Owner ruling 2026-11-09: an ARCHIVED pool takes no new jobs — the singular
// create and the batch (whole batch) answer 409 POOL_ARCHIVED; a SUSPENDED
// pool's jobs are accepted (they wait).
func TestCreateDispatchJob_ArchivedPoolIsRefused(t *testing.T) {
	pool := testpg.Pool(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO msg_dispatch_pools (id, code, name, status) VALUES
		   ('dpl_sdkarch001', 'sdk-arch', 'A', 'ARCHIVED'), ('dpl_sdksusp001', 'sdk-susp', 'S', 'SUSPENDED')
		 ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), anchorAC())))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{Repo: dispatchjob.NewRepository(pool), Pools: dispatchpool.NewRepository(pool)})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	item := func(poolID string) string {
		return `{"code":"it:pool:job:created","targetUrl":"https://target.test/hook","payload":"{}",
			"serviceAccountId":"sadjpool00001","dispatchPoolId":"` + poolID + `"}`
	}
	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", item("dpl_sdkarch001"))
	assert.Equal(t, http.StatusConflict, resp.StatusCode, body)
	assert.Contains(t, body, "POOL_ARCHIVED")

	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs/batch",
		`{"items":[`+item("dpl_sdksusp001")+`,`+item("dpl_sdkarch001")+`]}`)
	assert.Equal(t, http.StatusConflict, resp.StatusCode, body)

	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs", item("dpl_sdksusp001"))
	assert.Equal(t, http.StatusCreated, resp.StatusCode, body)
}
