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
// SDK/outbox-facing audit-ingest endpoint. 1:1 with Rust
// shared/sdk_audit_batch_api.rs.
type AuditBatchState struct {
	Repo    *audit.Repository
	Apps    *application.Repository
	Clients *client.Repository
}

// AuditBatchItem is one inbound audit row. camelCase only (matches the Rust
// BatchAuditLogItem — no snake_case aliases, unlike the events batch).
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

// batchIngest ports Rust batch_audit_logs: per-item resolve applicationCode →
// application_id and clientCode → client_id (unknown code → SKIPPED), enforce
// per-item client access (no access → SKIPPED), parse performedAt (RFC3339,
// default now), then insert. Returns {results:[{id,status}]} with status
// SUCCESS or SKIPPED (id is empty for SKIPPED), 1:1 with Rust.
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

	for i := range body.Items {
		it := &body.Items[i]

		// Resolve application_code → application_id.
		var applicationID *string
		if it.ApplicationCode != nil && *it.ApplicationCode != "" {
			app, err := s.Apps.FindByCode(r.Context(), *it.ApplicationCode)
			if err != nil {
				httperror.Write(w, usecase.Internal("REPO", "application find_by_code failed", err))
				return
			}
			if app == nil {
				results = append(results, skipped)
				continue
			}
			id := app.ID
			applicationID = &id
		}

		// Resolve client_code → client_id.
		var clientID *string
		if it.ClientCode != nil && *it.ClientCode != "" {
			c, err := s.Clients.FindByIdentifier(r.Context(), *it.ClientCode)
			if err != nil {
				httperror.Write(w, usecase.Internal("REPO", "client find_by_identifier failed", err))
				return
			}
			if c == nil {
				results = append(results, skipped)
				continue
			}
			id := c.ID
			clientID = &id
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
		if err := s.Repo.Insert(r.Context(), log); err != nil {
			httperror.Write(w, usecase.Internal("REPO", "insert audit log failed", err))
			return
		}
		results = append(results, BatchResultItem{ID: log.ID, Status: "SUCCESS"})
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(BatchResponse{Results: results})
}
