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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/dispatchqueue"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// DispatchJobsBatchState bundles deps.
type DispatchJobsBatchState struct {
	Repo *dispatchjob.Repository
	// Apps and Subs back the application confinement of application-scoped
	// callers (see checkJobApplication). Nil means every application code /
	// subscription is unknown, so a scoped caller is refused outright;
	// callers with all-applications access never consult them.
	Apps *application.Repository
	Subs *subscription.Repository
	// Clients resolves an item's clientCode to its client (ClientCodes). Nil
	// means every code is unknown, so an item carrying one is refused.
	Clients *client.Repository
	// Principals resolves a serviceAccountId given as a principal id (prn_,
	// which is what an application's own serviceAccountId is) to the service
	// account it fronts (sac_), the id a delivery is signed by. Nil leaves a
	// principal id unresolved, and it is refused rather than stored.
	Principals *principal.Repository
}

// resolveJobServiceAccount returns the service-account id a job must carry.
// A sac_ id passes through. A prn_ id (an application's serviceAccountId is its
// principal's id) is resolved to the service account that principal is, so a
// caller does not need to know the two apart; a principal that is not a
// service account is a 400, because a job stored under it could never be
// signed and would only fail at delivery. Anything else is passed through
// unchanged (existing behaviour).
func (s *DispatchJobsBatchState) resolveJobServiceAccount(ctx context.Context, id *string) (*string, error) {
	if id == nil || !strings.HasPrefix(*id, "prn_") {
		return id, nil
	}
	if s.Principals == nil {
		return nil, httperror.BadRequest("SERVICE_ACCOUNT_NOT_FOUND", "serviceAccountId "+*id+" is a principal id and cannot be resolved here; pass the sac_ service account id")
	}
	p, err := s.Principals.FindByID(ctx, ids.PrincipalID(*id))
	if err != nil {
		return nil, usecase.Internal("REPO", "service account lookup failed", err)
	}
	if p == nil || p.ServiceAccountID == nil || *p.ServiceAccountID == "" {
		return nil, httperror.BadRequest("SERVICE_ACCOUNT_NOT_FOUND", "serviceAccountId "+*id+" is not a service account principal")
	}
	return p.ServiceAccountID, nil
}

// checkJobApplication enforces application confinement for an
// application-scoped caller (ac.IsApplicationScoped()); a no-op for callers
// with all-applications access. Returns a non-empty message when the job must
// be refused. The rule, both of which must hold:
//
//  1. the application named by the first segment of the job's `code`
//     (`{application}:...` — the same split_part(code, ':', 1) the platform
//     uses as the job's application facet) exists and the caller can access it;
//  2. if the job carries a subscriptionId, that subscription exists, has an
//     application_code, and the caller can access that application — a job
//     cannot be routed through another application's subscription.
//
// An unknown application or subscription is refused (indistinguishable from an
// inaccessible one).
func (s *DispatchJobsBatchState) checkJobApplication(ctx context.Context, ac *auth.AuthContext, scope *AppScope, subApp map[string]string, j *dispatchjob.DispatchJob) (string, error) {
	if !ac.IsApplicationScoped() {
		return "", nil
	}
	ok, err := scope.CanAccessCode(ctx, ac, FirstSegment(j.Code))
	if err != nil {
		return "", err
	}
	if !ok {
		return "dispatch job code belongs to an application this credential cannot ingest for", nil
	}
	if j.SubscriptionID == nil || *j.SubscriptionID == "" {
		return "", nil
	}
	subID := *j.SubscriptionID
	appCode, seen := subApp[subID]
	if !seen {
		if s.Subs != nil {
			sub, err := s.Subs.FindByID(ctx, subID)
			if err != nil {
				return "", usecase.Internal("REPO", "subscription find_by_id failed", err)
			}
			if sub != nil && sub.ApplicationCode != nil {
				appCode = *sub.ApplicationCode
			}
		}
		subApp[subID] = appCode
	}
	ok, err = scope.CanAccessCode(ctx, ac, appCode)
	if err != nil {
		return "", err
	}
	if !ok {
		return "subscription belongs to an application this credential cannot ingest for", nil
	}
	return "", nil
}

// BatchItem is one row in the inbound batch.
type BatchItem struct {
	ID                 *string `json:"id,omitempty"` // SDK may supply for idempotency; otherwise minted server-side
	ExternalID         *string `json:"externalId,omitempty"`
	Kind               string  `json:"kind" required:"false"`
	Code               string  `json:"code"`
	Source             *string `json:"source,omitempty"`
	Subject            *string `json:"subject,omitempty"`
	TargetURL          string  `json:"targetUrl"`
	Payload            *string `json:"payload,omitempty"`
	PayloadContentType string  `json:"payloadContentType,omitempty"`
	DataOnly           bool    `json:"dataOnly" required:"false"`
	EventID            *string `json:"eventId,omitempty"`
	CorrelationID      *string `json:"correlationId,omitempty"`
	ClientID           *string `json:"clientId,omitempty"`
	// ClientCode is the client's identifier slug, resolved to its id when no
	// clientId is given (ResolveJobClient); never stored itself.
	ClientCode       *string                `json:"clientCode,omitempty"`
	SubscriptionID   *string                `json:"subscriptionId,omitempty"`
	ServiceAccountID *string                `json:"serviceAccountId,omitempty"`
	DispatchPoolID   *string                `json:"dispatchPoolId,omitempty"`
	MessageGroup     *string                `json:"messageGroup,omitempty"`
	Mode             string                 `json:"mode,omitempty"`
	Sequence         int32                  `json:"sequence,omitempty"`
	TimeoutSeconds   uint32                 `json:"timeoutSeconds,omitempty"`
	MaxRetries       uint32                 `json:"maxRetries,omitempty"`
	Metadata         []dispatchjob.Metadata `json:"metadata,omitempty"`
	// Queue is the job's own dispatch priority: DEFAULT or HIGH_PRIORITY,
	// matched ignoring case, same as a subscription's. Validated on the way
	// in — anything else is a 400 naming the field. Omitted means "not
	// asked for" and stays distinguishable from an explicit DEFAULT: the
	// column is left nil rather than defaulted, so resolution still falls
	// through to the subscription (docs/spec/dispatch-job-priority.md R3).
	Queue *string `json:"queue,omitempty"`
	// Descriptor is what the job is, in words, for the dispatch-jobs grid.
	// Optional.
	Descriptor *string `json:"descriptor,omitempty" maxLength:"255"`
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

// RegisterRoutes mounts the SDK dispatch-job ingest endpoints:
// POST /api/dispatch-jobs (singular) and /api/dispatch-jobs/batch.
func RegisterRoutes(r chi.Router, s *DispatchJobsBatchState) {
	r.Post("/api/dispatch-jobs", s.createOne)
	r.Post("/api/dispatch-jobs/batch", s.batchIngest)
}

// jobFromItem maps one inbound ingest item to a DispatchJob with the
// shared ingest defaults. Used by both the batch loop and the singular
// create so the two endpoints persist identically-shaped rows.
//
// it.Kind is wire input, not internal state, so an unrecognised (non-empty)
// value is a caller error (400 VALIDATION), not a silent coercion to EVENT
// — X-06's write-boundary rejection. Empty (the field omitted) is the
// documented "unspecified" case and defaults to EVENT, same as
// common.ParseDispatchMode's empty-means-unspecified handling for Mode
// just below (Mode itself stays lenient by ruling X-01 either way).
func jobFromItem(it BatchItem) (dispatchjob.DispatchJob, error) {
	kind := dispatchjob.KindEvent
	if it.Kind != "" {
		k, ok := dispatchjob.ParseKind(it.Kind)
		if !ok {
			return dispatchjob.DispatchJob{}, httperror.BadRequest(
				"INVALID_KIND", fmt.Sprintf("unknown dispatch job kind %q; must be EVENT or TASK", it.Kind))
		}
		kind = k
	}
	// it.Queue is wire input, validated the same way a subscription's is
	// (dispatchqueue.Parse): DEFAULT | HIGH_PRIORITY only, case-insensitive
	// — anything else is a 400 naming the field, and no job row is written
	// (docs/spec/dispatch-job-priority.md R3). Absent stays absent: only a
	// recognised non-blank value is stored, so "not asked for" never
	// collapses into a stored DEFAULT.
	if err := netguard.Default.ValidateURL(it.TargetURL); err != nil {
		return dispatchjob.DispatchJob{}, httperror.BadRequest("INVALID_TARGET_URL", "targetUrl "+err.Error())
	}
	var queue *string
	if it.Queue != nil {
		p, err := dispatchqueue.Parse(*it.Queue)
		if err != nil {
			return dispatchjob.DispatchJob{}, err
		}
		if p != "" {
			stored := string(p)
			queue = &stored
		}
	}
	j := dispatchjob.DispatchJob{
		Kind:               kind,
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
		Queue:              queue,
		Descriptor:         it.Descriptor,
	}
	if it.ID != nil && *it.ID != "" {
		j.ID = *it.ID
	} else {
		// 13-char untyped TSID — `msg_dispatch_jobs.id` is VARCHAR(13).
		j.ID = tsid.GenerateUntyped()
	}
	return j, nil
}

// NewJobFromItem is jobFromItem for callers outside the package that create a
// dispatch job on the platform's own behalf (the function runner's dispatch
// capability): the same defaults and the same validation as the SDK ingest
// routes, so a job persists identically whichever way it was created.
func NewJobFromItem(it BatchItem) (dispatchjob.DispatchJob, error) { return jobFromItem(it) }

func (s *DispatchJobsBatchState) batchIngest(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	// Permission: ingest needs a write-dispatch-jobs permission. Service
	// accounts authenticated via client_credentials typically carry this.
	if err := auth.CanWritePermission(ac, "platform:messaging:batch:dispatch-jobs-write"); err != nil {
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
	scope := NewAppScope(s.Apps)
	codes := NewClientCodes(s.Clients)
	subApp := map[string]string{}
	for idx, it := range body.Items {
		j, err := jobFromItem(it)
		if err != nil {
			// An unrecognised kind is a whole-batch rejection (400), same as
			// every other required-field validation this loop performs —
			// no per-item results are produced yet, so there's nothing to
			// partially commit.
			httperror.Write(w, err)
			return
		}
		// Tenant guard: SDK service accounts can only ingest for clients
		// they have access to; a clientCode resolves (ResolveJobClient for
		// the rule). Any refusal rejects the whole batch.
		cid, err := ResolveJobClient(r.Context(), ac, codes, it.ClientID, it.ClientCode)
		if err != nil {
			httperror.Write(w, err)
			return
		}
		j.ClientID = cid
		if j.ServiceAccountID, err = s.resolveJobServiceAccount(r.Context(), j.ServiceAccountID); err != nil {
			httperror.Write(w, err)
			return
		}
		// Application confinement (application-scoped callers only): any
		// offending item rejects the whole batch, like the client guard above.
		msg, err := s.checkJobApplication(r.Context(), ac, scope, subApp, &j)
		if err != nil {
			httperror.Write(w, err)
			return
		}
		if msg != "" {
			httperror.Write(w, httperror.Forbidden(fmt.Sprintf("items[%d]: %s", idx, msg)))
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
