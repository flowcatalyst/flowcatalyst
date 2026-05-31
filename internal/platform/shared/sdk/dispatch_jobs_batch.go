// Package sdk hosts the /api/dispatch-jobs/batch and /api/events/batch
// endpoints that consumer apps' SDK outbox processors POST to. These
// are infrastructure-processing paths (no UoW — see
// docs/conventions.md §3): high-throughput batch inserts directly into
// msg_dispatch_jobs / msg_events.
//
// Phase 3g ships dispatch_jobs_batch.go as the worked example. The
// events batch endpoint follows the same shape and lands alongside it
// in a focused follow-up.
package sdk

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// DispatchJobsBatchState bundles deps.
type DispatchJobsBatchState struct {
	Repo *dispatchjob.Repository
}

// BatchItem is one row in the inbound batch.
type BatchItem struct {
	ID                 *string                `json:"id,omitempty"` // SDK may supply for idempotency; otherwise minted server-side
	ExternalID         *string                `json:"externalId,omitempty"`
	Kind               string                 `json:"kind"`
	Code               string                 `json:"code"`
	Source             *string                `json:"source,omitempty"`
	Subject            *string                `json:"subject,omitempty"`
	TargetURL          string                 `json:"targetUrl"`
	Payload            *string                `json:"payload,omitempty"`
	PayloadContentType string                 `json:"payloadContentType,omitempty"`
	DataOnly           bool                   `json:"dataOnly"`
	EventID            *string                `json:"eventId,omitempty"`
	CorrelationID      *string                `json:"correlationId,omitempty"`
	ClientID           *string                `json:"clientId,omitempty"`
	SubscriptionID     *string                `json:"subscriptionId,omitempty"`
	ServiceAccountID   *string                `json:"serviceAccountId,omitempty"`
	DispatchPoolID     *string                `json:"dispatchPoolId,omitempty"`
	MessageGroup       *string                `json:"messageGroup,omitempty"`
	Mode               string                 `json:"mode,omitempty"`
	Sequence           int32                  `json:"sequence,omitempty"`
	TimeoutSeconds     uint32                 `json:"timeoutSeconds,omitempty"`
	MaxRetries         uint32                 `json:"maxRetries,omitempty"`
	Metadata           []dispatchjob.Metadata `json:"metadata,omitempty"`
}

// BatchRequest is the inbound POST shape.
type BatchRequest struct {
	Items []BatchItem `json:"items"`
}

// BatchResponse is the outbound JSON.
// BatchResultItem is one per-item outcome. Status is the SCREAMING_SNAKE
// OutboxStatus the outbox dispatcher parses.
type BatchResultItem struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// BatchResponse is the wire body for the batch endpoints: a per-item result
// list, 1:1 with the outbox/SDK contract {results:[{id,status,error?}]}.
type BatchResponse struct {
	Results []BatchResultItem `json:"results"`
}

// RegisterRoutes mounts /api/dispatch-jobs/batch.
func RegisterRoutes(r chi.Router, s *DispatchJobsBatchState) {
	r.Post("/api/dispatch-jobs/batch", s.batchIngest)
}

func (s *DispatchJobsBatchState) batchIngest(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// Permission: ingest needs a write-dispatch-jobs permission. Service
	// accounts authenticated via client_credentials typically carry this.
	if err := auth.CanWritePermission(ac, "WRITE_DISPATCH_JOBS"); err != nil {
		httperror.Write(w, err)
		return
	}

	var body BatchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if len(body.Items) == 0 {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(BatchResponse{Results: []BatchResultItem{}})
		return
	}
	if len(body.Items) > 1000 {
		httperror.Write(w, httperror.BadRequest("BATCH_TOO_LARGE", "max 1000 items per batch"))
		return
	}

	jobs := make([]dispatchjob.DispatchJob, 0, len(body.Items))
	for _, it := range body.Items {
		j := dispatchjob.DispatchJob{
			Kind:               dispatchjob.ParseKind(it.Kind),
			Code:               it.Code,
			Source:             it.Source,
			Subject:            it.Subject,
			TargetURL:          it.TargetURL,
			Protocol:           dispatchjob.ProtocolHTTPWebhook,
			Payload:            it.Payload,
			PayloadContentType: defaultIfEmpty(it.PayloadContentType, "application/json"),
			DataOnly:           it.DataOnly,
			ExternalID:         it.ExternalID,
			EventID:            it.EventID,
			CorrelationID:      it.CorrelationID,
			ClientID:           it.ClientID,
			SubscriptionID:     it.SubscriptionID,
			ServiceAccountID:   it.ServiceAccountID,
			DispatchPoolID:     it.DispatchPoolID,
			MessageGroup:       it.MessageGroup,
			Mode:               common.ParseDispatchMode(it.Mode),
			Sequence:           defaultI32(it.Sequence, 99),
			TimeoutSeconds:     defaultU32(it.TimeoutSeconds, 30),
			MaxRetries:         defaultU32(it.MaxRetries, 3),
			RetryStrategy:      dispatchjob.RetryExponentialBackoff,
			Status:             common.DispatchPending,
			Metadata:           it.Metadata,
		}
		if it.ID != nil && *it.ID != "" {
			j.ID = *it.ID
		} else {
			// 13-char untyped TSID — `msg_dispatch_jobs.id` is VARCHAR(13).
			j.ID = tsid.GenerateUntyped()
		}
		// Tenant guard: SDK service accounts can only ingest for clients
		// they have access to.
		if j.ClientID != nil && !ac.CanAccessClient(*j.ClientID) {
			httperror.Write(w, httperror.Forbidden("No access to client: "+*j.ClientID))
			return
		}
		jobs = append(jobs, j)
	}

	if err := s.Repo.InsertBatch(r.Context(), jobs); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "insert batch failed", err))
		return
	}
	// Per-item result list — 1:1 with the outbox/SDK contract. Insert is
	// all-or-nothing here, so every persisted job reports SUCCESS.
	results := make([]BatchResultItem, len(jobs))
	for i := range jobs {
		results[i] = BatchResultItem{ID: jobs[i].ID, Status: "SUCCESS"}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(BatchResponse{Results: results})
}

func defaultIfEmpty(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func defaultI32(v, fallback int32) int32 {
	if v == 0 {
		return fallback
	}
	return v
}

func defaultU32(v, fallback uint32) uint32 {
	if v == 0 {
		return fallback
	}
	return v
}
