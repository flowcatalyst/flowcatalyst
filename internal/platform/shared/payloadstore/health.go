package payloadstore

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"
)

// HealthStatus is the payload store's state as health reports it
// (docs/payload-offload-design.md, sections 7 and 13). A closed set.
type HealthStatus string

const (
	// HealthNotConfigured: no payload store is configured, so every payload
	// is kept in the database. A healthy state.
	HealthNotConfigured HealthStatus = "NOT_CONFIGURED"
	// HealthUp: a store is configured and no write or read of it failed
	// within the window.
	HealthUp HealthStatus = "UP"
	// HealthDegraded: within the window a write of the store failed (its
	// payloads were kept in the database, or refused when too large for it)
	// or a read of it failed. A warning, never a reason to stop taking
	// traffic: ingest keeps working by design (section 7).
	HealthDegraded HealthStatus = "DEGRADED"
)

// DegradedWindow is how long a failed write or read of the store keeps the
// store DEGRADED.
const DegradedWindow = 5 * time.Minute

// HealthOperation is the store operation whose outcome health last recorded.
type HealthOperation string

const (
	HealthOpPut     HealthOperation = "put"     // a segment write
	HealthOpGet     HealthOperation = "get"     // a range read
	HealthOpStartup HealthOperation = "startup" // the one check made when the service starts
)

// HealthReport is the payload store's health, as GET /health/payload-store
// and the metrics listener's /ready show it. It is derived from the outcomes
// of the operations the service has made; reading it never calls the store.
type HealthReport struct {
	Status HealthStatus `json:"status"`
	// Configured reports whether a payload store is configured.
	Configured bool `json:"configured"`
	// Kind is the store's kind (s3, file, memory); never its bucket, prefix
	// or directory. Omitted when none is configured.
	Kind string `json:"kind,omitempty"`
	// Reachable is the outcome of the last operation on the store: true when
	// the store answered (a missing object is an answer), false when it
	// failed. Omitted while no operation has been made.
	Reachable *bool `json:"reachable,omitempty"`
	// Offloading reports whether payloads over the threshold are written to
	// the store: a store, a threshold that is not 0 and the payload table
	// written.
	Offloading bool `json:"offloading"`
	// LastSuccessAt and LastFailureAt are the last operation that the store
	// answered and the last one that failed (UTC).
	LastSuccessAt *time.Time `json:"lastSuccessAt,omitempty"`
	LastFailureAt *time.Time `json:"lastFailureAt,omitempty"`
	// LastFailure is the operation that failed last: put, get or startup.
	LastFailure HealthOperation `json:"lastFailure,omitempty"`
	// WindowSeconds is DegradedWindow: how long a failure keeps the store
	// DEGRADED.
	WindowSeconds int `json:"windowSeconds"`
}

// health records the outcomes of the store's operations. Safe for concurrent
// use; a nil *health records nothing.
type health struct {
	mu          sync.Mutex
	lastSuccess time.Time
	lastFailure time.Time
	failedOp    HealthOperation
}

func (h *health) success(at time.Time) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if at.After(h.lastSuccess) {
		h.lastSuccess = at
	}
}

func (h *health) failure(at time.Time, op HealthOperation) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !at.Before(h.lastFailure) {
		h.lastFailure = at
		h.failedOp = op
	}
}

func (h *health) snapshot() (success, failure time.Time, op HealthOperation) {
	if h == nil {
		return time.Time{}, time.Time{}, ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lastSuccess, h.lastFailure, h.failedOp
}

// Health reports the payload store's health at the service's current time
// (docs/payload-offload-design.md, sections 7 and 13). No store configured is
// NOT_CONFIGURED, a healthy state; a failed write or read of the store within
// DegradedWindow is DEGRADED; anything else is UP. It never calls the store.
// Safe on a nil service (NOT_CONFIGURED).
func (s *Service) Health() HealthReport {
	report := HealthReport{Status: HealthNotConfigured, WindowSeconds: int(DegradedWindow / time.Second)}
	if s == nil || s.store == nil {
		return report
	}
	kind, _ := s.store.Describe()
	report.Configured = true
	report.Kind = kind
	report.Offloading = s.TableWrites() && s.offloading()
	report.Status = HealthUp
	success, failure, op := s.health.snapshot()
	if !success.IsZero() {
		report.LastSuccessAt = &success
	}
	if !failure.IsZero() {
		report.LastFailureAt = &failure
		report.LastFailure = op
		if s.now().Sub(failure) < DegradedWindow {
			report.Status = HealthDegraded
		}
	}
	switch {
	case success.IsZero() && failure.IsZero():
		// Nothing has been tried yet: reachability is unknown.
	case failure.After(success):
		reachable := false
		report.Reachable = &reachable
	default:
		reachable := true
		report.Reachable = &reachable
	}
	return report
}

// startupChecker is implemented by a store that can say cheaply, once at
// startup, whether it can be used. proven is false when the check passed but
// does not show the store reachable (the S3 store's credential chain
// resolving, say): only a failure is then recorded.
type startupChecker interface {
	startupCheck(ctx context.Context) (proven bool, err error)
}

// errStartupCheck marks a failed startup check in the log.
var errStartupCheck = errors.New("payloadstore: startup check failed")

// checkAtStartup makes the store's one startup check, when it has one, and
// records its outcome for health. It never fails the start: a store that
// cannot be used is reported DEGRADED and its writes fall back to the
// database.
func (s *Service) checkAtStartup(ctx context.Context) {
	c, ok := s.store.(startupChecker)
	if !ok {
		return
	}
	proven, err := c.startupCheck(ctx)
	switch {
	case err != nil:
		s.health.failure(s.now(), HealthOpStartup)
		kind, _ := s.store.Describe()
		slog.Error("payload store: startup check failed; health reports the store DEGRADED and writes fall back to the database",
			"store_kind", kind, "err", errors.Join(errStartupCheck, err))
	case proven:
		s.health.success(s.now())
	}
}
