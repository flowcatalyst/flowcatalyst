//go:build integration

package sdk

import (
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpec_RealRoutesMatchDocumentedSchemas posts spec-valid bodies to the
// REAL chi routes and checks the real responses validate against the
// documented response schemas and statuses. The stubs in RegisterSpec are
// therefore proven against the handlers' behaviour, not just their types.
func TestSpec_RealRoutesMatchDocumentedSchemas(t *testing.T) {
	api := specAPI(t)

	t.Run("POST /api/dispatch-jobs", func(t *testing.T) {
		srv, _ := newIngestServer(t, anchorAC())
		op := specOp(t, api, http.MethodPost, "/api/dispatch-jobs")
		body := `{"code":"spec:single:created","targetUrl":"https://target.test/hook","payload":"{}","serviceAccountId":"saspecsingle1"}`
		require.Empty(t, validate(t, api, schemaOf(t, api, op, false, ""), huma.ModeWriteToServer, body))

		resp, got := postJSON(t, srv.URL+"/api/dispatch-jobs", body)
		require.Equal(t, http.StatusCreated, resp.StatusCode, got)
		assert.Empty(t, validate(t, api, schemaOf(t, api, op, true, "201"), huma.ModeReadFromServer, got))

		// Documented-required field missing: the handler refuses it too.
		resp, _ = postJSON(t, srv.URL+"/api/dispatch-jobs", `{"code":"spec:single:created","targetUrl":"https://target.test/hook","serviceAccountId":"saspecsingle1"}`)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("POST /api/dispatch-jobs/batch", func(t *testing.T) {
		srv, _ := newIngestServer(t, anchorAC())
		op := specOp(t, api, http.MethodPost, "/api/dispatch-jobs/batch")
		// Only the documented-required fields: kind and dataOnly omitted.
		body := `{"items":[{"code":"spec:batch:created","targetUrl":"https://target.test/hook"},{"code":"spec:batch:created2","targetUrl":"https://target.test/hook","payload":"{}"}]}`
		require.Empty(t, validate(t, api, schemaOf(t, api, op, false, ""), huma.ModeWriteToServer, body))

		resp, got := postJSON(t, srv.URL+"/api/dispatch-jobs/batch", body)
		require.Equal(t, http.StatusCreated, resp.StatusCode, got)
		assert.Empty(t, validate(t, api, schemaOf(t, api, op, true, "201"), huma.ModeReadFromServer, got))
	})

	t.Run("POST /api/audit-logs/batch", func(t *testing.T) {
		srv, _ := newAuditBatchServer(t, anchorAC())
		op := specOp(t, api, http.MethodPost, "/api/audit-logs/batch")
		body := `{"items":[{"entityType":"widget","entityId":"w_spec_1","operation":"CREATE","principalId":"prn_spec_1"},{"entityType":"widget","entityId":"w_spec_2","operation":"CREATE","principalId":"prn_spec_1","clientCode":"no-such-client"}]}`
		require.Empty(t, validate(t, api, schemaOf(t, api, op, false, ""), huma.ModeWriteToServer, body))

		resp, got := postJSON(t, srv.URL+"/api/audit-logs/batch", body)
		require.Equal(t, http.StatusOK, resp.StatusCode, got)
		assert.Empty(t, validate(t, api, schemaOf(t, api, op, true, "200"), huma.ModeReadFromServer, got))
		assert.Contains(t, got, `"SKIPPED"`, "an unknown clientCode is SKIPPED, as documented")
	})
}
