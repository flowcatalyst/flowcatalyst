package sdk

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// The ingest endpoints in this package are plain chi handlers (they stream
// large batches and own their own per-item error envelope), so they never
// reach the huma registry and would be missing from the dumped OpenAPI spec.
// RegisterSpec describes them to a huma API for tools/dump-spec ONLY. It must
// never be called from the server wiring: the routes are already mounted by
// RegisterRoutes / RegisterAuditRoutes, and the stub handlers here do nothing.
//
// The request and response types are the handlers' own wire types, so the
// documented shape cannot drift from the decoded shape; spec_test.go also
// drives the real handlers with a body built from the documented schema.

// The unqualified names BatchRequest, BatchResultItem, BatchResponse and
// CreatedResponse are already taken in the spec by other types of the same
// shape (events batch, apicommon), and huma refuses duplicate schema names, so
// the spec-side twins below carry distinct names. spec_test.go proves each
// twin encodes exactly like the handler's own type.

// DispatchJobBatchRequest is BatchRequest under a spec-unique name.
type DispatchJobBatchRequest BatchRequest

// DispatchJobCreatedResponse is CreatedResponse under a spec-unique name.
type DispatchJobCreatedResponse CreatedResponse

// DispatchJobBatchResult is BatchResultItem under a spec-unique name.
type DispatchJobBatchResult BatchResultItem

// DispatchJobBatchResponse is BatchResponse under spec-unique names.
type DispatchJobBatchResponse struct {
	Results []DispatchJobBatchResult `json:"results"`
}

// AuditBatchResult is BatchResultItem under a spec-unique name.
type AuditBatchResult BatchResultItem

// AuditBatchResponse is BatchResponse under spec-unique names.
type AuditBatchResponse struct {
	Results []AuditBatchResult `json:"results"`
}

type specCreateInput struct {
	Body CreateDispatchJobRequest
}

type specCreateOutput struct {
	Body DispatchJobCreatedResponse
}

type specDispatchBatchInput struct {
	Body DispatchJobBatchRequest
}

type specDispatchBatchOutput struct {
	Body DispatchJobBatchResponse
}

type specAuditBatchInput struct {
	Body AuditBatchRequest
}

type specAuditBatchOutput struct {
	Body AuditBatchResponse
}

// RegisterSpec documents POST /api/dispatch-jobs, POST /api/dispatch-jobs/batch
// and POST /api/audit-logs/batch on api. Spec only; see the note above.
func RegisterSpec(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID:   "createDispatchJob",
		Method:        http.MethodPost,
		Path:          "/api/dispatch-jobs",
		Summary:       "Create a single dispatch job (SDK)",
		Description:   "Requires platform:messaging:batch:dispatch-jobs-write. code, targetUrl, payload and serviceAccountId are required. Responds 201 with the new job id.",
		Tags:          []string{"dispatch-jobs"},
		DefaultStatus: http.StatusCreated,
	}, func(context.Context, *specCreateInput) (*specCreateOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID:   "batchCreateDispatchJobs",
		Method:        http.MethodPost,
		Path:          "/api/dispatch-jobs/batch",
		Summary:       "Ingest a batch of dispatch jobs (SDK)",
		Description:   "Requires platform:messaging:batch:dispatch-jobs-write. At most 1000 items; any refused item (unknown kind, bad targetUrl, client or application the credential cannot ingest for) rejects the whole batch, and the insert is all-or-nothing. Responds 201 with one result per item, in order, status SUCCESS; an empty batch responds 200 with an empty results list.",
		Tags:          []string{"dispatch-jobs"},
		DefaultStatus: http.StatusCreated,
	}, func(context.Context, *specDispatchBatchInput) (*specDispatchBatchOutput, error) { return nil, nil })

	huma.Register(api, huma.Operation{
		OperationID:   "batchIngestAuditLogs",
		Method:        http.MethodPost,
		Path:          "/api/audit-logs/batch",
		Summary:       "Ingest a batch of audit log entries (SDK)",
		Description:   "Requires platform:admin:batch:audit-logs-write. At most 100 items. Partial success: each item reports SUCCESS (with the new id), SKIPPED (unknown applicationCode or clientCode, or no access to the client) or BAD_REQUEST (principalId missing, or application not permitted for an application-scoped credential) in its own results slot, in order.",
		Tags:          []string{"audit-logs"},
		DefaultStatus: http.StatusOK,
	}, func(context.Context, *specAuditBatchInput) (*specAuditBatchOutput, error) { return nil, nil })
}
