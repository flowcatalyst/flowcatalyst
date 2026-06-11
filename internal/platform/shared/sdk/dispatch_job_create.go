// dispatch_job_create.go hosts POST /api/dispatch-jobs — the singular
// SDK create, mirroring Rust's create_dispatch_job (dispatch_job/api.rs).
// Registered chi-style alongside the batch endpoint (RegisterRoutes in
// dispatch_jobs_batch.go); the item is mapped through the SAME
// jobFromItem defaults and the SAME repository insert path as the batch.
package sdk

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// CreateDispatchJobRequest is the wire body for POST /api/dispatch-jobs.
// 1:1 with Rust's CreateDispatchJobRequest and the Laravel SDK's
// Model\CreateDispatchJobRequest: required code/targetUrl/payload/
// serviceAccountId; metadata is a string map (the batch items carry the
// entity's [{key,value}] array instead).
type CreateDispatchJobRequest struct {
	Source             *string           `json:"source,omitempty"`
	Kind               string            `json:"kind,omitempty"`
	Code               string            `json:"code"`
	Subject            *string           `json:"subject,omitempty"`
	EventID            *string           `json:"eventId,omitempty"`
	CorrelationID      *string           `json:"correlationId,omitempty"`
	TargetURL          string            `json:"targetUrl"`
	Payload            *string           `json:"payload,omitempty"`
	PayloadContentType string            `json:"payloadContentType,omitempty"`
	DataOnly           bool              `json:"dataOnly,omitempty"`
	ServiceAccountID   *string           `json:"serviceAccountId,omitempty"`
	ClientID           *string           `json:"clientId,omitempty"`
	SubscriptionID     *string           `json:"subscriptionId,omitempty"`
	Mode               string            `json:"mode,omitempty"`
	DispatchPoolID     *string           `json:"dispatchPoolId,omitempty"`
	MessageGroup       *string           `json:"messageGroup,omitempty"`
	Sequence           *int32            `json:"sequence,omitempty"`
	TimeoutSeconds     uint32            `json:"timeoutSeconds,omitempty"`
	MaxRetries         uint32            `json:"maxRetries,omitempty"`
	RetryStrategy      string            `json:"retryStrategy,omitempty"`
	IdempotencyKey     *string           `json:"idempotencyKey,omitempty"`
	ExternalID         *string           `json:"externalId,omitempty"`
	Metadata           map[string]string `json:"metadata,omitempty"`
}

// CreatedResponse is the wire body for POST /api/dispatch-jobs: {id},
// matching Rust's shared::api_common::CreatedResponse and the SDK's
// Model\CreatedResponse decode on 201.
type CreatedResponse struct {
	ID string `json:"id"`
}

// createOne handles POST /api/dispatch-jobs. Same permission + tenant
// guards as the batch; required-field validation mirrors Rust's
// serde-required fields (code, targetUrl, payload, serviceAccountId).
func (s *DispatchJobsBatchState) createOne(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "WRITE_DISPATCH_JOBS"); err != nil {
		httperror.Write(w, err)
		return
	}

	var req CreateDispatchJobRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	// Rust deserializes these as non-Option fields — a missing one is a
	// 400 before the handler body runs. Mirror that here.
	switch {
	case req.Code == "":
		httperror.Write(w, httperror.BadRequest("VALIDATION", "code is required"))
		return
	case req.TargetURL == "":
		httperror.Write(w, httperror.BadRequest("VALIDATION", "targetUrl is required"))
		return
	case req.Payload == nil:
		httperror.Write(w, httperror.BadRequest("VALIDATION", "payload is required"))
		return
	case req.ServiceAccountID == nil:
		httperror.Write(w, httperror.BadRequest("VALIDATION", "serviceAccountId is required"))
		return
	}

	// Tenant guard — same rule as the batch loop.
	if req.ClientID != nil && !ac.CanAccessClient(*req.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to client: "+*req.ClientID))
		return
	}

	// Delegate through the batch item mapping so the singular create and a
	// batch-of-1 persist identically, then layer on the fields only the
	// singular contract carries (retryStrategy, idempotencyKey, metadata map).
	j := jobFromItem(BatchItem{
		ExternalID:         req.ExternalID,
		Kind:               req.Kind,
		Code:               req.Code,
		Source:             req.Source,
		Subject:            req.Subject,
		TargetURL:          req.TargetURL,
		Payload:            req.Payload,
		PayloadContentType: req.PayloadContentType,
		DataOnly:           req.DataOnly,
		EventID:            req.EventID,
		CorrelationID:      req.CorrelationID,
		ClientID:           req.ClientID,
		SubscriptionID:     req.SubscriptionID,
		ServiceAccountID:   req.ServiceAccountID,
		DispatchPoolID:     req.DispatchPoolID,
		MessageGroup:       req.MessageGroup,
		Mode:               req.Mode,
		TimeoutSeconds:     req.TimeoutSeconds,
		MaxRetries:         req.MaxRetries,
	})
	if req.Sequence != nil {
		// Pointer on the singular DTO so an explicit `"sequence": 0` is
		// honored (the batch's value-typed field coerces 0 → default 99).
		j.Sequence = *req.Sequence
	}
	j.RetryStrategy = dispatchjob.ParseRetryStrategy(req.RetryStrategy)
	j.IdempotencyKey = req.IdempotencyKey
	j.Metadata = metadataFromMap(req.Metadata)

	if err := s.Repo.InsertBatch(r.Context(), []dispatchjob.DispatchJob{j}); err != nil {
		httperror.Write(w, usecase.Internal("REPO", "insert failed", err))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(CreatedResponse{ID: j.ID})
}

// metadataFromMap converts the singular contract's string map into the
// entity's [{key,value}] slice, key-sorted for a deterministic stored
// order (Rust iterates a HashMap, so its order is unspecified anyway).
func metadataFromMap(m map[string]string) []dispatchjob.Metadata {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]dispatchjob.Metadata, 0, len(keys))
	for _, k := range keys {
		out = append(out, dispatchjob.Metadata{Key: k, Value: m[k]})
	}
	return out
}
