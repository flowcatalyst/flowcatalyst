//go:build integration

package api

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// T7: "sign" returns a real signature of the job's stored body (which an
// operator can rewrite first), so it is an anchor-tier action.
func TestSign_RequiresAnchor(t *testing.T) {
	httpcompat.Init()
	pool := testpg.Pool(t)
	s := newState(pool)
	s.Plan = func(_ context.Context, j *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error) {
		return &dispatchprocessing.DeliveryPlan{Headers: map[string]string{"X-FlowCatalyst-Signature": "sig-for-" + j.ID}, Body: "{}"}, nil
	}
	client := "clt_signgate"
	job := seedJobs(t, pool, "sgnga", "signgate:job", "FAILED", &client, 1)[0]

	serve := func(c *auth.AuthContext) humatest.TestAPI {
		_, api := humatest.New(t)
		api.UseMiddleware(func(h huma.Context, next func(huma.Context)) {
			next(huma.WithContext(h, auth.WithContext(h.Context(), c)))
		})
		Register(api, s)
		httpcompat.RelaxRequestBodies(api)
		return api
	}

	// A client-tier caller with the raw-view permission and access to the
	// job's client is refused, on both mounts.
	clientTier := serve(&auth.AuthContext{
		PrincipalID: "prn_signclient", Scope: auth.ScopeClient, Clients: []string{client},
		AllApplications: true, Permissions: []string{viewRawPerm},
	})
	for _, base := range []string{"/api/dispatch-jobs/", "/bff/dispatch-jobs/"} {
		r := clientTier.Post(base + job + "/sign")
		assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())
		assert.Contains(t, r.Body.String(), "SCOPE_FORBIDDEN")
		assert.NotContains(t, r.Body.String(), "sig-for-")
	}

	// An operator still gets the plan.
	operator := serve(&auth.AuthContext{
		PrincipalID: "prn_signop", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{viewRawPerm},
	})
	r := operator.Post("/api/dispatch-jobs/" + job + "/sign")
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "sig-for-"+job)

	// An application-scoped anchor is confined to its applications' jobs: this
	// job's application (the first segment of its code) is not one of them.
	appAnchor := serve(&auth.AuthContext{
		PrincipalID: "prn_signapp", Scope: auth.ScopeAnchor, Applications: []string{"app_other"}, Permissions: []string{viewRawPerm},
	})
	r = appAnchor.Post("/api/dispatch-jobs/" + job + "/sign")
	assert.Equal(t, http.StatusNotFound, r.Code, r.Body.String())
}
