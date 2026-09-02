// Package settled implements POST /api/dispatch/settled — the
// router→platform "settled message" hook that closes the fast-move half of
// T3/A-01 (BLOCK_ON_ERROR group recovery; the reaper in
// dispatchjob.RunReaper is the backstop for a router crash between the ACK
// and this call — see that package's doc comment).
//
// Flow: internal/router/pool.go's ackBuffered ACKs a message group's
// untried buffered siblings the instant its head fails terminally under
// BLOCK_ON_ERROR — its own doc comment names the gap this closes: "nothing
// marks those job rows for review, so they sit at QUEUED/PROCESSING...a
// settled-message hook carrying the reason would make the ids recoverable."
// The router already holds, for each message it's about to ACK, that
// message's own scheduler-signed bearer token (common.Message.AuthToken —
// the same token MessageGroupDispatcher.buildMessage signs per job id and
// the router already forwards as Authorization: Bearer to
// /api/dispatch/process), so it POSTs the ids + those tokens + a reason
// here rather than the platform inventing a new credential for the router
// to carry. Each id/token pair is verified independently — same mechanism
// as processing.Verifier, just applied per item in a batch instead of to a
// single messageId.
package settled

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// maxJobsPerRequest bounds one call's batch size. ackBuffered's backlog is
// bounded by a single message group's buffer in one pool, so this is a
// generous ceiling against a pathological request, not a real-traffic
// limit.
const maxJobsPerRequest = 10000

// maxBodyBytes bounds the request body the handler will decode.
const maxBodyBytes = 1 << 20 // 1 MiB

// defaultReason is recorded when the caller sends an empty reason string.
const defaultReason = "settled: router ACKed as an untried buffered sibling behind a failed BLOCK_ON_ERROR head"

// Verifier checks the HMAC bearer token the scheduler signed for a job id.
// Satisfied by *scheduler.DispatchAuthService. dispatchjob (and this
// sub-package) cannot import internal/platform/scheduler — scheduler
// already imports dispatchjob (poller.go), so the dependency would cycle —
// so, like internal/platform/dispatchjob/processing.Verifier, this package
// declares its own structurally-identical interface instead of sharing one.
type Verifier interface {
	Verify(jobID, token string) bool
}

// Handler serves the settled-message hook.
type Handler struct {
	repo     *dispatchjob.Repository
	verifier Verifier
}

// New wires the handler. verifier may be nil (dev/no-auth), matching
// processing.New's convention — but that is a misconfiguration in any
// deployment where the scheduler signs tokens, so callers should pass one.
func New(repo *dispatchjob.Repository, verifier Verifier) *Handler {
	return &Handler{repo: repo, verifier: verifier}
}

// Mount attaches POST /api/dispatch/settled to the given (unauthenticated)
// chi router. The handler self-verifies each job's scheduler-signed HMAC
// bearer, so — like processing.Handler.Mount — it must live OUTSIDE the
// platform JWT middleware: the router carries no platform JWT.
func (h *Handler) Mount(r chi.Router) {
	r.Post("/api/dispatch/settled", h.serve)
}

// settledJob pairs an ACKed job id with the scheduler-signed token the
// router already holds for it.
type settledJob struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

// settledRequest is the wire body: reason is free text recorded on every
// row this call settles (surfaced to operators via DispatchJob.LastError).
type settledRequest struct {
	Reason string       `json:"reason"`
	Jobs   []settledJob `json:"jobs"`
}

// settledResponse reports how many of the submitted jobs were actually
// reset (a submitted id can be dropped for a bad/missing token, for not
// existing, or for already being past QUEUED/PROCESSING — e.g. a duplicate
// call, or a race with the reaper) and echoes which ones, so a caller doing
// its own bookkeeping/logging doesn't have to guess.
type settledResponse struct {
	Settled int      `json:"settled"`
	IDs     []string `json:"ids,omitempty"`
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var req settledRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, settledResponse{})
		return
	}
	if len(req.Jobs) == 0 {
		writeJSON(w, http.StatusOK, settledResponse{Settled: 0})
		return
	}
	if len(req.Jobs) > maxJobsPerRequest {
		writeJSON(w, http.StatusBadRequest, settledResponse{})
		return
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = defaultReason
	}

	// Verify each id/token pair independently. A bad token for one job must
	// not block settling the others in the same batch — the router built
	// this batch from one group's buffered messages, and there's no reason a
	// single corrupt entry should sink the rest.
	ids := make([]string, 0, len(req.Jobs))
	for _, j := range req.Jobs {
		id := strings.TrimSpace(j.ID)
		if id == "" || strings.TrimSpace(j.Token) == "" {
			continue
		}
		if h.verifier != nil && !h.verifier.Verify(id, j.Token) {
			slog.Warn("dispatch settled: bad auth token", "job_id", id)
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		// Nothing verified — an empty/garbage batch, or every token failed.
		// Either way there is nothing to settle here; the reaper remains the
		// backstop for anything genuinely stranded.
		writeJSON(w, http.StatusUnauthorized, settledResponse{Settled: 0})
		return
	}

	settledIDs, err := h.repo.SettleAcked(ctx, ids, reason)
	if err != nil {
		slog.Error("dispatch settled: settle failed", "err", err, "submitted", len(ids))
		writeJSON(w, http.StatusInternalServerError, settledResponse{})
		return
	}
	if len(settledIDs) > 0 {
		slog.Info("dispatch settled: siblings marked PENDING", "count", len(settledIDs), "reason", reason)
	}
	writeJSON(w, http.StatusOK, settledResponse{Settled: len(settledIDs), IDs: settledIDs})
}

func writeJSON(w http.ResponseWriter, code int, body settledResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}
