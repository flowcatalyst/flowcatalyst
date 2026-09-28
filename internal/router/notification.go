package router

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WarningCategory classifies a warning by subsystem.
type WarningCategory string

const (
	WarningCategoryConfiguration  WarningCategory = "CONFIGURATION"
	WarningCategoryConnection     WarningCategory = "CONNECTION"
	WarningCategoryRateLimit      WarningCategory = "RATE_LIMIT"
	WarningCategoryCircuitBreak   WarningCategory = "CIRCUIT_BREAKER"
	WarningCategoryStall          WarningCategory = "STALL"
	WarningCategoryResource       WarningCategory = "RESOURCE"
	WarningCategoryRouting        WarningCategory = "ROUTING"
	WarningCategoryPoolCapacity   WarningCategory = "POOL_CAPACITY"
	WarningCategoryQueueHealth    WarningCategory = "QUEUE_HEALTH"
	WarningCategoryConsumerHealth WarningCategory = "CONSUMER_HEALTH"
)

// WarningSeverity ranks a warning's importance.
type WarningSeverity string

const (
	WarningInfo     WarningSeverity = "INFO"
	WarningWarning  WarningSeverity = "WARNING"
	WarningError    WarningSeverity = "ERROR"
	WarningCritical WarningSeverity = "CRITICAL"
)

// parseWarningSeverity parses FC_NOTIFY_MIN_SEVERITY (case-insensitive) into
// a WarningSeverity. Reports false for "" and for anything not one of the
// four known levels — the ONLY safe default, since severityRank ranks an
// unrecognised value as lowest (INFO), and passing that straight to
// SetMinSeverity would silently reopen the INFO floodgate the WARNING
// default exists to close. Server.NewServer only calls SetMinSeverity when
// this returns true, leaving the default floor untouched otherwise.
func parseWarningSeverity(raw string) (WarningSeverity, bool) {
	switch WarningSeverity(strings.ToUpper(strings.TrimSpace(raw))) {
	case WarningInfo:
		return WarningInfo, true
	case WarningWarning:
		return WarningWarning, true
	case WarningError:
		return WarningError, true
	case WarningCritical:
		return WarningCritical, true
	default:
		return "", false
	}
}

// Warning is a structured operational notice. The same shape is
// persisted by WarningService and forwarded to NotificationService
// consumers without translation.
type Warning struct {
	ID             string          `json:"id"`
	Category       WarningCategory `json:"category"`
	Severity       WarningSeverity `json:"severity"`
	Message        string          `json:"message"`
	Source         string          `json:"source"`
	CreatedAt      time.Time       `json:"createdAt"`
	Acknowledged   bool            `json:"acknowledged"`
	AcknowledgedAt *time.Time      `json:"acknowledgedAt,omitempty"`
}

// NewWarning constructs a Warning with a freshly-minted UUID and the
// current time.
func NewWarning(category WarningCategory, severity WarningSeverity, message, source string) Warning {
	return Warning{
		ID:        uuid.NewString(),
		Category:  category,
		Severity:  severity,
		Message:   message,
		Source:    source,
		CreatedAt: time.Now().UTC(),
	}
}

// AgeMinutes returns the warning's age in whole minutes.
func (w Warning) AgeMinutes() int64 {
	return int64(time.Since(w.CreatedAt).Minutes())
}

// Notifier delivers warnings to an external channel (Teams, Slack, etc.).
// Batches warnings to avoid hammering the destination during incidents.
type Notifier struct {
	webhookURL  string
	batchSize   int
	interval    time.Duration
	minSeverity WarningSeverity // floor below which Add drops a warning; see NewNotifier/SetMinSeverity
	client      *http.Client

	mu    sync.Mutex
	queue []Warning

	stopOnce sync.Once
	stopCh   chan struct{}

	// flushNow asks Run's loop for an immediate flush (a full batch, or a
	// CRITICAL warning). Buffered with room for one: a request made while
	// one is already pending is folded into it, since that flush takes the
	// whole queue anyway. Written only by Add (non-blocking), read only by
	// Run; never closed.
	flushNow chan struct{}
}

// severityRank orders severities for the MinSeverity filter (higher = more
// severe). Unknown/INFO rank lowest.
func severityRank(s WarningSeverity) int {
	switch s {
	case WarningCritical:
		return 3
	case WarningError:
		return 2
	case WarningWarning:
		return 1
	default:
		return 0
	}
}

// NewNotifier builds a notifier. webhookURL empty → noop.
//
// minSeverity defaults to WARNING (X-04): an INFO-severity warning is stored
// by the WarningService (so it still shows on /warnings) but is never
// webhooked unless SetMinSeverity(WarningInfo) is called explicitly. Without
// this default, a chatty INFO source would crowd the destination channel the
// same way it would have crowded the store before the per-severity TTL.
func NewNotifier(webhookURL string, batchSize int, interval time.Duration) *Notifier {
	return &Notifier{
		webhookURL:  webhookURL,
		batchSize:   batchSize,
		interval:    interval,
		minSeverity: WarningWarning,
		client:      &http.Client{Timeout: 10 * time.Second},
		stopCh:      make(chan struct{}),
		flushNow:    make(chan struct{}, 1),
	}
}

// Run starts the flush loop. Returns when ctx is cancelled or Stop is called.
func (n *Notifier) Run(ctx context.Context) {
	if n.webhookURL == "" {
		return // noop
	}
	tick := time.NewTicker(n.interval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done(): // shutdown
			n.finalFlush()
			return
		case <-n.stopCh: // Stop
			n.finalFlush()
			return
		case <-tick.C: // batch interval elapsed
			n.flush(ctx)
		case <-n.flushNow: // Add asked for an immediate flush
			n.flush(ctx)
		}
	}
}

// SetMinSeverity sets the minimum severity that will be delivered; warnings
// below it are dropped before enqueue. NewNotifier defaults this to WARNING
// (X-04); pass WarningInfo to deliver everything, including INFO. Intended
// to be driven by FC_NOTIFY_MIN_SEVERITY at startup — see the deferred
// wiring note on this function.
//
// Deferred: the env var lives in internal/server/envcfg.go and the call site
// in internal/router/server.go, both owned by another lane. When free, add:
//
//	s.Notifier.SetMinSeverity(WarningSeverity(cfg.NotifyMinSeverity))
//
// guarded so an empty/invalid env value leaves the WARNING default in place
// rather than falling through to severityRank's "unknown ranks lowest" and
// silently reopening the INFO floodgate.
func (n *Notifier) SetMinSeverity(s WarningSeverity) {
	n.mu.Lock()
	n.minSeverity = s
	n.mu.Unlock()
}

// Add enqueues a warning. Flushed by the next tick, when the batch is full, or
// immediately for a CRITICAL warning (fast-track, so incidents aren't delayed
// by the batch interval).
// Warnings below MinSeverity are dropped. Fills in ID + CreatedAt if the caller
// passed a bare-literal Warning.
func (n *Notifier) Add(w Warning) {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now().UTC()
	}
	n.mu.Lock()
	if severityRank(w.Severity) < severityRank(n.minSeverity) {
		n.mu.Unlock()
		return
	}
	n.queue = append(n.queue, w)
	flushNow := len(n.queue) >= n.batchSize || w.Severity == WarningCritical
	n.mu.Unlock()
	if flushNow {
		// Hand the flush to Run's loop rather than starting a goroutine per
		// warning: an incident raising CRITICAL warnings by the thousand used
		// to start one goroutine each, every one holding an HTTP call to a
		// webhook that was probably slow by then. At most one request is
		// ever pending, and one flush sends everything queued.
		select {
		case n.flushNow <- struct{}{}:
		default: // a flush is already pending; it will take this warning too
		}
	}
}

// finalFlush sends whatever is still queued on the way out. It runs on a
// context of its own: the run context is already cancelled by then, and a
// request made on it would fail before it was sent, losing exactly the
// warnings raised during the shutdown.
func (n *Notifier) finalFlush() {
	ctx, cancel := context.WithTimeout(context.Background(), n.client.Timeout)
	defer cancel()
	n.flush(ctx)
}

// Stop signals the loop to exit and flushes any pending warnings.
func (n *Notifier) Stop() {
	n.stopOnce.Do(func() { close(n.stopCh) })
}

func (n *Notifier) flush(ctx context.Context) {
	n.mu.Lock()
	if len(n.queue) == 0 || n.webhookURL == "" {
		n.mu.Unlock()
		return
	}
	batch := n.queue
	n.queue = nil
	n.mu.Unlock()

	body, err := json.Marshal(map[string]any{"warnings": batch})
	if err != nil {
		slog.Warn("notifier: marshal failed", "err", err)
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.webhookURL, bytes.NewReader(body))
	if err != nil {
		slog.Warn("notifier: build req failed", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := n.client.Do(req)
	if err != nil {
		slog.Warn("notifier: post failed", "err", err, "batch_size", len(batch))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		slog.Warn("notifier: non-2xx", "status", resp.StatusCode)
	}
}

// String formats a warning for diagnostic logs.
func (w Warning) String() string {
	return fmt.Sprintf("[%s/%s] %s (from %s)", w.Category, w.Severity, w.Message, w.Source)
}
