//go:build integration

package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// An application's serviceAccountId is its principal's id (prn_). A dispatch
// job is signed by the service account (sac_), so a job created with the
// principal id is stored under the account it fronts instead of being created
// unsignable. A principal that is not a service account is refused up front.
func TestCreateDispatchJob_ResolvesPrincipalIDToServiceAccount(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, name, service_account_id) VALUES
		('prn_djsa00000001', 'SERVICE', 'app sa', 'sac_djsa00000001'),
		('prn_djuser000001', 'USER', 'a user', NULL)
		ON CONFLICT (id) DO NOTHING`)
	require.NoError(t, err)

	repo := dispatchjob.NewRepository(pool)
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			next.ServeHTTP(w, req.WithContext(auth.WithContext(req.Context(), anchorAC())))
		})
	})
	RegisterRoutes(r, &DispatchJobsBatchState{Repo: repo, Principals: principal.NewRepository(pool)})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	job := func(sa string) string {
		return `{"kind":"EVENT","code":"it:sa:resolve:done","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"` + sa + `"}`
	}
	stored := func(body string) *string {
		var c struct {
			ID string `json:"id"`
		}
		require.NoError(t, json.Unmarshal([]byte(body), &c))
		j, err := repo.FindByID(ctx, c.ID)
		require.NoError(t, err)
		require.NotNil(t, j)
		return j.ServiceAccountID
	}

	// Singular: prn_ -> sac_.
	resp, body := postJSON(t, srv.URL+"/api/dispatch-jobs", job("prn_djsa00000001"))
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	if sa := stored(body); assert.NotNil(t, sa) {
		assert.Equal(t, "sac_djsa00000001", *sa)
	}

	// A sac_ id passes through untouched.
	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs", job("sac_djsa00000001"))
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	if sa := stored(body); assert.NotNil(t, sa) {
		assert.Equal(t, "sac_djsa00000001", *sa)
	}

	// Batch: same resolution.
	resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs/batch", `{"items":[`+job("prn_djsa00000001")+`]}`)
	require.Equal(t, http.StatusCreated, resp.StatusCode, body)
	var b struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &b))
	j, err := repo.FindByID(ctx, b.Results[0].ID)
	require.NoError(t, err)
	if assert.NotNil(t, j.ServiceAccountID) {
		assert.Equal(t, "sac_djsa00000001", *j.ServiceAccountID)
	}

	// A principal that is not a service account, and one that does not exist,
	// are 400s: such a job could never be signed.
	for _, id := range []string{"prn_djuser000001", "prn_doesnotexist01"} {
		resp, body = postJSON(t, srv.URL+"/api/dispatch-jobs", job(id))
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode, id+": "+body)
		assert.Contains(t, body, "SERVICE_ACCOUNT_NOT_FOUND", id)
	}
}
