package outbox

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Config tunes the outbox processor.
type Config struct {
	PollInterval time.Duration
	BatchSize    int
	MaxInFlight  int64
	HTTPTimeout  time.Duration
	PlatformURL  string
	AuthToken    string
	// TokenSource, when non-nil, supplies the platform bearer token per request
	// (self-refreshing) and takes precedence over the static AuthToken. Used by
	// the standalone poller to mint its own client_credentials token.
	TokenSource TokenSource
	// MaxRetries no longer does anything and is kept so callers that set it
	// still compile. It used to cap re-queues of a retryable failure, after
	// which the row kept its failure status and was never sent: an outage of a
	// few seconds lost events. A retryable failure is now retried until it
	// succeeds, with RetryBackoffBase..RetryBackoffMax between attempts.
	//
	// Deprecated: retryable failures are retried without a limit.
	MaxRetries int
	// RetryBackoffBase / RetryBackoffMax bound the hold after a retryable
	// failure (5xx, transport error, 401, an unreadable answer): the message
	// group (or, for ungrouped rows, the item type or the row) waits
	// RetryBackoffBase after its first consecutive failure, doubling with each
	// further one up to RetryBackoffMax, less up to a quarter of jitter.
	// Defaults 1 s and 5 min.
	RetryBackoffBase time.Duration
	RetryBackoffMax  time.Duration
	// RecoveryInterval / RecoveryThreshold drive crash recovery (OB2): every
	// RecoveryInterval, rows stuck IN_PROGRESS longer than RecoveryThreshold
	// (claimed by a since-crashed processor) are reset to PENDING.
	RecoveryInterval  time.Duration
	RecoveryThreshold time.Duration
	// MaxConcurrentGroups caps how many distinct message groups dispatch
	// concurrently (OB7). <= 0 = unbounded.
	MaxConcurrentGroups int
	// BlockOnError stops a message group at a non-retryable failure until an
	// operator unblocks (retries) or skips the row, releasing the rest of the
	// group to re-run in order behind it (OB4 ordering guarantee). The block
	// is rebuilt from the stored failed rows at start-up. Default true.
	// Ungrouped items are unaffected. Off, a non-retryable row is left failed
	// and its group carries on past it.
	BlockOnError bool
}

// DefaultConfig returns the standard outbox defaults.
func DefaultConfig() Config {
	return Config{
		PollInterval:        1 * time.Second,
		BatchSize:           100,
		MaxInFlight:         1000,
		HTTPTimeout:         30 * time.Second,
		RetryBackoffBase:    defaultRetryBackoffBase,
		RetryBackoffMax:     defaultRetryBackoffMax,
		RecoveryInterval:    60 * time.Second,
		RecoveryThreshold:   5 * time.Minute,
		MaxConcurrentGroups: 10,
		BlockOnError:        true,
	}
}

const (
	defaultRetryBackoffBase = time.Second
	defaultRetryBackoffMax  = 5 * time.Minute
)

// retryBackoff is the hold after the n-th consecutive retryable failure (n >= 1): base doubled
// n-1 times, at most ceiling, less jitter (in [0, 1)) times a quarter of it.
func retryBackoff(n int, base, ceiling time.Duration, jitter float64) time.Duration {
	if base <= 0 {
		base = defaultRetryBackoffBase
	}
	if ceiling < base {
		ceiling = base
	}
	d := base
	for i := 1; i < n && d < ceiling; i++ {
		d *= 2
	}
	d = min(d, ceiling)
	return d - time.Duration(jitter*float64(d)/4)
}

// Processor wires the outbox pipeline:
//
//	repo.ClaimPending → groupDistributor → httpDispatcher → repo.MarkSuccess/Failed
//
// No row is ever given up on. A retryable failure puts the row back to PENDING
// and holds what it belongs to — its message group, else its type (the whole
// request failed) or the row alone — with a growing backoff; a non-retryable
// failure leaves the row failed and blocks its group (BlockOnError) until an
// operator resolves it, and the block outlives a restart.
type Processor struct {
	cfg          Config
	repo         Repository
	dispatcher   *HTTPDispatcher
	distributor  *GroupDistributor
	groups       *GroupStateManager
	inFlight     atomic.Int64
	totalSucceed atomic.Uint64
	totalFailed  atomic.Uint64
	totalRetried atomic.Uint64

	// IsLeader gates polling; nil means always-leader (single instance /
	// standby disabled). When standby is enabled only the leader polls — the
	// Mongo backend has no atomic claim, so a single active poller avoids
	// double-claims.
	IsLeader func() bool

	// prepared is set once the start-up sweep and the blocked-group rebuild
	// have run for the current leadership (see prepare). Read and written by
	// the Run goroutine only.
	prepared bool

	// holds keeps what must wait until its time: a message group, an item type
	// (its ungrouped rows) or a single row. A 429's Retry-After sets one, and so
	// does a retryable failure, for a backoff that grows with the key's
	// consecutive failures (failures).
	holdMu   sync.Mutex
	holds    map[string]time.Time
	failures map[string]int
}

// Hold key prefixes: what a hold applies to.
const (
	holdGroupPrefix = "group:"
	holdTypePrefix  = "type:"
	holdItemPrefix  = "item:"
)

// holdKey names what a 429 holds: the item's message group, else its type (ungrouped rows are
// sent in batches per type).
func holdKey(item Item) string {
	if grouped(item) {
		return holdGroupPrefix + *item.MessageGroup
	}
	return holdTypePrefix + string(item.ItemType)
}

// itemHoldKey names a hold on one ungrouped row.
func itemHoldKey(item Item) string { return holdItemPrefix + item.ID }

func grouped(item Item) bool { return item.MessageGroup != nil && *item.MessageGroup != "" }

func (p *Processor) hold(item Item, d time.Duration) { p.holdUntil(holdKey(item), time.Now().Add(d)) }

// holdUntil holds key until until, or keeps a later hold it already has: a straggling answer
// never shortens a hold.
func (p *Processor) holdUntil(key string, until time.Time) {
	p.holdMu.Lock()
	defer p.holdMu.Unlock()
	if p.holds == nil {
		p.holds = map[string]time.Time{}
	}
	if until.After(p.holds[key]) {
		p.holds[key] = until
	}
}

// backoff records a retryable failure of key and holds it for the backoff of its consecutive
// failures. A failure that comes back while key is already held was sent before the hold (another
// batch of the same type in flight) and does not escalate it. It logs each escalation once.
func (p *Processor) backoff(key string, item Item, out DispatchOutcome) {
	now := time.Now()
	p.holdMu.Lock()
	if p.holds == nil {
		p.holds = map[string]time.Time{}
	}
	if p.failures == nil {
		p.failures = map[string]int{}
	}
	if now.Before(p.holds[key]) {
		p.holdMu.Unlock()
		return
	}
	p.failures[key]++
	n := p.failures[key]
	d := retryBackoff(n, p.cfg.RetryBackoffBase, p.cfg.RetryBackoffMax, rand.Float64()) //nolint:gosec // G404: jitter, not a secret
	p.holds[key] = now.Add(d)
	p.holdMu.Unlock()
	slog.Warn("outbox holding after a retryable failure; the row stays pending and is retried",
		"hold", key, "consecutiveFailures", n, "retryIn", d.Round(time.Millisecond),
		"id", item.ID, "attempts", item.AttemptCount+1, "status", out.Status.String(), "error", out.Message)
}

// recovered forgets key's consecutive failures after a success, logging the recovery.
func (p *Processor) recovered(key string) {
	p.holdMu.Lock()
	n := p.failures[key]
	delete(p.failures, key)
	p.holdMu.Unlock()
	if n > 0 {
		slog.Info("outbox delivering again after retryable failures", "hold", key, "consecutiveFailures", n)
	}
}

// held reports whether item's group (or type, or the row itself) is held now.
func (p *Processor) held(item Item) bool { return p.heldAt(item, time.Now()) }

// heldAt reports whether item's group (or type, or the row itself) is held at now, and forgets
// holds that have passed by then.
func (p *Processor) heldAt(item Item, now time.Time) bool {
	p.holdMu.Lock()
	defer p.holdMu.Unlock()
	if len(p.holds) == 0 {
		return false
	}
	for k, until := range p.holds {
		if !now.Before(until) {
			delete(p.holds, k)
		}
	}
	if now.Before(p.holds[holdKey(item)]) {
		return true
	}
	return !grouped(item) && now.Before(p.holds[itemHoldKey(item)])
}

// HoldInfo is one current hold (for the status/admin surface).
type HoldInfo struct {
	// Key is "group:<message group>", "type:<item type>" (ungrouped rows of the type) or
	// "item:<row id>".
	Key   string    `json:"key"`
	Until time.Time `json:"until"`
	// ConsecutiveFailures counts the retryable failures behind the hold; 0 for a 429 hold.
	ConsecutiveFailures int `json:"consecutiveFailures"`
}

// Holds returns the holds in force now: what is waiting out a 429 or a retryable failure.
func (p *Processor) Holds() []HoldInfo {
	now := time.Now()
	p.holdMu.Lock()
	defer p.holdMu.Unlock()
	out := make([]HoldInfo, 0, len(p.holds))
	for k, until := range p.holds {
		if now.Before(until) {
			out = append(out, HoldInfo{Key: k, Until: until.UTC(), ConsecutiveFailures: p.failures[k]})
		}
	}
	slices.SortFunc(out, func(a, b HoldInfo) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// claimFilter is what the next claim leaves alone: inactive groups and the holds in force at now.
func (p *Processor) claimFilter(now time.Time) ClaimFilter {
	f := ClaimFilter{Groups: p.groups.Inactive()}
	p.holdMu.Lock()
	defer p.holdMu.Unlock()
	for k, until := range p.holds {
		if !now.Before(until) {
			continue
		}
		switch {
		case strings.HasPrefix(k, holdGroupPrefix):
			f.Groups = append(f.Groups, strings.TrimPrefix(k, holdGroupPrefix))
		case strings.HasPrefix(k, holdTypePrefix):
			f.Types = append(f.Types, common.OutboxItemType(strings.TrimPrefix(k, holdTypePrefix)))
		case strings.HasPrefix(k, holdItemPrefix):
			f.IDs = append(f.IDs, strings.TrimPrefix(k, holdItemPrefix))
		}
	}
	return f
}

// NewProcessor wires a processor.
func NewProcessor(cfg Config, repo Repository) *Processor {
	d := NewHTTPDispatcher(cfg.PlatformURL, cfg.AuthToken, cfg.HTTPTimeout)
	d.tokenSource = cfg.TokenSource
	return &Processor{
		cfg:         cfg,
		repo:        repo,
		dispatcher:  d,
		distributor: NewGroupDistributor(cfg.MaxConcurrentGroups, cfg.BlockOnError),
		groups:      NewGroupStateManager(),
	}
}

// prepare runs before the first claim of a leadership: it returns rows an older version left in a
// retryable failure status to PENDING, marks rows an older version refused (no marker) as skipped
// before upgrade, and (BlockOnError) loads the blocked groups from the rows this version refused
// (BlockingMarker), so a restart or a new leader never moves a group past its poison row. Until it
// succeeds nothing is claimed.
func (p *Processor) prepare(ctx context.Context) error {
	n, err := p.repo.RequeueRetryableFailures(ctx)
	if err != nil {
		return fmt.Errorf("requeue retryable failures: %w", err)
	}
	if n > 0 {
		slog.Warn("outbox returned rows left in a retryable failure status to pending", "count", n)
	} else {
		slog.Info("outbox start-up sweep: no rows left in a retryable failure status")
	}
	legacy, err := p.repo.MarkLegacyRefusals(ctx)
	if err != nil {
		return fmt.Errorf("mark legacy refusals: %w", err)
	}
	if legacy > 0 {
		slog.Warn("outbox marked rows refused by an earlier version as skipped before upgrade; their groups carry on",
			"count", legacy, "marker", SkippedBeforeUpgradeMarker)
	}
	if !p.cfg.BlockOnError {
		return nil
	}
	heads, err := p.repo.BlockingFailures(ctx)
	if err != nil {
		return fmt.Errorf("load blocking failures: %w", err)
	}
	blocks := make([]GroupInfo, 0, len(heads))
	for _, h := range heads {
		if !grouped(h) {
			continue
		}
		msg := StripMarkers(h.StatusMessage)
		blocks = append(blocks, GroupInfo{Group: *h.MessageGroup, Status: GroupBlocked, BlockedItemID: h.ID, Error: msg})
		slog.Warn("outbox message group blocked by a stored failure; unblock or skip it to resume",
			"group", *h.MessageGroup, "id", h.ID, "status", h.Status.String(), "error", msg)
	}
	p.groups.ReplaceBlocked(blocks)
	slog.Info("outbox blocked groups loaded from stored failures", "count", len(blocks))
	return nil
}

// Run drives the processor until ctx is cancelled. Two tickers: the poll
// loop (claim + dispatch) and the crash-recovery loop (reset stuck rows).
func (p *Processor) Run(ctx context.Context) {
	tick := time.NewTicker(p.cfg.PollInterval)
	defer tick.Stop()
	recoveryInterval := p.cfg.RecoveryInterval
	if recoveryInterval <= 0 {
		recoveryInterval = 60 * time.Second
	}
	recoveryTick := time.NewTicker(recoveryInterval)
	defer recoveryTick.Stop()
	// A repository that can listen wakes the loop when a PUSH row is written (ChannelPush).
	var woken chan struct{}
	if w, ok := p.repo.(Waker); ok {
		woken = make(chan struct{}, 1)
		listenCtx, stopListening := context.WithCancel(ctx)
		defer stopListening()
		go w.Listen(listenCtx, func() {
			select {
			case woken <- struct{}{}:
			default: // one wake is already pending
			}
		})
	}
	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox processor stopped")
			return
		case <-woken:
			if p.IsLeader != nil && !p.IsLeader() {
				p.prepared = false // a new leadership reloads the stored state
				continue
			}
			if p.inFlight.Load() >= p.cfg.MaxInFlight {
				continue // backpressure; the next poll picks the rows up
			}
			p.tick(ctx)
		case <-tick.C:
			if p.IsLeader != nil && !p.IsLeader() {
				p.prepared = false // only the leader polls; a new leadership reloads the stored state
				continue
			}
			if p.inFlight.Load() >= p.cfg.MaxInFlight {
				continue // backpressure
			}
			p.tick(ctx)
		case <-recoveryTick.C:
			if p.IsLeader != nil && !p.IsLeader() {
				continue
			}
			threshold := p.cfg.RecoveryThreshold
			if threshold <= 0 {
				threshold = 5 * time.Minute
			}
			if n, err := p.repo.RecoverStuck(ctx, threshold); err != nil {
				slog.Warn("outbox recover stuck failed", "err", err)
			} else if n > 0 {
				slog.Info("outbox recovered stuck items", "count", n)
			}
		}
	}
}

func (p *Processor) tick(ctx context.Context) {
	if !p.prepared {
		if err := p.prepare(ctx); err != nil {
			slog.Warn("outbox start-up preparation failed; nothing is claimed until it succeeds", "err", err)
			return
		}
		p.prepared = true
	}
	items, err := p.repo.ClaimPending(ctx, p.cfg.BatchSize, p.claimFilter(time.Now()))
	if err != nil {
		slog.Warn("outbox claim failed", "err", err)
		return
	}
	if len(items) == 0 {
		return
	}
	// A claim's rows come back in no particular order (UPDATE ... RETURNING), and rows written in
	// the same instant tie on created_at: order them as written, (created_at, id), before a group's
	// rows go to its serial lane. The id breaks ties: a TSID grows with time.
	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), strings.Compare(a.ID, b.ID))
	})

	// Partition the claim: grouped items keep strict per-group FIFO +
	// block-on-error (serial via the distributor, OB7-bounded); ungrouped items
	// are batched by ItemType into a single HTTP call each (OB4 throughput —
	// there's no ordering to preserve for them).
	byType := make(map[common.OutboxItemType][]Item)
	// One instant for the whole claim, and a group (or type) that had a row put back stays put
	// back for the rest of the claim: a hold expiring, or a group resuming, half-way through must
	// not send a later row ahead of an earlier one.
	now := time.Now()
	putBack := map[string]bool{}
	for _, item := range items {
		key := holdKey(item)
		// a group (or type, or row) that is waiting: back to PENDING, no penalty
		if putBack[key] || p.heldAt(item, now) {
			putBack[key] = true
			p.release(ctx, item)
			continue
		}
		if grouped(item) {
			// State machine: skip a Paused/Blocked group — release its claimed
			// items back to PENDING (re-claimed once the group is resumed/
			// unblocked) instead of dispatching past a block.
			if !p.groups.IsActive(*item.MessageGroup) {
				putBack[key] = true
				p.release(ctx, item)
				continue
			}
			p.inFlight.Add(1)
			p.distributor.Submit(item,
				func() bool {
					defer p.inFlight.Add(-1)
					// The group may have been held (or paused) since the claim, by an earlier row
					// failing: never send a later row ahead of it, whatever BlockOnError says.
					if p.held(item) || !p.groups.IsActive(*item.MessageGroup) {
						p.release(ctx, item)
						return false
					}
					return p.dispatch(ctx, item)
				},
				func() {
					defer p.inFlight.Add(-1)
					p.release(ctx, item)
				})
			continue
		}
		byType[item.ItemType] = append(byType[item.ItemType], item)
	}
	for _, batch := range byType {
		p.inFlight.Add(int64(len(batch)))
		go p.dispatchBatch(ctx, batch)
	}
}

// dispatchBatch sends a batch of ungrouped, same-ItemType items in one HTTP
// call (OB4) and records each item's outcome — MarkSuccess in bulk, and per
// item the same rules as dispatch: a retryable failure goes back to PENDING
// under a hold, a non-retryable one keeps its failure status. When every item
// failed retryably (the request itself failed) the type is held; otherwise
// only the failed rows are, so one bad row does not slow its whole type.
func (p *Processor) dispatchBatch(ctx context.Context, batch []Item) {
	defer p.inFlight.Add(-int64(len(batch)))
	outcomes := p.dispatcher.SendBatch(ctx, batch)
	var succeeded []string
	var retry []Item
	for _, item := range batch {
		out, ok := outcomes[item.ID]
		if !ok {
			out = DispatchOutcome{Status: common.OutboxInternalError, Message: "no per-item result"}
		}
		if out.RetryAfter > 0 {
			p.hold(item, out.RetryAfter)
			p.release(ctx, item)
			continue
		}
		switch {
		case out.Status == common.OutboxSuccess:
			succeeded = append(succeeded, item.ID)
			p.totalSucceed.Add(1)
		case out.Status.IsRetryable():
			retry = append(retry, item)
		default:
			p.refuse(ctx, item, out)
		}
	}
	if len(succeeded) > 0 {
		if err := p.repo.MarkSuccess(ctx, succeeded); err != nil {
			slog.Warn("outbox mark success failed (batch)", "count", len(succeeded), "err", err)
		}
	}
	if len(retry) == 0 {
		if len(succeeded) > 0 {
			p.recovered(holdKey(batch[0]))
			for _, item := range batch {
				p.recovered(itemHoldKey(item))
			}
		}
		return
	}
	// Hold before the rows go back to PENDING, so no poll can claim them ahead of the hold.
	if len(retry) == len(batch) {
		p.backoff(holdKey(retry[0]), retry[0], outcomes[retry[0].ID])
	} else {
		p.recovered(holdKey(batch[0]))
		for _, item := range retry {
			p.backoff(itemHoldKey(item), item, outcomes[item.ID])
		}
	}
	for _, item := range retry {
		out := outcomes[item.ID]
		p.requeue(ctx, item, out)
	}
}

// requeue records a retryable failure and puts the row back to PENDING: it is never left in a
// failure status, however many attempts it has had.
func (p *Processor) requeue(ctx context.Context, item Item, out DispatchOutcome) {
	msg := out.Message
	if msg == "" {
		msg = "no per-item result"
	}
	if err := p.repo.MarkFailed(ctx, []string{item.ID}, out.Status, msg, true); err != nil {
		slog.Warn("outbox requeue failed; crash recovery will return the row", "id", item.ID, "err", err)
	}
	p.totalRetried.Add(1)
}

// refuse records a non-retryable failure: the row keeps its failure status and its message is
// stored behind BlockingMarker, which is what blocks its group again after a restart.
func (p *Processor) refuse(ctx context.Context, item Item, out DispatchOutcome) {
	if err := p.repo.MarkFailed(ctx, []string{item.ID}, out.Status, BlockingMarker+out.Message, false); err != nil {
		slog.Warn("outbox mark failed", "id", item.ID, "err", err)
	}
	p.totalFailed.Add(1)
}

// release returns an undispatched, group-blocked item to PENDING (no failure
// penalty) so the next poll re-claims it in order behind the failed item.
func (p *Processor) release(ctx context.Context, item Item) {
	if err := p.repo.Release(ctx, []string{item.ID}); err != nil {
		slog.Warn("outbox release failed", "id", item.ID, "err", err)
	}
}

// dispatch sends one grouped item and records its outcome. Returns true on
// success, false otherwise — the distributor then releases the group's later
// rows to re-run behind this one.
func (p *Processor) dispatch(ctx context.Context, item Item) bool {
	out := p.dispatcher.Send(ctx, item)
	if out.RetryAfter > 0 {
		// the platform asked to wait: hold the group, put the row back untouched, and stop the
		// group for this drain (its other claimed rows are released behind it)
		p.hold(item, out.RetryAfter)
		p.release(ctx, item)
		return false
	}
	if out.Status == common.OutboxSuccess {
		if err := p.repo.MarkSuccess(ctx, []string{item.ID}); err != nil {
			slog.Warn("outbox mark success failed", "id", item.ID, "err", err)
			return false
		}
		p.totalSucceed.Add(1)
		p.recovered(holdKey(item))
		return true
	}
	if out.Status.IsRetryable() {
		// Never terminal: hold the group with a growing backoff (before the row is PENDING again,
		// so no poll claims it ahead of the hold), put the row back, and stop the group's drain.
		// retry_count and error_message still record the attempt.
		p.backoff(holdKey(item), item, out)
		p.requeue(ctx, item, out)
		return false
	}
	// Non-retryable: the platform refused the row. It keeps its failure status
	// and, with BlockOnError, blocks its group until an operator unblocks
	// (retry) or skips it; the stored row re-blocks the group after a restart.
	p.refuse(ctx, item, out)
	if p.cfg.BlockOnError && grouped(item) {
		p.groups.Block(*item.MessageGroup, item.ID, out.Message)
		slog.Warn("outbox message group blocked", "group", *item.MessageGroup, "id", item.ID, "error", out.Message)
	}
	return false
}

// ── Operational state machine controls ──

// PauseGroup stops dispatching a message group; its rows stay PENDING (claims
// pass over them) until ResumeGroup. In memory only: a restart resumes it.
// No-op when the group is Blocked.
func (p *Processor) PauseGroup(group string) { p.groups.Pause(group) }

// ResumeGroup resumes a Paused group.
func (p *Processor) ResumeGroup(group string) { p.groups.Resume(group) }

// UnblockGroup re-queues a Blocked group's poison item (a fresh retry) and
// clears the block, so the whole group runs again in order. Returns false if
// the group is not Blocked; on an error the group stays Blocked.
func (p *Processor) UnblockGroup(ctx context.Context, group string) (bool, error) {
	itemID, ok := p.groups.BlockedItem(group)
	if !ok {
		return false, nil
	}
	if itemID != "" {
		if err := p.repo.Requeue(ctx, []string{itemID}); err != nil {
			return true, fmt.Errorf("requeue %s: %w", itemID, err)
		}
	}
	p.groups.ClearBlock(group)
	slog.Info("outbox group unblocked (poison re-queued)", "group", group, "id", itemID)
	return true, nil
}

// SkipGroup clears a Blocked group WITHOUT re-queuing the poison item: it stays
// failed, marked skipped so a restart does not block the group on it again, and
// the group advances past it. Returns false if the group is not Blocked; on an
// error the group stays Blocked.
func (p *Processor) SkipGroup(ctx context.Context, group string) (bool, error) {
	itemID, ok := p.groups.BlockedItem(group)
	if !ok {
		return false, nil
	}
	if itemID != "" {
		if err := p.repo.MarkSkipped(ctx, []string{itemID}); err != nil {
			return true, fmt.Errorf("mark %s skipped: %w", itemID, err)
		}
	}
	p.groups.ClearBlock(group)
	slog.Info("outbox group skipped blocking item", "group", group, "id", itemID)
	return true, nil
}

// GroupStates returns the non-default (Paused/Blocked) message-group states.
func (p *Processor) GroupStates() []GroupInfo { return p.groups.Snapshot() }

// BlockedGroups returns only the Blocked message groups.
func (p *Processor) BlockedGroups() []GroupInfo { return p.groups.Blocked() }

// InFlight returns the count of items currently in dispatch.
func (p *Processor) InFlight() int64 { return p.inFlight.Load() }

// Totals returns (success, failure) counters since process start; failure
// counts non-retryable failures only.
func (p *Processor) Totals() (uint64, uint64) {
	return p.totalSucceed.Load(), p.totalFailed.Load()
}

// Retried returns how many retryable failures were put back to PENDING since
// process start.
func (p *Processor) Retried() uint64 { return p.totalRetried.Load() }
