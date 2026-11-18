package sdk

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// AuditBatchState bundles the deps for POST /api/audit-logs/batch — the
// SDK/outbox-facing audit-ingest endpoint.
type AuditBatchState struct {
	Repo    *audit.Repository
	Apps    *application.Repository
	Clients *client.Repository
}

// AuditBatchItem is one inbound audit row. camelCase only
// (no snake_case aliases, unlike the events batch).
type AuditBatchItem struct {
	EntityType    string          `json:"entityType"`
	EntityID      string          `json:"entityId"`
	Operation     string          `json:"operation"`
	OperationData json.RawMessage `json:"operationData,omitempty"`
	// PrincipalID is required (ruling 2026-09-06 #10b): an audit entry
	// without an actor is refused per item, never defaulted to the caller.
	PrincipalID     string  `json:"principalId"`
	PerformedAt     *string `json:"performedAt,omitempty"`
	ApplicationCode *string `json:"applicationCode,omitempty"`
	ClientCode      *string `json:"clientCode,omitempty"`
}

// AuditBatchRequest is the inbound POST shape.
type AuditBatchRequest struct {
	Items []AuditBatchItem `json:"items"`
}

// RegisterAuditRoutes mounts /api/audit-logs/batch.
func RegisterAuditRoutes(r chi.Router, s *AuditBatchState) {
	r.Post("/api/audit-logs/batch", s.batchIngest)
}

// batchIngest handles the batch: per item, application confinement, then
// applicationCode → application_id and clientCode → client_id (AuditClient),
// then performedAt (RFC3339, default now) and principalId, then one insert.
// Returns {results:[{id,status,error?}]} in item order: SUCCESS with the new
// id, or a refusal with its reason and an empty id. Nothing is dropped
// without a refusal: an item the platform does not store is answered
// BAD_REQUEST or FORBIDDEN, never SKIPPED, so an outbox processor keeps the
// row for an operator instead of clearing it as delivered.
func (s *AuditBatchState) batchIngest(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}

	// Permission: same posture as the events and dispatch-jobs batch endpoints.
	// Held by the built-in application-service role (migration 069) and by
	// platform:*:*:* wildcards.
	if err := auth.CanWritePermission(ac, "platform:admin:batch:audit-logs-write"); err != nil {
		httperror.Write(w, err)
		return
	}

	var body AuditBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if len(body.Items) > 100 {
		httperror.Write(w, httperror.BadRequest("BATCH_TOO_LARGE", "Maximum 100 items per batch"))
		return
	}

	results := make([]BatchResultItem, 0, len(body.Items))
	logs := make([]*audit.Log, 0, len(body.Items))

	// Per-request memo maps for code → id resolution (nil value = known
	// missing), so a 100-item batch does one SELECT per DISTINCT code
	// instead of one per item.
	appByCode := map[string]*string{}
	clients := NewClientCodes(s.Clients)
	scope := NewAppScope(s.Apps)

	for i := range body.Items {
		it := &body.Items[i]

		// Application confinement: an application-scoped caller may only write
		// audit rows attributed to an application it can access, and may not
		// omit the application (an unattributed row would escape the
		// confinement). Refused per item.
		hasApp := it.ApplicationCode != nil && *it.ApplicationCode != ""
		if ac.IsApplicationScoped() {
			if !hasApp {
				results = append(results, BatchResultItem{Status: "BAD_REQUEST", Error: "applicationCode is required for an application-scoped credential"})
				continue
			}
			ok, err := scope.CanAccessCode(r.Context(), ac, *it.ApplicationCode)
			if err != nil {
				httperror.Write(w, err)
				return
			}
			if !ok {
				results = append(results, BatchResultItem{Status: "BAD_REQUEST", Error: "audit entry belongs to an application this credential cannot write for"})
				continue
			}
		}

		// Resolve application_code → application_id.
		var applicationID *string
		if it.ApplicationCode != nil && *it.ApplicationCode != "" {
			code := *it.ApplicationCode
			id, seen := appByCode[code]
			if !seen {
				app, err := s.Apps.FindByCode(r.Context(), code)
				if err != nil {
					httperror.Write(w, usecase.Internal("REPO", "application find_by_code failed", err))
					return
				}
				if app != nil {
					v := app.ID
					id = ids.StringPtr(&v)
				}
				appByCode[code] = id
			}
			if id == nil {
				// Not stored, and not silently: the row stays with its writer.
				results = append(results, BatchResultItem{Status: "BAD_REQUEST", Error: ApplicationNotFound + code})
				continue
			}
			applicationID = id
		}

		// Resolve client_code → client_id, and the caller's reach.
		clientID, refused, err := AuditClient(r.Context(), ac, clients, it.ClientCode)
		if err != nil {
			httperror.Write(w, err)
			return
		}
		if refused != "" {
			results = append(results, BatchResultItem{Status: "FORBIDDEN", Error: refused})
			continue
		}

		// Parse performed_at (RFC3339); default to now on absence/parse failure.
		performedAt := time.Now().UTC()
		if it.PerformedAt != nil {
			if t, err := time.Parse(time.RFC3339, *it.PerformedAt); err == nil {
				performedAt = t.UTC()
			}
		}

		principalID := strings.TrimSpace(it.PrincipalID)
		if principalID == "" {
			results = append(results, BatchResultItem{Status: "BAD_REQUEST", Error: "principalId is required"})
			continue
		}

		// The SDK outbox writers send operationData as a JSON string holding
		// the encoded object; store the object (F13, UnwrapEncodedJSON), and
		// unwrap first so the redaction backstop below can see inside it.
		opData := UnwrapEncodedJSON(it.OperationData)
		// Backstop: an SDK that predates audit redaction, or an app writing its
		// own outbox rows, still never stores a password or secret.
		if len(opData) > 0 {
			if red, err := usecase.RedactAuditJSON(opData, nil); err == nil {
				opData = red
			}
		}

		log := &audit.Log{
			ID:            tsid.Generate(tsid.AuditLog),
			EntityType:    it.EntityType,
			EntityID:      it.EntityID,
			Operation:     it.Operation,
			OperationJSON: opData,
			PrincipalID:   &principalID,
			ApplicationID: applicationID,
			ClientID:      clientID,
			PerformedAt:   performedAt,
		}
		logs = append(logs, log)
		results = append(results, BatchResultItem{ID: log.ID, Status: "SUCCESS"})
	}

	// One pipelined batch insert (single round trip, all-or-nothing) —
	// previously each row was its own auto-committed Exec, so a mid-batch
	// failure left earlier rows committed while the caller saw a 500.
	if err := s.Repo.InsertBatch(r.Context(), logs); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "insert audit logs failed", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(BatchResponse{Results: results})
}

// ApplicationNotFound prefixes the refusal of an audit item whose
// applicationCode names no application.
const ApplicationNotFound = "Application not found: "

// AuditClient is the audit-ingest client rule, per item: the events batch's
// clientCode rule (ResolveEventClient) with its refusal reported in the item's
// results slot instead of refusing the batch, because the audit batch reports
// every outcome per item. An audit row is a fact, like an event, so it is kept:
//
//   - a clientCode naming a client the caller reaches: that client;
//   - a clientCode naming no client, from an anchor: no client (the row is
//     stored platform-scoped, as an anchor's event with an unknown code is);
//   - a clientCode naming no client from a non-anchor, or a client the caller
//     does not reach: refused, "No access to client: <clientCode>" (one answer
//     for both, so it neither confirms a code exists nor reveals a client id);
//   - no clientCode (absent, empty or whitespace): no client, as before.
//
// refused is the refusal's message, "" when the item is accepted; err is a
// lookup failure, which fails the request.
func AuditClient(ctx context.Context, ac *auth.AuthContext, codes *ClientCodes, clientCode *string) (clientID *string, refused string, err error) {
	code := NonBlank(clientCode)
	if code == nil {
		return nil, "", nil
	}
	id, err := codes.Resolve(ctx, *code)
	if err != nil {
		return nil, "", err
	}
	switch {
	case id == nil && ac.IsAnchor():
		return nil, "", nil
	case id == nil || !ac.CanAccessClient(*id):
		return nil, "No access to client: " + *code, nil
	}
	return id, "", nil
}
