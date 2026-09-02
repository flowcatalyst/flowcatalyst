package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// This file implements the router half of T3/A-01: reporting the ids
// ackBuffered ACKs off the broker (the untried siblings buffered behind a
// BLOCK_ON_ERROR head that just failed terminally) to the platform's
// settled-message hook, POST /api/dispatch/settled
// (internal/platform/dispatchjob/settled — read its package doc for the
// full contract before changing anything here).
//
// The hook is the fast path; internal/platform/dispatchjob.RunReaper is the
// crash backstop for a router that dies between the ACK and this call — so
// this call is deliberately best-effort: fire-and-forget, its own bounded
// timeout, never blocking ackBuffered or holding a pool worker/semaphore
// slot. A router with no reporter wired (a standalone router with no
// platform, or before the wiring below has landed) behaves exactly as it
// does today — silently; see SetSettledReporter's doc comment.
//
// Deferred wiring (another lane's files, not this one): construct one
// HTTPSettledReporter per Server from the platform base URL already used
// elsewhere for router→platform calls, then call SetSettledReporter on
// every Pool the Manager creates:
//
//	reporter := router.NewHTTPSettledReporter(router.SettledReporterConfig{Endpoint: platformBaseURL})
//	pool.SetSettledReporter(reporter) // once per Pool, e.g. inside Manager.NewPool's caller
//
// A nil reporter (never calling SetSettledReporter) is the correct default
// until that wiring lands.

// SettledJob pairs an ACKed dispatch-job id with the scheduler-signed HMAC
// token the router already holds for it (common.Message.AuthToken — the
// same token MessageGroupDispatcher signs per job id and the router already
// forwards as "Authorization: Bearer" when delivering to
// /api/dispatch/process; see mediator.go). settled.Handler verifies each
// pair independently, exactly like /api/dispatch/process's own verifier, so
// the platform never has to trust the router with a separate credential.
type SettledJob struct {
	ID    string
	Token string
}

// SettledReport is one ackBuffered event: the ids ACKed off the broker
// because a BLOCK_ON_ERROR group's head failed terminally, plus context for
// logging on either side. Only Reason and Jobs cross the wire (see
// settledRequest in internal/platform/dispatchjob/settled/settled.go) —
// PoolCode and Group are carried for the reporter implementation's own
// diagnostics and are not part of the request body.
type SettledReport struct {
	PoolCode string
	Group    string
	Reason   string
	Jobs     []SettledJob
}

// SettledReporter reports a settled group to the platform. Satisfied by
// *HTTPSettledReporter in production; a fake in tests.
type SettledReporter interface {
	ReportSettled(ctx context.Context, report SettledReport) error
}

// settledWireJob and settledWireRequest mirror
// internal/platform/dispatchjob/settled/settled.go's settledJob/
// settledRequest field-for-field. Kept as unexported wire types here (rather
// than shared) for the same reason settled.go itself gives for not sharing
// its Verifier interface: importing across the router/platform boundary
// would either cycle or reach into an internal package the router must not
// depend on. Field names/tags must be kept in sync by hand.
type settledWireJob struct {
	ID    string `json:"id"`
	Token string `json:"token"`
}

type settledWireRequest struct {
	Reason string           `json:"reason"`
	Jobs   []settledWireJob `json:"jobs"`
}

// settledPath is appended to SettledReporterConfig.Endpoint.
const settledPath = "/api/dispatch/settled"

// settledChunkSize bounds how many jobs go in one request. The endpoint caps
// a request body at 1 MiB (maxBodyBytes in settled.go) and a batch at 10,000
// items (maxJobsPerRequest) — but 10,000 items of
// {"id":"<~13 char TSID>","token":"<64-hex HMAC>"} is already ~93% of the
// body cap BEFORE the reason string, so the advertised ceiling 400s in
// practice. 1,000 items is comfortably inside both limits (~98 KiB), so a
// group larger than that is reported over several sequential requests
// instead of one. The endpoint is idempotent (SettleAcked only touches rows
// still QUEUED/PROCESSING), so overlapping or duplicate chunks are
// harmless — no coordination needed between them.
const settledChunkSize = 1000

// defaultSettledTimeout bounds a single chunk's HTTP call when
// SettledReporterConfig.Timeout is unset.
const defaultSettledTimeout = 5 * time.Second

// SettledReporterConfig configures HTTPSettledReporter.
type SettledReporterConfig struct {
	// Endpoint is the platform's base URL (no trailing path); settledPath is
	// appended. Required.
	Endpoint string
	// Timeout bounds a single chunk's HTTP call. Defaults to
	// defaultSettledTimeout when zero.
	Timeout time.Duration
}

// HTTPSettledReporter is the production SettledReporter: POSTs
// /api/dispatch/settled on the platform, chunked at settledChunkSize. Owns a
// small dedicated http.Client rather than borrowing the mediator's
// HostPoolRegistry — this is one known, trusted, low-volume destination
// (unlike arbitrary mediation targets), so the per-host slot machinery
// would add nothing but complexity.
type HTTPSettledReporter struct {
	url    string
	client *http.Client
}

// NewHTTPSettledReporter builds the HTTP implementation. Construction is
// cheap and holds no goroutines, so callers may build one per Server and
// share it across every Pool (see this file's header comment for the
// one-line wiring).
func NewHTTPSettledReporter(cfg SettledReporterConfig) *HTTPSettledReporter {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultSettledTimeout
	}
	return &HTTPSettledReporter{
		url:    strings.TrimRight(cfg.Endpoint, "/") + settledPath,
		client: &http.Client{Timeout: timeout},
	}
}

// ReportSettled posts report.Jobs to the platform in settledChunkSize
// batches, sequentially. Returns a joined error naming every chunk that
// failed (still attempting the rest — a slow/down platform on one chunk
// must not sink the others), or nil once every chunk succeeded. Callers
// that treat this as best-effort (ackBuffered does, via the goroutine in
// pool.go's reportSettled) only need to know whether to log; they do not
// need per-chunk detail.
func (r *HTTPSettledReporter) ReportSettled(ctx context.Context, report SettledReport) error {
	if len(report.Jobs) == 0 {
		return nil
	}
	var errs []error
	for start := 0; start < len(report.Jobs); start += settledChunkSize {
		end := min(start+settledChunkSize, len(report.Jobs))
		if err := r.postChunk(ctx, report.Reason, report.Jobs[start:end]); err != nil {
			errs = append(errs, fmt.Errorf("settled report chunk [%d:%d]: %w", start, end, err))
		}
	}
	return errors.Join(errs...)
}

func (r *HTTPSettledReporter) postChunk(ctx context.Context, reason string, jobs []SettledJob) error {
	wireJobs := make([]settledWireJob, len(jobs))
	for i, j := range jobs {
		wireJobs[i] = settledWireJob{ID: j.ID, Token: j.Token}
	}
	body, err := json.Marshal(settledWireRequest{Reason: reason, Jobs: wireJobs})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("settled hook returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	return nil
}

// dispatchJobFromMessage extracts the (id, token) pair the settled hook
// needs from a buffered sibling, or ok=false when the message carries no
// scheduler-signed AuthToken — the same signal mediator.go uses to decide
// whether to forward an Authorization header when delivering to
// /api/dispatch/process (see msg.AuthToken != nil there). A message with no
// AuthToken did not come from the platform scheduler (an operator
// submission, or a producer pointing straight at a subscriber — see
// docs/router-architecture.md §4, "The router's own mode handling governs
// operator-submitted messages... and any producer pointing a message at a
// subscriber directly"), so there is no dispatch-job row for the hook to
// verify or mark; skip it rather than sending an unverifiable id.
func dispatchJobFromMessage(m common.Message) (SettledJob, bool) {
	if m.AuthToken == nil || *m.AuthToken == "" {
		return SettledJob{}, false
	}
	return SettledJob{ID: m.ID, Token: *m.AuthToken}, true
}
