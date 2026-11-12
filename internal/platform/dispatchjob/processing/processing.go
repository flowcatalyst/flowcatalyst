// Package processing implements POST /api/dispatch/process — the internal
// callback the message router invokes for each queued dispatch job.
//
// Flow: the scheduler publishes a job to the broker with its mediation_target
// pointed here (not at the subscriber). The router consumes the message and
// POSTs {"messageId": id} to this endpoint, which then:
//
//  1. loads the job and verifies the scheduler-signed bearer token,
//  2. atomically claims it for delivery (a conditional UPDATE that flips
//     PENDING/QUEUED → PROCESSING; a redelivery that loses the race — the
//     row is already PROCESSING or terminal — acks WITHOUT delivering, and a
//     claim that errors NACKs rather than deliver with unknown ownership),
//  3. delivers the real webhook to the subscriber's target_url,
//  4. records the attempt in msg_dispatch_job_attempts,
//  5. advances the job status (COMPLETED / retry-scheduled / FAILED),
//  6. returns {"ack": true} so the router removes the queue message.
//
// Retries are driven by the scheduler poller via scheduled_for, NOT by the
// queue: this endpoint always ACKs and reschedules failed jobs to
// NOW()+backoff, so exactly one component re-dispatches a job (no queue-NACK
// racing the poller into a double dispatch).
//
// Two ways a job used to be lost, now closed (delivery harness, platform-down;
// the Rust platform's dispatch_process_api does the same):
//
//   - An internal error (the database unreachable while loading, holding,
//     claiming or recording) answers 503 {"ack": false}, never 500. Every
//     router treats a 500 as the target's permanent answer (R-57) and ACKs
//     the message away, which left the job QUEUED until stale recovery.
//   - A claim has a lease (claimLease). A copy that loses the claim to a
//     PROCESSING job inside its lease is deferred ({"ack": false,
//     "delaySeconds": <rest of the lease>}) so the router keeps the message
//     and holds its group; one that finds the lease run out takes the claim
//     over and delivers. Before, a copy that lost the claim was always acked
//     away, so a platform killed mid-delivery left the job PROCESSING for
//     good. The delivery also no longer follows the request context: a
//     router hanging up mid-call cannot cut an attempt short, so only a
//     dying process leaves a claim behind. At-least-once: after such a death
//     the subscriber may see the message twice.
package processing

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/netguard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
)

// Signature headers on delivered webhooks. Byte-format matches the router's
// webhook signing and the SDK's WebhookValidator: HMAC-SHA256 over
// `timestamp + body`, millisecond-precision ISO8601 UTC timestamp.
const (
	signatureHeader = "X-FlowCatalyst-Signature"
	timestampHeader = "X-FlowCatalyst-Timestamp"

	// clientHeader names the tenant a multi-tenant subscriber's endpoint is
	// receiving a delivery for — "{clientId}:{clientCode}", the same
	// "{id}:{code}" pair shape the platform's clients/applications claims
	// already use. Sent for dataOnly deliveries too, since that mode's raw
	// body carries no envelope for clientCode to ride in. See
	// docs/spec/webhook-client-code.md R2.
	clientHeader = "X-FlowCatalyst-Client"
)

// DeliveryCredsResolver returns the delivery credentials (bearer token +
// HMAC signing secret) for a dispatch job (zero-value = deliver bare). Wired
// by the server to resolve job → subscription → application →
// service-account webhook credentials.
type DeliveryCredsResolver func(ctx context.Context, job *dispatchjob.DispatchJob) (serviceaccount.OutboundCreds, error)

// ClientCodeResolver returns the client's identifier slug for a job's
// client_id (ok=false when it has none, or the client cannot be resolved).
// Wired by the server to client.NewCachedIdentifierResolver so the delivery
// path pays no DB round trip per job. See docs/spec/webhook-client-code.md.
type ClientCodeResolver func(ctx context.Context, clientID string) (identifier string, ok bool)

// maxResponseBody caps how much of a subscriber response we read into the
// recorded attempt — a hostile or chatty endpoint must not balloon a row.
const maxResponseBody = 64 << 10 // 64 KiB

// defaultTimeout applies when a job carries no explicit timeout_seconds.
const defaultTimeout = 30 * time.Second

// retryBackoff is the "exponential" retry strategy (the default): it maps a
// just-finished attempt number (1-based) to the delay before the poller may
// re-dispatch. Index attemptNumber-1, clamped to the last element.
// Exponential-ish, capped at 2m.
var retryBackoff = []time.Duration{
	5 * time.Second,
	15 * time.Second,
	30 * time.Second,
	60 * time.Second,
	120 * time.Second,
}

// fixedRetryDelay is the "fixed" retry strategy's delay before every retry.
const fixedRetryDelay = 30 * time.Second

// notBeforeRecheck caps how long one not-before deferral asks the router to
// hold a job. A job the router holds stays QUEUED, and stale recovery
// republishes a QUEUED job untouched for 15 minutes; each deferral stamps the
// job (AwaitNotBefore), so a delay longer than this is served as a series of
// holds of at most this long, the last one ending exactly at not_before.
const notBeforeRecheck = 5 * time.Minute

// PoolHeldResolver reports whether a dispatch pool takes no deliveries right
// now — SUSPENDED or ARCHIVED (see dispatchpool.HeldPools). nil = never held.
type PoolHeldResolver func(ctx context.Context, poolID string) (bool, error)

// Verifier checks the HMAC bearer token the router forwards (the scheduler
// signed the job id). Satisfied by *scheduler.DispatchAuthService. previousKey
// reports a token only a previous FLOWCATALYST_APP_KEY accepted.
type Verifier interface {
	Verify(jobID, token string) (ok, previousKey bool)
}

// Handler serves the dispatch-processing callback.
type Handler struct {
	repo     *dispatchjob.Repository
	verifier Verifier
	client   *http.Client
	// fnClient delivers function targets through the internal policy; see
	// WithFunctionTargetResolver.
	fnClient   *http.Client
	isFunction FunctionTargetResolver
	// creds (nil = bare delivery) stamps bearer + signature on subscriber
	// deliveries; see WithDeliveryCredsResolver.
	creds DeliveryCredsResolver
	// clientCode (nil = never resolved) stamps the envelope's clientCode and
	// the X-FlowCatalyst-Client header; see WithClientCodeResolver.
	clientCode ClientCodeResolver
	// poolHeld (nil = pools never hold) hands back jobs of a SUSPENDED or
	// ARCHIVED pool; see WithPoolHeldResolver.
	poolHeld PoolHeldResolver
}

// New wires the handler. verifier may be nil (dev/no-auth), in which case the
// bearer token is not checked — but that is a misconfiguration in any
// deployment where the scheduler signs tokens, so callers should pass one.
func New(repo *dispatchjob.Repository, verifier Verifier) *Handler {
	return &Handler{
		repo:     repo,
		verifier: verifier,
		// Outer ceiling only; each delivery uses a per-job context timeout.
		// No redirect-following: a 3xx from a webhook target is not a success.
		client:   newClient(netguard.Default),
		fnClient: newClient(netguard.Internal),
	}
}

// newClient is the delivery client whose dials policy checks.
func newClient(policy *netguard.Policy) *http.Client {
	return &http.Client{
		Timeout:   deliveryClientCeiling,
		Transport: guardedTransport(policy),
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// FunctionTargetResolver reports whether the platform itself wrote job's
// target for a function (a function subscription's fan-out job), as opposed to
// a customer supplying it. Decide it from stored facts, never from the URL.
type FunctionTargetResolver func(ctx context.Context, job *dispatchjob.DispatchJob) bool

// WithFunctionTargetResolver makes a job the resolver recognises be delivered
// through the internal policy (netguard.Internal), which alone exempts the
// function runner. nil (the default) delivers every job through the customer
// policy.
func (h *Handler) WithFunctionTargetResolver(fn FunctionTargetResolver) *Handler {
	h.isFunction = fn
	return h
}

// WithDeliveryPolicies replaces the policies the delivery dials are checked
// against: customer for customer-supplied targets, internal for the targets
// the function resolver recognises. New uses netguard.Default and
// netguard.Internal; this exists for tests and embedders with their own.
func (h *Handler) WithDeliveryPolicies(customer, internal *netguard.Policy) *Handler {
	h.client = newClient(customer)
	h.fnClient = newClient(internal)
	return h
}

// clientFor picks the delivery client for job: the internal-policy client for
// a function target, the customer-policy client for everything else.
func (h *Handler) clientFor(ctx context.Context, job *dispatchjob.DispatchJob) *http.Client {
	if h.isFunction != nil && h.fnClient != nil && h.isFunction(ctx, job) {
		return h.fnClient
	}
	return h.client
}

// WithDeliveryCredsResolver makes every subscriber delivery carry the same
// header set as router-mediated webhooks: Authorization Bearer (when the SA
// has a token) + the X-FlowCatalyst-Signature/-Timestamp pair (when it has a
// signing secret). Resolution failures and empty creds fall back to bare
// delivery with a warning — the subscriber's fail-closed validator then
// rejects with a descriptive 401 recorded on the attempt.
func (h *Handler) WithDeliveryCredsResolver(fn DeliveryCredsResolver) *Handler {
	h.creds = fn
	return h
}

// WithClientCodeResolver makes every delivery whose client resolves carry
// X-FlowCatalyst-Client: {clientId}:{clientCode} (dataOnly deliveries
// included), and makes the non-dataOnly envelope carry clientCode alongside
// clientId. Never a half pair: a platform-scoped job (no client_id) or a
// client that does not resolve gets neither the header nor the field. See
// docs/spec/webhook-client-code.md R1/R2.
func (h *Handler) WithClientCodeResolver(fn ClientCodeResolver) *Handler {
	h.clientCode = fn
	return h
}

// WithPoolHeldResolver makes the callback hand back, undelivered, a job whose
// dispatch pool is SUSPENDED or ARCHIVED: the job returns to PENDING (no
// attempt spent) and the message is acked. The scheduler does not claim such
// jobs, so they wait in PENDING until the pool is resumed — this catches the
// ones already on the queue when the pool was suspended.
func (h *Handler) WithPoolHeldResolver(fn PoolHeldResolver) *Handler {
	h.poolHeld = fn
	return h
}

// Mount attaches POST /api/dispatch/process to the given (unauthenticated)
// chi router. The handler self-verifies the scheduler HMAC bearer, so it must
// live OUTSIDE the platform JWT middleware.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/dispatch/process", h.serve)
}

type processRequest struct {
	MessageID string `json:"messageId"`
}

// processResponse is the router's contract (see internal/router mediator):
// ack=false with an optional delaySeconds asks the router to retry via the
// queue. A message this endpoint handled is always acked (the poller owns
// retries); ack=false is only for a message it could not handle yet — an
// internal error (503), a delivery still in progress elsewhere (200 with
// delaySeconds, see lostClaim), or a job whose not_before is still ahead
// (200 with notBefore and delaySeconds, see awaitNotBefore).
//
// NotBefore (RFC 3339, UTC, milliseconds) is WHEN to come back: a router
// that reads it redelivers no earlier than that instant and prefers it to
// delaySeconds. DelaySeconds is the same instant as whole seconds from now,
// rounded up, for a router that predates notBefore.
type processResponse struct {
	Ack          bool    `json:"ack"`
	Message      string  `json:"message,omitempty"`
	DelaySeconds *uint32 `json:"delaySeconds,omitempty"`
	NotBefore    *string `json:"notBefore,omitempty"`
}

// claimLeaseMargin is the slack on top of a delivery's own bound before its
// claim counts as dead (see claimLease): the credential lookup before it and
// the attempt and status writes after it.
const claimLeaseMargin = 30 * time.Second

// deliveryClientCeiling is the delivery client's outer timeout (see New).
const deliveryClientCeiling = 2 * time.Minute

// claimLease is how long a claimed attempt may hold its job before a
// redelivery may take it over: the delivery's own bound (the job's timeout,
// capped by the client's ceiling) plus claimLeaseMargin.
func claimLease(job *dispatchjob.DispatchJob) time.Duration {
	d := defaultTimeout
	if job.TimeoutSeconds > 0 {
		d = time.Duration(job.TimeoutSeconds) * time.Second
	}
	return min(d, deliveryClientCeiling) + claimLeaseMargin
}

// unavailable answers 503 {"ack": false}: this endpoint could not handle the
// message (an internal error), so the router must keep it and retry.
func unavailable(w http.ResponseWriter, msg string) {
	writeJSON(w, http.StatusServiceUnavailable, processResponse{Ack: false, Message: msg})
}

// deferred answers 200 {"ack": false, "delaySeconds": n}: come back in n
// seconds (the router holds the message and its group behind it).
func deferred(w http.ResponseWriter, seconds uint32, msg string) {
	writeJSON(w, http.StatusOK, processResponse{Ack: false, Message: msg, DelaySeconds: &seconds})
}

// deferredUntil answers 200 {"ack": false, "notBefore": t, "delaySeconds": n}:
// come back at t (n = t - now in whole seconds, rounded up, at least 1).
func deferredUntil(w http.ResponseWriter, now, t time.Time, msg string) {
	secs := uint32(min((t.Sub(now)+time.Second-1)/time.Second, math.MaxInt32))
	if secs == 0 {
		secs = 1
	}
	at := t.UTC().Format(envelopeTime)
	writeJSON(w, http.StatusOK, processResponse{Ack: false, Message: msg, DelaySeconds: &secs, NotBefore: &at})
}

func writeJSON(w http.ResponseWriter, code int, body processResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// A panic here must not reach the server's recoverer, which answers 500:
	// the router would ACK the message away (R-57) with the job possibly
	// already claimed. 503 keeps the message; if the job was claimed, its
	// next copy takes the claim over once the lease runs out.
	defer func() {
		if rec := recover(); rec != nil {
			slog.Error("dispatch process: panic", "panic", rec, "stack", string(debug.Stack()))
			unavailable(w, "internal error")
		}
	}()

	var req processRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil || strings.TrimSpace(req.MessageID) == "" {
		writeJSON(w, http.StatusBadRequest, processResponse{Ack: true, Message: "invalid messageId"})
		return
	}
	jobID := req.MessageID

	// Verify the scheduler-signed bearer. Absent/invalid → 401, no ack: a
	// forged callback must not be able to trigger deliveries, and the router
	// (which always carries a valid token) will never hit this branch.
	if h.verifier != nil {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		ok, previousKey := false, false
		if token != "" {
			ok, previousKey = h.verifier.Verify(jobID, token)
		}
		dispatchjob.ObserveCallbackAuth(dispatchjob.AuthEndpointProcess, ok, previousKey)
		if !ok {
			// Counted (fc_dispatch_job_auth_rejected_total): a steady stream
			// of these is two processes disagreeing about the app key.
			slog.Warn("dispatch process: bad auth token", "job_id", jobID)
			writeJSON(w, http.StatusUnauthorized, processResponse{Ack: false, Message: "unauthorized"})
			return
		}
	}

	job, err := h.repo.FindByID(ctx, jobID)
	if err != nil {
		// Transient DB error — NACK so the queue redelivers.
		slog.Error("dispatch process: load job failed", "job_id", jobID, "err", err)
		unavailable(w, "load failed")
		return
	}
	if job == nil {
		// The row is gone; nothing to deliver. Ack to drop the message.
		writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "job not found"})
		return
	}
	if job.Status.IsTerminal() {
		// Already COMPLETED/FAILED/CANCELLED/EXPIRED (e.g. a duplicate
		// redelivery). Ack without re-delivering.
		writeJSON(w, http.StatusOK, processResponse{Ack: true})
		return
	}

	// Past its expiry (the subscription's maxAgeSeconds, made absolute at
	// fan-out, or an expiresAt the job was created with): settle it EXPIRED
	// and drop the message. Nothing is sent and no attempt is counted. An
	// EXPIRED job holds no group, so a BLOCK_ON_ERROR group moves on past it.
	now := time.Now()
	if job.ExpiresAt != nil && !now.Before(*job.ExpiresAt) {
		reason := "expired: not delivered by " + job.ExpiresAt.UTC().Format(envelopeTime)
		if err := h.repo.Expire(ctx, jobID, job.CreatedAt, reason); err != nil {
			slog.Error("dispatch process: expire failed", "job_id", jobID, "err", err)
			unavailable(w, "expire failed")
			return
		}
		slog.Info("dispatch expired", "job_id", jobID, "expires_at", job.ExpiresAt)
		writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "expired"})
		return
	}

	// The job's pool is SUSPENDED or ARCHIVED: it waits. Back to PENDING (no
	// attempt spent), message acked; the scheduler leaves it unclaimed until
	// the pool is resumed, and then claims it again in its group's order.
	if job.DispatchPoolID != nil && *job.DispatchPoolID != "" && h.poolHeld != nil {
		held, err := h.poolHeld(ctx, *job.DispatchPoolID)
		if err != nil {
			slog.Error("dispatch process: pool status check failed", "job_id", jobID, "err", err)
			unavailable(w, "pool check failed")
			return
		}
		if held {
			if err := h.repo.Hold(ctx, jobID, job.CreatedAt, now); err != nil {
				slog.Error("dispatch process: suspended-pool revert failed", "job_id", jobID, "err", err)
				unavailable(w, "revert failed")
				return
			}
			slog.Info("dispatch held: pool suspended, returned to PENDING",
				"job_id", jobID, "pool_id", *job.DispatchPoolID)
			writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "pool suspended"})
			return
		}
	}

	// Held-group hold-back at delivery time, for BLOCK_ON_ERROR only — the one
	// mode that promises to stop for a sibling in front of it (IMMEDIATE and
	// NEXT_ON_ERROR keep flowing, matching the poller's filter, and skip the
	// query entirely). The poller stops QUEUEING these jobs once one ahead of
	// them is held, but messages already in the queue at that moment would
	// still arrive here and deliver past it. Mirror the poller instead: ack the
	// queue message (dropping it from the router) and put the job back to
	// PENDING without consuming retry budget — once the job in front gets
	// through (its retry succeeds, or an operator resolves its failure) the
	// group moves and the poller re-queues these in order.
	if job.Mode == common.DispatchBlockOnError && job.MessageGroup != nil && *job.MessageGroup != "" {
		blocked, err := h.repo.GroupHeldBefore(ctx, *job.MessageGroup, job.Sequence, job.CreatedAt, jobID)
		if err != nil {
			// Transient DB error — NACK so the queue redelivers.
			slog.Error("dispatch process: blocked-group check failed", "job_id", jobID, "err", err)
			unavailable(w, "blocked check failed")
			return
		}
		if blocked {
			if err := h.repo.Hold(ctx, jobID, job.CreatedAt, time.Now()); err != nil {
				// Revert failed: NACK rather than ack, or the job would sit
				// QUEUED with no queue message until stale recovery.
				slog.Error("dispatch process: blocked-group revert failed", "job_id", jobID, "err", err)
				unavailable(w, "revert failed")
				return
			}
			slog.Info("dispatch held: group blocked, returned to PENDING",
				"job_id", jobID, "group", *job.MessageGroup)
			writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "group blocked"})
			return
		}
	}

	// Not yet due (its subscription's delaySeconds, made absolute at fan-out):
	// tell the router when to come back, and keep the message. Checked after
	// the hold: a held job goes back to PENDING whatever its delay.
	if job.NotBefore != nil && now.Before(*job.NotBefore) {
		h.awaitNotBefore(ctx, w, job, now)
		return
	}

	// Atomically claim the job for this delivery: a conditional UPDATE
	// guarded on the status it flips FROM (PENDING/QUEUED only), so the
	// affected-row count answers "did I win this delivery?" — see the
	// dispatch-job lifecycle's ClaimForDelivery. This is no longer best-effort like the old
	// unconditional MarkInProgress: a claim error means ownership is unknown,
	// and delivering anyway is exactly the duplicate the guard exists to
	// prevent.
	claimed, err := h.repo.ClaimForDelivery(ctx, jobID, job.CreatedAt)
	if err != nil {
		// Transient DB error — NACK so the queue redelivers. Do NOT deliver:
		// we don't know whether we hold the claim.
		slog.Error("dispatch process: claim failed", "job_id", jobID, "err", err)
		unavailable(w, "claim failed")
		return
	}
	if !claimed {
		var ok bool
		if job, ok = h.lostClaim(ctx, w, jobID); !ok {
			return // answered
		}
	}

	// The delivery and its bookkeeping run on a context the router cannot
	// cancel: once claimed, the attempt's outcome is always recorded, even if
	// the router hangs up (a router restarting mid-call). The delivery itself
	// is still bounded by the job's timeout (see deliver).
	if !h.deliverAndRecord(context.WithoutCancel(ctx), job) {
		// The job is still PROCESSING with its outcome unwritten. Keep the
		// message: its next copy takes the claim over once the lease runs out
		// (at-least-once), instead of the job staying PROCESSING for good.
		unavailable(w, "status update failed")
		return
	}
	writeJSON(w, http.StatusOK, processResponse{Ack: true})
}

// awaitNotBefore answers a job reached before its not_before: ack=false with
// the time to come back — not_before itself, or notBeforeRecheck from now when
// that is sooner (see notBeforeRecheck). The job stays QUEUED; its updated_at
// is stamped so stale recovery does not take the held message for lost.
func (h *Handler) awaitNotBefore(ctx context.Context, w http.ResponseWriter, job *dispatchjob.DispatchJob, now time.Time) {
	if err := h.repo.AwaitNotBefore(ctx, job.ID, job.CreatedAt); err != nil {
		slog.Error("dispatch process: not-before stamp failed", "job_id", job.ID, "err", err)
		unavailable(w, "not-before stamp failed")
		return
	}
	at := *job.NotBefore
	if recheck := now.Add(notBeforeRecheck); recheck.Before(at) {
		at = recheck
	}
	slog.Debug("dispatch deferred: not before", "job_id", job.ID, "not_before", job.NotBefore, "come_back", at)
	deferredUntil(w, now, at, "not before "+job.NotBefore.UTC().Format(envelopeTime))
}

// lostClaim decides what a call that lost the claim does. Before, it always
// acked the message away without delivering — right while another attempt is
// live, and the job's end when that attempt died with its process (a platform
// killed mid-delivery: the webhook may have gone out, the outcome was never
// written, and the router's retry was acked away). So:
//   - the job is PROCESSING and its claim is inside claimLease: an attempt may
//     be live. Answer {"ack": false, "delaySeconds": <rest of the lease>} so
//     the router keeps the message (and the group behind it) and asks again;
//   - the lease has run out: that attempt is dead. Take the claim over and
//     deliver again (at-least-once);
//   - anything else (finished, or back to PENDING for a retry the poller
//     owns): ack without delivering, as before.
//
// Returns the job to deliver and true when this call took the claim over;
// otherwise it has written the answer and returns false.
func (h *Handler) lostClaim(ctx context.Context, w http.ResponseWriter, jobID string) (*dispatchjob.DispatchJob, bool) {
	job, err := h.repo.FindByID(ctx, jobID)
	if err != nil {
		slog.Error("dispatch process: reload after a lost claim failed", "job_id", jobID, "err", err)
		unavailable(w, "load failed")
		return nil, false
	}
	if job == nil {
		writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "job not found"})
		return nil, false
	}
	if job.Status != common.DispatchProcessing {
		slog.Info("dispatch process: already claimed, skipping duplicate delivery",
			"job_id", jobID, "status", job.Status)
		writeJSON(w, http.StatusOK, processResponse{Ack: true, Message: "already claimed"})
		return nil, false
	}
	lease := claimLease(job)
	claimedAt := job.UpdatedAt
	if job.LastAttemptAt != nil {
		claimedAt = *job.LastAttemptAt
	}
	now := time.Now()
	if leaseEnds := claimedAt.Add(lease); now.Before(leaseEnds) {
		wait := leaseEnds.Sub(now)
		secs := uint32((wait + time.Second - 1) / time.Second)
		if secs == 0 {
			secs = 1
		}
		slog.Info("dispatch process: delivery in progress elsewhere; asking the router to retry",
			"job_id", jobID, "delay_seconds", secs)
		deferred(w, secs, "delivery in progress")
		return nil, false
	}
	took, err := h.repo.ReclaimStaleDelivery(ctx, jobID, job.CreatedAt, now.Add(-lease))
	if err != nil {
		slog.Error("dispatch process: reclaim failed", "job_id", jobID, "err", err)
		unavailable(w, "claim failed")
		return nil, false
	}
	if !took {
		// Another copy took it over first; it is now the live attempt.
		deferred(w, 1, "delivery in progress")
		return nil, false
	}
	slog.Warn("dispatch process: the previous attempt never finished; delivering again",
		"job_id", jobID, "claimed_at", claimedAt, "lease", lease)
	return job, true
}

// deliverAndRecord delivers a job this call has claimed, records the attempt
// and advances the job. Returns false when the outcome could not be written
// (the job is still PROCESSING).
func (h *Handler) deliverAndRecord(ctx context.Context, job *dispatchjob.DispatchJob) bool {
	jobID := job.ID
	attemptNumber := job.AttemptCount + 1
	attempt := dispatchjob.NewAttempt(attemptNumber)
	res := h.deliver(ctx, job)

	// Record the attempt (best-effort; a recording failure must not change
	// the delivery decision). The response body is kept on failure too —
	// it is the subscriber's stated reason — and the request summary says
	// what was sent to earn it.
	attempt.Request = res.request
	if res.success {
		attempt.CompleteSuccess(res.statusCode, res.body)
	} else {
		attempt.CompleteFailure(res.errMessage, res.errType, res.statusCodePtr(), res.body)
	}
	if err := h.repo.RecordAttempt(ctx, jobID, attempt); err != nil {
		slog.Warn("dispatch process: record attempt failed", "job_id", jobID, "err", err)
	}

	return h.advance(ctx, job, attemptNumber, res, attempt)
}

// advance transitions the job row based on the delivery result. Returns
// false when the status write failed, leaving the job PROCESSING.
func (h *Handler) advance(ctx context.Context, job *dispatchjob.DispatchJob, attemptNumber int32, res deliveryResult, attempt *dispatchjob.Attempt) bool {
	jobID := job.ID
	dur := int64(0)
	if attempt.DurationMillis != nil {
		dur = *attempt.DurationMillis
	}

	switch {
	case res.success:
		if err := h.repo.MarkCompleted(ctx, jobID, job.CreatedAt, dur); err != nil {
			slog.Warn("dispatch process: mark completed failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Debug("dispatch delivered", "job_id", jobID, "status", res.statusCode, "attempt", attemptNumber)

	case res.deferral:
		// Cooperative back-pressure (ack=false or HTTP 429): retry later
		// WITHOUT consuming the retry budget.
		if err := h.repo.Reschedule(ctx, jobID, job.CreatedAt, time.Now().Add(res.retryAfter)); err != nil {
			slog.Warn("dispatch process: reschedule failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Info("dispatch deferred", "job_id", jobID, "retry_after", res.retryAfter, "reason", res.errMessage)

	case res.hasStatus && (res.statusCode == http.StatusUnauthorized || res.statusCode == http.StatusForbidden):
		// The subscriber refused the credentials. Nothing about a retry
		// changes what was sent — same secret, same token — so retrying only
		// spends the budget over a few minutes and reports the same 401 at
		// the end of it (owner, 2026-09-22: fail fast). Terminal on the first
		// attempt; the operator fixes the credential and requeues.
		errMsg := res.errMessage
		if err := h.repo.MarkFailed(ctx, jobID, job.CreatedAt, &errMsg, dur); err != nil {
			slog.Warn("dispatch process: mark failed failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Warn("dispatch failed (subscriber refused credentials; not retried)",
			"job_id", jobID, "status", res.statusCode, "attempt", attemptNumber, "err", errMsg)

	case res.rejected:
		// The subscriber answered that retrying cannot help (a function's
		// fn.Reject): terminal on this attempt, like refused credentials.
		errMsg := res.errMessage
		if err := h.repo.MarkFailed(ctx, jobID, job.CreatedAt, &errMsg, dur); err != nil {
			slog.Warn("dispatch process: mark failed failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Warn("dispatch failed (subscriber rejected; not retried)",
			"job_id", jobID, "status", res.statusCode, "attempt", attemptNumber)

	case int(attemptNumber) >= int(job.MaxRetries):
		// Out of retries → terminal failure.
		errMsg := res.errMessage
		if err := h.repo.MarkFailed(ctx, jobID, job.CreatedAt, &errMsg, dur); err != nil {
			slog.Warn("dispatch process: mark failed failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Warn("dispatch failed (retries exhausted)", "job_id", jobID, "attempts", attemptNumber, "max", job.MaxRetries, "err", errMsg)

	default:
		// Retryable failure → schedule a backoff (by the job's retry
		// strategy) and let the poller pick it up.
		errMsg := res.errMessage
		backoff := retryDelay(job.RetryStrategy, attemptNumber)
		if err := h.repo.ScheduleRetry(ctx, jobID, job.CreatedAt, time.Now().Add(backoff), &errMsg); err != nil {
			slog.Warn("dispatch process: schedule retry failed", "job_id", jobID, "err", err)
			return false
		}
		slog.Info("dispatch retry scheduled", "job_id", jobID, "attempt", attemptNumber, "strategy", job.RetryStrategy, "backoff", backoff, "err", errMsg)
	}
	return true
}

// retryDelay is the wait before the retry after attempt attemptNumber, by the
// job's retry strategy: "immediate" — none (the next poll takes it);
// "fixed" — fixedRetryDelay every time; "exponential" (and an unset strategy,
// the column's default) — the retryBackoff curve.
func retryDelay(strategy dispatchjob.RetryStrategy, attemptNumber int32) time.Duration {
	switch strategy {
	case dispatchjob.RetryImmediate:
		return 0
	case dispatchjob.RetryFixed:
		return fixedRetryDelay
	case dispatchjob.RetryExponentialBackoff:
		return backoffFor(attemptNumber)
	default: // unset: the column's default, exponential
		return backoffFor(attemptNumber)
	}
}

func backoffFor(attemptNumber int32) time.Duration {
	i := max(int(attemptNumber)-1, 0)
	if i >= len(retryBackoff) {
		i = len(retryBackoff) - 1
	}
	return retryBackoff[i]
}

// deliveryResult is the outcome of one webhook POST.
type deliveryResult struct {
	success    bool
	deferral   bool // cooperative back-pressure (retry, no budget spend)
	rejected   bool // the target's terminal "do not retry" (422 + FlowCatalyst-Outcome: reject)
	retryAfter time.Duration
	statusCode int
	hasStatus  bool
	body       *string
	errMessage string
	errType    dispatchjob.ErrorType
	// request is what was sent — recorded on the attempt.
	request *dispatchjob.RequestSummary
}

func (r deliveryResult) statusCodePtr() *int {
	if !r.hasStatus {
		return nil
	}
	s := r.statusCode
	return &s
}

// deliver POSTs the real event to the subscriber's target_url and classifies
// the response.
func (h *Handler) deliver(ctx context.Context, job *dispatchjob.DispatchJob) deliveryResult {
	timeout := defaultTimeout
	if job.TimeoutSeconds > 0 {
		timeout = time.Duration(job.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, summary, err := h.buildRequest(ctx, job)
	if err != nil {
		return deliveryResult{errMessage: "build request: " + err.Error(), errType: dispatchjob.ErrorConnection}
	}
	res := h.exchange(h.clientFor(ctx, job), req)
	res.request = summary
	if summary.UnsignedReason != "" && !res.success && !res.deferral {
		res.errMessage += " (delivered unsigned: " + summary.UnsignedReason + ")"
	}
	return res
}

// buildRequest assembles the subscriber request for job — body, headers,
// credentials — and the RequestSummary describing it. Shared by deliver
// (which sends it) and Plan (which only shows it), so what the operator is
// shown is byte-for-byte what a delivery would send at that instant.
func (h *Handler) buildRequest(ctx context.Context, job *dispatchjob.DispatchJob) (*http.Request, *dispatchjob.RequestSummary, error) {
	// Resolve the client's identifier once, up front: buildPayload needs it
	// for the envelope's clientCode and the header decision below needs it
	// too. A platform-scoped job (no client_id) skips the lookup entirely.
	var clientCode string
	if job.ClientID != nil && *job.ClientID != "" && h.clientCode != nil {
		if code, ok := h.clientCode(ctx, string(*job.ClientID)); ok {
			clientCode = code
		}
	}

	body := buildPayloadWithGroup(job, clientCode, h.envelopeGroup(ctx, job))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, job.TargetURL, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", bodyContentType(job))
	req.Header.Set("X-Dispatch-Job-Id", job.ID)
	req.Header.Set("X-Event-Type", job.Code)
	// Sent for dataOnly deliveries too (R2) — the raw-payload body has no
	// envelope for clientCode to ride in, which is the whole reason this is
	// a header rather than just an envelope field. Computed from job.ClientID
	// directly (not clientCode alone) so a resolved code never gets attached
	// to a header build that skips the id half.
	if v, ok := clientHeaderValue(job, clientCode); ok {
		req.Header.Set(clientHeader, v)
	}

	summary := &dispatchjob.RequestSummary{Target: job.TargetURL}
	// UnsignedReason is why this delivery carries no credentials, when it
	// carries none. A bare delivery to a subscriber that verifies signatures
	// is a guaranteed rejection, and the rejection alone ("HTTP 401") says
	// nothing about the cause — so the cause is logged, recorded on the
	// attempt, and on a failure appended to its error message (owner,
	// 2026-09-22: never skip silently).
	if h.creds == nil {
		summary.UnsignedReason = "no credential resolver configured"
	} else {
		creds, serr := h.creds(ctx, job)
		switch {
		case serr != nil:
			summary.UnsignedReason = "credential lookup failed: " + serr.Error()
			slog.Warn("dispatch process: delivery-creds lookup failed; delivering unsigned",
				"job_id", job.ID, "err", serr)
		case creds.Empty():
			summary.UnsignedReason = creds.Reason
			if summary.UnsignedReason == "" {
				summary.UnsignedReason = "no credentials resolved"
			}
			slog.Warn("dispatch process: delivering unsigned",
				"job_id", job.ID, "subscription_id", deref(job.SubscriptionID), "reason", summary.UnsignedReason)
		default:
			// Same header set as router-mediated webhooks: bearer (static
			// convenience credential) + HMAC signature (the real boundary).
			summary.SignedBy = creds.SignedBy
			if creds.BearerToken != "" {
				req.Header.Set("Authorization", "Bearer "+creds.BearerToken)
				summary.Bearer = true
			}
			if creds.SigningSecret != "" {
				ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				req.Header.Set(signatureHeader, signBody(creds.SigningSecret, ts, body))
				req.Header.Set(timestampHeader, ts)
				summary.Signature = true
				summary.Timestamp = ts
			}
		}
	}
	for name := range req.Header {
		summary.Headers = append(summary.Headers, headerDisplayName(name))
	}
	sort.Strings(summary.Headers)
	return req, summary, nil
}

// headerDisplayName undoes http.Header's canonicalisation for the headers
// this package documents with their own casing (X-FlowCatalyst-*): the wire
// is case-insensitive, but an operator reading a plan against the SDK docs
// should see the spelling the docs use.
func headerDisplayName(canonical string) string {
	for _, documented := range []string{signatureHeader, timestampHeader, clientHeader} {
		if http.CanonicalHeaderKey(documented) == canonical {
			return documented
		}
	}
	return canonical
}

// signBody is the delivery signature: HMAC-SHA256 over timestamp+body, hex
// — the byte format every SDK's WebhookValidator verifies.
func signBody(secret, ts string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// DeliveryPlan is what a delivery of the job would send RIGHT NOW, without
// sending it (owner, 2026-09-22: "an action on the dispatch job where I can
// see which service account it will use and generate a signature for the
// payload"). Every header is real and the signature is a real, verifiable
// signature over Body with Timestamp — an operator can hand the three to
// the subscriber's verify command (php artisan flowcatalyst:verify-signature)
// and see whether THEIR secret accepts it. The Authorization value is
// masked: the bearer is a static credential, not a diagnostic.
type DeliveryPlan struct {
	Request *dispatchjob.RequestSummary `json:"request"`
	Headers map[string]string           `json:"headers"`
	Body    string                      `json:"body"`
}

// Plan builds the delivery for job and returns it instead of sending it.
func (h *Handler) Plan(ctx context.Context, job *dispatchjob.DispatchJob) (*DeliveryPlan, error) {
	req, summary, err := h.buildRequest(ctx, job)
	if err != nil {
		return nil, err
	}
	headers := make(map[string]string, len(req.Header))
	for name, values := range req.Header {
		if len(values) == 0 {
			continue
		}
		if name == "Authorization" {
			headers[name] = "Bearer ••••••"
			continue
		}
		headers[headerDisplayName(name)] = values[0]
	}
	raw, _ := io.ReadAll(req.Body)
	return &DeliveryPlan{Request: summary, Headers: headers, Body: string(raw)}, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// exchange sends the built request and classifies the response.
func (h *Handler) exchange(client *http.Client, req *http.Request) deliveryResult {
	resp, err := client.Do(req)
	if err != nil {
		msg, et := classifyTransportErr(err)
		return deliveryResult{errMessage: msg, errType: et}
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	bodyStr := string(raw)
	status := resp.StatusCode

	switch {
	case status >= 200 && status < 300:
		// Honour a cooperative deferral: {"ack": false} means "accepted but
		// not done — try again later".
		if deferDelay, deferred := parseDeferral(raw); deferred {
			return deliveryResult{
				deferral:   true,
				retryAfter: deferDelay,
				statusCode: status,
				hasStatus:  true,
				body:       &bodyStr,
				errMessage: "subscriber deferred (ack=false)",
			}
		}
		return deliveryResult{success: true, statusCode: status, hasStatus: true, body: &bodyStr}

	case status == http.StatusTooManyRequests: // 429 → back-pressure, not a failure
		return deliveryResult{
			deferral:   true,
			retryAfter: retryAfterOrDefault(resp),
			statusCode: status,
			hasStatus:  true,
			body:       &bodyStr,
			errMessage: "rate limited (429)",
		}

	case common.IsDeliveryRejected(status, resp.Header.Get(common.DeliveryOutcomeHeader)):
		return deliveryResult{
			rejected:   true,
			statusCode: status,
			hasStatus:  true,
			body:       &bodyStr,
			errMessage: "subscriber rejected the delivery (422, FlowCatalyst-Outcome: reject): not retried",
			errType:    dispatchjob.ErrorHTTPError,
		}

	default: // 3xx / 4xx / 5xx → delivery failure
		return deliveryResult{
			statusCode: status,
			hasStatus:  true,
			body:       &bodyStr,
			errMessage: "HTTP " + resp.Status,
			errType:    dispatchjob.ErrorHTTPError,
		}
	}
}

// bodyContentType is the request's Content-Type: the envelope is always JSON;
// a data-only body is the raw payload, so it goes with the content type the
// job was made with (application/json when it has none).
func bodyContentType(job *dispatchjob.DispatchJob) string {
	if job.DataOnly && strings.TrimSpace(job.PayloadContentType) != "" {
		return job.PayloadContentType
	}
	return "application/json"
}

// envelopeTime is how the envelope writes the job's creation time: UTC, RFC
// 3339, milliseconds.
const envelopeTime = "2006-01-02T15:04:05.000Z"

// envelopeGroup is the messageGroup a subscriber is told: the event's own
// group, not the per-subscription key the job is ordered by (see
// dispatchjob.SubscriptionMessageGroup). When the stored key is the digest of
// an over-long event group, the event's group is read from the event; if that
// fails the key itself is sent rather than nothing.
func (h *Handler) envelopeGroup(ctx context.Context, job *dispatchjob.DispatchJob) *string {
	group, ok := dispatchjob.EventMessageGroup(job)
	if ok {
		return group
	}
	if job.EventID != nil && h.repo != nil {
		g, err := h.repo.EventMessageGroup(ctx, *job.EventID, job.CreatedAt)
		if err == nil && g != nil {
			return g
		}
		if err != nil {
			slog.Warn("dispatch process: event message group lookup failed; sending the job's group key",
				"job_id", job.ID, "err", err)
		}
	}
	return job.MessageGroup
}

// buildPayload renders the request body with the envelope's messageGroup
// taken from the job alone (EventMessageGroup's prefix removal; the digest
// form is sent as stored). See buildPayloadWithGroup.
func buildPayload(job *dispatchjob.DispatchJob, clientCode string) []byte {
	group, _ := dispatchjob.EventMessageGroup(job)
	if group == nil {
		group = job.MessageGroup
	}
	return buildPayloadWithGroup(job, clientCode, group)
}

// buildPayloadWithGroup renders the request body: raw payload in data-only mode,
// otherwise a CloudEvents-style envelope. clientCode is the job's client's
// identifier slug ("" when the job has no client_id, or the client didn't
// resolve) — embedded as clientCode alongside clientId, per
// docs/spec/webhook-client-code.md R1. Ignored in data-only mode: that body
// is the raw payload, byte-for-byte, with no envelope to carry it — R2 covers
// that case with the X-FlowCatalyst-Client header instead.
func buildPayloadWithGroup(job *dispatchjob.DispatchJob, clientCode string, messageGroup *string) []byte {
	if job.DataOnly {
		if job.Payload != nil {
			return []byte(*job.Payload)
		}
		return []byte("{}")
	}

	env := map[string]any{
		"id":            job.ID,
		"type":          job.Code,
		"attemptNumber": job.AttemptCount + 1,
	}
	if !job.CreatedAt.IsZero() {
		env["time"] = job.CreatedAt.UTC().Format(envelopeTime)
	}
	if job.PayloadContentType != "" {
		env["dataContentType"] = job.PayloadContentType
	}
	if job.Source != nil {
		env["source"] = *job.Source
	}
	if job.Subject != nil {
		env["subject"] = *job.Subject
	}
	if job.CorrelationID != nil {
		env["correlationId"] = *job.CorrelationID
	}
	if messageGroup != nil {
		env["messageGroup"] = *messageGroup
	}
	if job.ClientID != nil {
		env["clientId"] = *job.ClientID
		// Omitted — the key absent, not null — when the client cannot be
		// resolved. clientId keeps its current meaning and position.
		if clientCode != "" {
			env["clientCode"] = clientCode
		}
	}
	if job.Payload != nil {
		// Embed as JSON when it parses; otherwise pass the raw string through
		// so a non-JSON payload isn't silently dropped.
		var parsed json.RawMessage
		if json.Unmarshal([]byte(*job.Payload), &parsed) == nil {
			env["data"] = parsed
		} else {
			env["data"] = *job.Payload
		}
	}
	out, err := json.Marshal(env)
	if err != nil {
		return []byte("{}")
	}
	return out
}

// clientHeaderValue composes the X-FlowCatalyst-Client header value, or
// ok=false to omit the header entirely. Never a half pair (R2): a
// platform-scoped job (no client_id) or a client that did not resolve
// (clientCode == "") omits the header, exactly like the envelope's
// clientCode field.
func clientHeaderValue(job *dispatchjob.DispatchJob, clientCode string) (string, bool) {
	if job.ClientID == nil || *job.ClientID == "" || clientCode == "" {
		return "", false
	}
	return string(*job.ClientID) + ":" + clientCode, true
}

// defaultDeferral is how long a subscriber's deferral (ack=false, 429) waits
// when it does not say.
const defaultDeferral = 30 * time.Second

// parseDeferral reports a 2xx body of the form {"ack": false} as a
// cooperative deferral, and how long to wait: until notBefore (RFC 3339) when
// the body names one and it parses, else delaySeconds, else defaultDeferral.
// A notBefore already past waits nothing.
func parseDeferral(body []byte) (time.Duration, bool) {
	return parseDeferralAt(body, time.Now())
}

func parseDeferralAt(body []byte, now time.Time) (time.Duration, bool) {
	if len(body) == 0 {
		return 0, false
	}
	var r struct {
		Ack          *bool   `json:"ack"`
		DelaySeconds *uint32 `json:"delaySeconds"`
		NotBefore    *string `json:"notBefore"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Ack == nil || *r.Ack {
		return 0, false
	}
	if r.NotBefore != nil {
		if t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(*r.NotBefore)); err == nil {
			return max(t.Sub(now), 0), true
		}
	}
	d := defaultDeferral
	if r.DelaySeconds != nil {
		d = time.Duration(*r.DelaySeconds) * time.Second
	}
	return d, true
}

func retryAfterOrDefault(resp *http.Response) time.Duration {
	if d, ok := common.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now()); ok {
		return d
	}
	return defaultDeferral
}

func classifyTransportErr(err error) (string, dispatchjob.ErrorType) {
	var netErr interface{ Timeout() bool }
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "Connection timeout", dispatchjob.ErrorTimeout
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "Connection timeout", dispatchjob.ErrorTimeout
	}
	return "Connection error: " + err.Error(), dispatchjob.ErrorConnection
}

// guardedTransport is http.DefaultTransport with the dial refusing the
// destinations policy forbids: a subscription's target URL is customer
// input, and this is the hop that POSTs to it from inside the cluster.
func guardedTransport(policy *netguard.Policy) *http.Transport {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.DialContext = policy.DialContext(&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second})
	return t
}
