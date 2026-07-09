package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// defaultMessageGroup is the grouping key for jobs without a
// message_group — mirrors Rust's DEFAULT_MESSAGE_GROUP (poller.rs).
const defaultMessageGroup = "default"

// PausedConnectionCache caches the set of subscription IDs whose target
// connections are PAUSED. The poller filters jobs whose subscription
// matches; those jobs sit in PENDING until the connection is reactivated.
type PausedConnectionCache struct {
	pool *pgxpool.Pool
	ttl  time.Duration

	mu          sync.RWMutex
	paused      map[string]struct{}
	lastRefresh time.Time
}

// NewPausedConnectionCache wires the cache.
func NewPausedConnectionCache(pool *pgxpool.Pool, ttl time.Duration) *PausedConnectionCache {
	return &PausedConnectionCache{
		pool:        pool,
		ttl:         ttl,
		paused:      make(map[string]struct{}),
		lastRefresh: time.Now().Add(-2 * ttl), // force initial refresh
	}
}

// PausedSubscriptionIDs returns the cached set, refreshing if stale.
func (c *PausedConnectionCache) PausedSubscriptionIDs(ctx context.Context) (map[string]struct{}, error) {
	c.mu.RLock()
	if time.Since(c.lastRefresh) < c.ttl {
		out := make(map[string]struct{}, len(c.paused))
		for k := range c.paused {
			out[k] = struct{}{}
		}
		c.mu.RUnlock()
		return out, nil
	}
	c.mu.RUnlock()
	if err := c.refresh(ctx); err != nil {
		return nil, err
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]struct{}, len(c.paused))
	for k := range c.paused {
		out[k] = struct{}{}
	}
	return out, nil
}

func (c *PausedConnectionCache) refresh(ctx context.Context) error {
	rows, err := c.pool.Query(ctx,
		`SELECT s.id FROM msg_subscriptions s
		   JOIN msg_connections c ON c.id = s.connection_id
		  WHERE c.status = 'PAUSED'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	paused := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		paused[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	c.paused = paused
	c.lastRefresh = time.Now()
	c.mu.Unlock()
	slog.Debug("paused connection cache refreshed", "paused_subscriptions", len(paused))
	return nil
}

// PendingJobPoller polls msg_dispatch_jobs for PENDING jobs ready to
// dispatch (next_retry_at <= NOW or null), filters them through the
// pause + block-on-error checks, and submits to the MessageGroupDispatcher.
type PendingJobPoller struct {
	cfg         Config
	pool        *pgxpool.Pool
	dispatcher  *MessageGroupDispatcher
	pausedCache *PausedConnectionCache
	// IsLeader gates claiming: when non-nil and false, the poller idles.
	// The per-group FIFO dispatcher is in-process only, so within-group
	// ordering requires a single active scheduler — concurrent SKIP-LOCKED
	// claims across replicas would dispatch a group's jobs out of order.
	// nil = always run (standby disabled). Set by Scheduler.Run.
	IsLeader func() bool
}

// NewPendingJobPoller wires the poller.
func NewPendingJobPoller(cfg Config, pool *pgxpool.Pool, dispatcher *MessageGroupDispatcher, pausedCache *PausedConnectionCache) *PendingJobPoller {
	return &PendingJobPoller{cfg: cfg, pool: pool, dispatcher: dispatcher, pausedCache: pausedCache}
}

// Run drives the poller until ctx is cancelled.
func (p *PendingJobPoller) Run(ctx context.Context) {
	tick := time.NewTicker(p.cfg.PollInterval)
	defer tick.Stop()
	slog.Info("dispatch job poller starting", "interval", p.cfg.PollInterval, "batch_size", p.cfg.BatchSize)
	for {
		select {
		case <-ctx.Done():
			slog.Info("dispatch job poller stopped")
			return
		case <-tick.C:
			if p.IsLeader != nil && !p.IsLeader() {
				continue // only the leader claims
			}
			if err := p.pollOnce(ctx); err != nil {
				slog.Warn("poll error", "err", err)
			}
		}
	}
}

// pollOnce claims a batch of jobs and submits them to the dispatcher.
func (p *PendingJobPoller) pollOnce(ctx context.Context) error {
	paused, err := p.pausedCache.PausedSubscriptionIDs(ctx)
	if err != nil {
		return err
	}
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Claim PENDING jobs ready for dispatch. SKIP LOCKED so multiple
	// scheduler instances don't contend.
	// Retry timing is owned by the dispatcher's backoff loop, not by a
	// scheduled-for column on the row — matches the Rust poller in
	// crates/fc-platform/src/scheduler/poller.rs. The embedded schema
	// has neither `next_retry_at` (only added in migration 011's
	// no-op-on-embedded CREATE TABLE IF NOT EXISTS) nor a scheduled-
	// filtered claim path.
	// scheduled_for gates retry backoff: /api/dispatch/process reschedules a
	// failed job to NOW()+backoff (status back to PENDING) and ACKs the queue
	// message, so the poller is the single re-dispatch driver — no queue-NACK
	// racing the poll. A NULL scheduled_for (every freshly-created job) is
	// always eligible.
	rows, err := tx.Query(ctx,
		`SELECT id, subscription_id, message_group, mode, attempt_count, target_url
		   FROM msg_dispatch_jobs
		  WHERE status = 'PENDING'
		    AND (scheduled_for IS NULL OR scheduled_for <= NOW())
		  ORDER BY message_group ASC NULLS LAST, sequence ASC, created_at ASC
		  LIMIT $1
		  FOR UPDATE SKIP LOCKED`,
		p.cfg.BatchSize)
	if err != nil {
		return err
	}
	var claims []dispatchClaim
	for rows.Next() {
		var c dispatchClaim
		var msgGroup *string
		var subID *string
		if err := rows.Scan(&c.id, &subID, &msgGroup, &c.mode, &c.attempt, &c.target); err != nil {
			rows.Close()
			return err
		}
		if subID != nil {
			c.subID = *subID
		}
		if msgGroup != nil {
			c.group = *msgGroup
		}
		claims = append(claims, c)
	}
	rows.Close()
	if len(claims) == 0 {
		return nil
	}

	// Filter and gather IDs to mark QUEUED. Dispatch is deliberately
	// deferred until the claim tx has committed: publishing while the tx was
	// still open meant (a) a commit failure after a publish re-claimed the
	// already-published job on the next poll (duplicate dispatch), and (b) a
	// publish failure's QUEUED→PENDING revert no-oped because the QUEUED
	// status it guards on hadn't committed yet (row stuck until stale
	// recovery).
	//
	// Filter order mirrors the Rust poll (poller.rs): paused-subscription
	// filter, then group, then the blocked-group hold-back, then the
	// per-mode filter. Skipped claims are simply left PENDING — their row
	// locks release at commit and the next poll retries them.
	live, skippedPaused := filterPausedSubscriptions(claims, paused)

	byGroup := groupByMessageGroup(live)
	candidates := make([]string, 0, len(byGroup))
	for g := range byGroup {
		candidates = append(candidates, g)
	}
	blocked, err := blockedGroups(ctx, tx, candidates)
	if err != nil {
		return err
	}

	var queued []string
	var tokens []DispatchJobToken
	skippedBlocked := 0
	for group, jobs := range byGroup {
		// A FAILED/ERROR sibling holds back the whole group this tick —
		// ordered jobs must not jump past the failure, and the operator
		// resolving it (retry/cancel) unblocks the group for the next
		// poll. Rust skips the group before its mode filter, so even
		// IMMEDIATE jobs in a blocked group wait; preserved 1:1.
		if _, isBlocked := blocked[group]; isBlocked {
			slog.Debug("message group blocked, skipping", "group", group, "count", len(jobs))
			skippedBlocked += len(jobs)
			continue
		}
		for _, c := range filterByDispatchMode(jobs, blocked) {
			queued = append(queued, c.id)
			tokens = append(tokens, DispatchJobToken{
				JobID:        c.id,
				MessageGroup: c.group,
				TargetURL:    c.target,
			})
		}
	}

	if len(queued) > 0 {
		if _, err := tx.Exec(ctx,
			`UPDATE msg_dispatch_jobs SET status = 'QUEUED', updated_at = NOW()
			  WHERE id = ANY($1)`, queued); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}

	// QUEUED is durable — now hand the jobs to the dispatcher. A publish
	// failure reverts QUEUED→PENDING (dispatcher.dispatch), which is
	// guaranteed to see the committed status; a crash between commit and
	// Submit leaves rows QUEUED for stale recovery — the same failure mode
	// as a crash mid-publish, handled by the existing recovery loop.
	for _, t := range tokens {
		p.dispatcher.Submit(ctx, t)
	}

	if len(queued) > 0 || skippedPaused > 0 || skippedBlocked > 0 {
		slog.Debug("poll tick",
			"queued", len(queued),
			"skipped_paused", skippedPaused,
			"skipped_blocked", skippedBlocked)
	}
	return nil
}

// dispatchClaim is one PENDING row claimed by the poll query. group and
// subID are "" when the column is NULL.
type dispatchClaim struct {
	id, subID, group, mode, target string
	attempt                        int32
}

// messageGroupKey maps a claim's message_group to its grouping key: jobs
// without a group bucket under "default", mirroring Rust's
// group_by_message_group (poller.rs).
func messageGroupKey(group string) string {
	if group == "" {
		return defaultMessageGroup
	}
	return group
}

// groupByMessageGroup buckets claims by grouping key. The poll query's
// (message_group, sequence, created_at) order is preserved within each
// group — that order is what the dispatcher's per-group FIFO relies on.
func groupByMessageGroup(claims []dispatchClaim) map[string][]dispatchClaim {
	grouped := make(map[string][]dispatchClaim)
	for _, c := range claims {
		key := messageGroupKey(c.group)
		grouped[key] = append(grouped[key], c)
	}
	return grouped
}

// filterPausedSubscriptions drops claims whose subscription's connection
// is PAUSED; they sit in PENDING until the connection is reactivated.
// Claims without a subscription always pass. Returns the survivors and
// the dropped count.
func filterPausedSubscriptions(claims []dispatchClaim, paused map[string]struct{}) ([]dispatchClaim, int) {
	if len(paused) == 0 {
		return claims, 0
	}
	kept := make([]dispatchClaim, 0, len(claims))
	for _, c := range claims {
		if c.subID != "" {
			if _, isPaused := paused[c.subID]; isPaused {
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept, len(claims) - len(kept)
}

// filterByDispatchMode keeps the claims whose mode allows dispatch given
// the blocked groups: IMMEDIATE always dispatches; NEXT_ON_ERROR and
// BLOCK_ON_ERROR hold back while their group is blocked. 1:1 port of
// Rust's filter_by_dispatch_mode (poller.rs) — including the lenient
// parse where unknown modes count as IMMEDIATE. The group-level skip in
// pollOnce makes this currently redundant (a blocked group never reaches
// it), but it's kept for fidelity and so a future relaxation of the
// group skip doesn't silently lose the per-mode semantics.
func filterByDispatchMode(claims []dispatchClaim, blocked map[string]struct{}) []dispatchClaim {
	kept := make([]dispatchClaim, 0, len(claims))
	for _, c := range claims {
		switch common.ParseDispatchMode(c.mode) {
		case common.DispatchNextOnError, common.DispatchBlockOnError:
			if _, isBlocked := blocked[messageGroupKey(c.group)]; isBlocked {
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept
}

// blockedGroups returns the subset of candidate groups that currently
// hold a FAILED or ERROR job — one batch query per poll, the port of
// Rust's BlockOnErrorChecker (mod.rs). A NULL message_group can never
// block: `= ANY` never matches NULL, so a failed ungrouped job does not
// hold back the "default" bucket. Preserve that exactly — only a row
// whose message_group is literally 'default' blocks ungrouped jobs.
func blockedGroups(ctx context.Context, tx pgx.Tx, groups []string) (map[string]struct{}, error) {
	blocked := make(map[string]struct{})
	if len(groups) == 0 {
		return blocked, nil
	}
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT message_group FROM msg_dispatch_jobs
		  WHERE message_group = ANY($1) AND status IN ('FAILED', 'ERROR')`,
		groups)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g string
		if err := rows.Scan(&g); err != nil {
			return nil, err
		}
		blocked[g] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return blocked, nil
}

// DispatchJobToken is the value the poller hands the dispatcher. It
// carries just enough to publish to the queue without re-reading the
// job row.
type DispatchJobToken struct {
	JobID        string
	MessageGroup string
	TargetURL    string
}
