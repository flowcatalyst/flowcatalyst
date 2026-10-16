package sdk

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func specAPI(t *testing.T) huma.API {
	t.Helper()
	api := humachi.New(chi.NewMux(), huma.DefaultConfig("t", "0"))
	RegisterSpec(api)
	return api
}

func specOp(t *testing.T, api huma.API, method, path string) *huma.Operation {
	t.Helper()
	pi := api.OpenAPI().Paths[path]
	require.NotNil(t, pi, path)
	var op *huma.Operation
	switch method {
	case http.MethodPost:
		op = pi.Post
	case http.MethodGet:
		op = pi.Get
	}
	require.NotNil(t, op, method+" "+path)
	return op
}

// schemaOf resolves the JSON schema an operation documents for its request
// body (response=false) or its given response status (response=true).
func schemaOf(t *testing.T, api huma.API, op *huma.Operation, response bool, status string) *huma.Schema {
	t.Helper()
	var s *huma.Schema
	if response {
		r := op.Responses[status]
		require.NotNil(t, r, "response %s", status)
		s = r.Content["application/json"].Schema
	} else {
		s = op.RequestBody.Content["application/json"].Schema
	}
	require.NotNil(t, s)
	if s.Ref != "" {
		s = api.OpenAPI().Components.Schemas.SchemaFromRef(s.Ref)
	}
	return s
}

func validate(t *testing.T, api huma.API, s *huma.Schema, mode huma.ValidateMode, raw string) []error {
	t.Helper()
	var v any
	require.NoError(t, json.Unmarshal([]byte(raw), &v))
	res := &huma.ValidateResult{}
	huma.Validate(api.OpenAPI().Components.Schemas, s, huma.NewPathBuffer([]byte(""), 0), mode, v, res)
	return res.Errors
}

func TestSpec_RoutesAndStatuses(t *testing.T) {
	api := specAPI(t)

	single := specOp(t, api, http.MethodPost, "/api/dispatch-jobs")
	assert.Equal(t, "createDispatchJob", single.OperationID)
	assert.Contains(t, single.Responses, "201")

	batch := specOp(t, api, http.MethodPost, "/api/dispatch-jobs/batch")
	assert.Equal(t, "batchCreateDispatchJobs", batch.OperationID)
	assert.Contains(t, batch.Responses, "201", "the handler answers 201 for a non-empty batch")

	audit := specOp(t, api, http.MethodPost, "/api/audit-logs/batch")
	assert.Equal(t, "batchIngestAuditLogs", audit.OperationID)
	assert.Contains(t, audit.Responses, "200", "the audit batch answers 200")
	assert.NotContains(t, audit.Responses, "201")
}

// The documented required fields are exactly the ones the handlers insist on.
func TestSpec_RequiredFieldsMatchHandlers(t *testing.T) {
	api := specAPI(t)
	reg := api.OpenAPI().Components.Schemas

	req := func(name string) []string { return reg.Map()[name].Required }
	assert.ElementsMatch(t, []string{"code", "targetUrl", "payload", "serviceAccountId"}, req("CreateDispatchJobRequest"),
		"createOne 400s on a missing code, targetUrl, payload or serviceAccountId")
	assert.ElementsMatch(t, []string{"code", "targetUrl"}, req("BatchItem"),
		"the batch tolerates a missing kind (defaults EVENT) and dataOnly (defaults false)")
	assert.ElementsMatch(t, []string{"entityType", "entityId", "operation", "principalId"}, req("AuditBatchItem"))
	assert.ElementsMatch(t, []string{"items"}, req("DispatchJobBatchRequest"))
	assert.ElementsMatch(t, []string{"items"}, req("AuditBatchRequest"))
	assert.ElementsMatch(t, []string{"results"}, req("DispatchJobBatchResponse"))
	assert.ElementsMatch(t, []string{"results"}, req("AuditBatchResponse"))
	assert.ElementsMatch(t, []string{"id", "status"}, req("DispatchJobBatchResult"))
	assert.ElementsMatch(t, []string{"id", "status"}, req("AuditBatchResult"))
}

// A body the SDKs send validates against the documented schemas, and a body
// the handlers would refuse (missing required field) does not.
func TestSpec_RequestSchemasAcceptSDKBodiesAndRejectMissingRequired(t *testing.T) {
	api := specAPI(t)

	single := schemaOf(t, api, specOp(t, api, http.MethodPost, "/api/dispatch-jobs"), false, "")
	assert.Empty(t, validate(t, api, single, huma.ModeWriteToServer,
		`{"code":"a:b:c","targetUrl":"https://x.test/h","payload":"{}","serviceAccountId":"sa1","metadata":{"k":"v"},"queue":"HIGH_PRIORITY"}`))
	assert.NotEmpty(t, validate(t, api, single, huma.ModeWriteToServer,
		`{"code":"a:b:c","targetUrl":"https://x.test/h","serviceAccountId":"sa1"}`), "payload is required")

	batch := schemaOf(t, api, specOp(t, api, http.MethodPost, "/api/dispatch-jobs/batch"), false, "")
	assert.Empty(t, validate(t, api, batch, huma.ModeWriteToServer,
		`{"items":[{"code":"a:b:c","targetUrl":"https://x.test/h"},{"kind":"TASK","code":"a:b:d","targetUrl":"https://x.test/h","dataOnly":true,"metadata":[{"key":"k","value":"v"}]}]}`))
	assert.NotEmpty(t, validate(t, api, batch, huma.ModeWriteToServer, `{"items":[{"code":"a:b:c"}]}`), "targetUrl is required")

	audit := schemaOf(t, api, specOp(t, api, http.MethodPost, "/api/audit-logs/batch"), false, "")
	assert.Empty(t, validate(t, api, audit, huma.ModeWriteToServer,
		`{"items":[{"entityType":"widget","entityId":"w1","operation":"CREATE","principalId":"prn_1","operationData":"{\"a\":1}","applicationCode":"orders","clientCode":"acme","performedAt":"2026-01-01T00:00:00Z"}]}`))
	assert.NotEmpty(t, validate(t, api, audit, huma.ModeWriteToServer, `{"items":[{"entityType":"widget","entityId":"w1"}]}`), "operation is required")
}

// Each spec-unique twin encodes exactly like the handler's own type, so the
// documented response cannot drift from what the handler writes.
func TestSpec_TwinTypesEncodeLikeHandlerTypes(t *testing.T) {
	enc := func(v any) string {
		b, err := json.Marshal(v)
		require.NoError(t, err)
		return string(b)
	}
	items := []BatchResultItem{{ID: "i1", Status: "SUCCESS"}, {Status: "SKIPPED", Error: "e"}}
	real := enc(BatchResponse{Results: items})

	dj := DispatchJobBatchResponse{}
	au := AuditBatchResponse{}
	for _, it := range items {
		dj.Results = append(dj.Results, DispatchJobBatchResult(it))
		au.Results = append(au.Results, AuditBatchResult(it))
	}
	assert.JSONEq(t, real, enc(dj))
	assert.JSONEq(t, real, enc(au))
	assert.JSONEq(t, enc(CreatedResponse{ID: "x"}), enc(DispatchJobCreatedResponse(CreatedResponse{ID: "x"})))
	assert.JSONEq(t, enc(BatchRequest{Items: []BatchItem{{Code: "c"}}}), enc(DispatchJobBatchRequest(BatchRequest{Items: []BatchItem{{Code: "c"}}})))
}
