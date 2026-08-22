package sdk

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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
	EntityType      string          `json:"entityType"`
	EntityID        string          `json:"entityId"`
	Operation       string          `json:"operation"`
	OperationData   json.RawMessage `json:"operationData,omitempty"`
	PrincipalID     *string         `json:"principalId,omitempty"`
	PerformedAt     *string         `json:"performedAt,omitempty"`
	ApplicationCode *string         `json:"applicationCode,omitempty"`
	ClientCode      *string         `json:"clientCode,omitempty"`
}

// AuditBatchRequest is the inbound POST shape.
type AuditBatchRequest struct {
	Items []AuditBatchItem `json:"items"`
}

// RegisterAuditRoutes mounts /api/audit-logs/batch.
func RegisterAuditRoutes(r chi.Router, s *AuditBatchState) {
	r.Post("/api/audit-logs/batch", s.batchIngest)
}

// batchIngest handles the batch: per-item resolve applicationCode →
// application_id and clientCode → client_id (unknown code → SKIPPED), enforce
// per-item client access (no access → SKIPPED), parse performedAt (RFC3339,
// default now), then insert. Returns {results:[{id,status}]} with status
// SUCCESS or SKIPPED (id is empty for SKIPPED).
func (s *AuditBatchState) batchIngest(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
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

	skipped := BatchResultItem{ID: "", Status: "SKIPPED"}
	results := make([]BatchResultItem, 0, len(body.Items))
	logs := make([]*audit.Log, 0, len(body.Items))

	// Per-request memo maps for code → id resolution (nil value = known
	// missing), so a 100-item batch does one SELECT per DISTINCT code
	// instead of one per item.
	appByCode := map[string]*string{}
	clientByCode := map[string]*string{}

	for i := range body.Items {
		it := &body.Items[i]

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
					id = &v
				}
				appByCode[code] = id
			}
			if id == nil {
				results = append(results, skipped)
				continue
			}
			applicationID = id
		}

		// Resolve client_code → client_id.
		var clientID *string
		if it.ClientCode != nil && *it.ClientCode != "" {
			code := *it.ClientCode
			id, seen := clientByCode[code]
			if !seen {
				c, err := s.Clients.FindByIdentifier(r.Context(), code)
				if err != nil {
					httperror.Write(w, usecase.Internal("REPO", "client find_by_identifier failed", err))
					return
				}
				if c != nil {
					v := c.ID
					id = &v
				}
				clientByCode[code] = id
			}
			if id == nil {
				results = append(results, skipped)
				continue
			}
			clientID = id
		}

		// Per-item client-access check.
		if clientID != nil && !ac.CanAccessClient(*clientID) {
			results = append(results, skipped)
			continue
		}

		// Parse performed_at (RFC3339); default to now on absence/parse failure.
		performedAt := time.Now().UTC()
		if it.PerformedAt != nil {
			if t, err := time.Parse(time.RFC3339, *it.PerformedAt); err == nil {
				performedAt = t.UTC()
			}
		}

		log := &audit.Log{
			ID:            tsid.Generate(tsid.AuditLog),
			EntityType:    it.EntityType,
			EntityID:      it.EntityID,
			Operation:     it.Operation,
			OperationJSON: it.OperationData,
			PrincipalID:   it.PrincipalID,
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
