//go:build integration

package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestEditPayload_HTTP(t *testing.T) {
	httpcompat.Init() // the production error model: usecase errors -> their HTTP status
	pool := testpg.Pool(t)
	s := newState(pool)
	ctx := context.Background()
	failed := seedJobs(t, pool, "edpwa", "edptest:job", "FAILED", nil, 1)[0]
	done := seedJobs(t, pool, "edpwb", "edptest:job", "COMPLETED", nil, 1)[0]
	_, err := pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET payload = '{"a":1}', payload_content_type = 'application/json' WHERE id = $1`, failed)
	require.NoError(t, err)

	caller := auth.FromContext(anchor())
	serve := func(c *auth.AuthContext) humatest.TestAPI {
		_, api := humatest.New(t)
		api.UseMiddleware(func(h huma.Context, next func(huma.Context)) {
			next(huma.WithContext(h, auth.WithContext(h.Context(), c)))
		})
		Register(api, s)
		httpcompat.RelaxRequestBodies(api)
		return api
	}
	api := serve(caller)

	// Success on both mounts; the response is the job, still FAILED, attempts untouched.
	for _, base := range []string{"/api/dispatch-jobs/", "/bff/dispatch-jobs/"} {
		r := api.Put(base+failed+"/payload", map[string]any{"payload": `{"fixed":true}`})
		require.Equal(t, http.StatusOK, r.Code, r.Body.String())
		var body map[string]any
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &body))
		assert.Equal(t, `{"fixed":true}`, body["payload"])
		assert.Equal(t, "FAILED", body["status"])
		assert.EqualValues(t, 3, body["attemptCount"])
	}

	// Content type change.
	r := api.Put("/api/dispatch-jobs/"+failed+"/payload", map[string]any{"payload": "<a/>", "payloadContentType": "application/xml"})
	require.Equal(t, http.StatusOK, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "application/xml")

	// Refusals.
	r = api.Put("/api/dispatch-jobs/"+done+"/payload", map[string]any{"payload": "{}"})
	assert.Equal(t, http.StatusConflict, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "NOT_EDITABLE")
	r = api.Put("/api/dispatch-jobs/"+failed+"/payload", map[string]any{"payload": "", "payloadContentType": "application/json"})
	assert.GreaterOrEqual(t, r.Code, 400, "empty payload is refused: %s", r.Body.String())
	r = api.Put("/api/dispatch-jobs/"+failed+"/payload", map[string]any{"payload": "{nope", "payloadContentType": "application/json"})
	assert.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
	assert.Contains(t, r.Body.String(), "PAYLOAD_NOT_JSON")
	r = api.Put("/api/dispatch-jobs/nosuchjob/payload", map[string]any{"payload": "{}"})
	assert.Equal(t, http.StatusNotFound, r.Code, r.Body.String())

	// Without the dispatch-job permission: 403, nothing written.
	noPerm := serve(&auth.AuthContext{PrincipalID: "prn_edpnoperm", Scope: auth.ScopeAnchor})
	r = noPerm.Put("/api/dispatch-jobs/"+failed+"/payload", map[string]any{"payload": `{"x":1}`})
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())

	// A client-scoped caller cannot edit a platform job.
	other := serve(&auth.AuthContext{PrincipalID: "prn_edpclient", Scope: auth.ScopeClient, Clients: []string{"clt_other"}, Permissions: []string{viewPerm}})
	r = other.Put("/api/dispatch-jobs/"+failed+"/payload", map[string]any{"payload": `{"x":1}`})
	assert.Equal(t, http.StatusForbidden, r.Code, r.Body.String())

	var payload string
	require.NoError(t, pool.QueryRow(ctx, `SELECT payload FROM msg_dispatch_jobs WHERE id = $1`, failed).Scan(&payload))
	assert.Equal(t, "<a/>", payload)
}
