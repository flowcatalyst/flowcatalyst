package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// defaultMessageGroup is the grouping key for jobs without a
// message_group.
const defaultMessageGroup = "default"

// pollTimeout bounds one whole pollOnce (claim, publish, mark, commit). A stalled
// publish must not hold the claim's row locks and a pool connection for ever: on
// expiry the transaction rolls back and the rows stay PENDING. Generous on
// purpose — a full claim is ~10 sequential SendMessageBatch calls per queue
// (each separately bounded) — so it only ever fires on a genuine stall. A var
// only so tests can shorten it.
var pollTimeout = 2 * time.Minute

// finishTimeout bounds the mark-QUEUED UPDATE and COMMIT that follow a
// successful publish. They run detached from ctx (see pollOnce).
var finishTimeout = 10 * time.Second

// claimSQL is the poller's claim ($1 = batch size, $2 = paused subscription
// ids). Its ORDER BY is exactly the key of idx_dispatch_jobs_pending_poll
// (migration 065) and its status is a literal, so Postgres walks that partial
// index in order — a Merge Append across the partitions — and stops at the
// LIMIT: no sort, however many rows share a created_at and whatever the
// statistics say. TestClaimPlan_NeedsNoSort pins that. The scheduled_for and
// paused predicates are filters on the rows the walk visits: a row they
// exclude that sorts ahead of the batch is visited (and skipped) on every claim.
const claimSQL = `SELECT id, subscription_id, message_group, mode, dispatch_pool_id, client_id,
        attempt_count, target_url, created_at, sequence, queue
   FROM msg_dispatch_jobs
  WHERE status = 'PENDING'
    AND (scheduled_for IS NULL OR scheduled_for <= NOW())
    AND (subscription_id IS NULL OR subscription_id <> ALL($2::text[]))
  ORDER BY message_group ASC NULLS LAST, sequence ASC, created_at ASC, id ASC
  LIMIT $1
  FOR UPDATE SKIP LOCKED`

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
	// poolCodes composes each job's published pool code from its
	// dispatch_pool_id + client_id. Same refresh cadence as pausedCache.
	poolCodes *PoolCodeResolver
	// IsLeader gates claiming: when non-nil and false, the poller idles.
	// The per-group FIFO dispatcher is in-process only, so within-group
	// ordering requires a single active scheduler — concurrent SKIP-LOCKED
	// claims across replicas would dispatch a group's jobs out of order.
	// nil = always run (standby disabled). Set by Scheduler.Run.
	IsLeader func() bool

	// starveWarnedAt is when warnIfStarved last logged; pollOnce runs on one
	// goroutine, so it needs no lock.
	starveWarnedAt time.Time

	// poll is the per-pass claim+publish; nil means pollOnce. A seam so the run
	// loop's drain and fall-back rules can be tested without a database.
	poll func(ctx context.Context) (claimed, published int, err error)
}

// NewPendingJobPoller wires the poller. The pool-code resolver shares
// pausedCache's TTL: both cache slow-moving config read on every claim.
func NewPendingJobPoller(cfg Config, pool *pgxpool.Pool, dispatcher *MessageGroupDispatcher, pausedCache *PausedConnectionCache) *PendingJobPoller {
	return &PendingJobPoller{
		cfg:         cfg,
		pool:        pool,
		dispatcher:  dispatcher,
		pausedCache: pausedCache,
		poolCodes:   NewPoolCodeResolver(pool, cfg.PausedCacheTTL),
	}
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
			p.drain(ctx)
		}
	}
}

// drain runs poll passes for one tick. A pass that claimed a FULL batch and
// published something means the backlog is deeper than one batch, so it polls
// again at once rather than sleeping a PollInterval per 100 jobs (which capped
// throughput at BatchSize/PollInterval). It falls back to the ticker on a short
// batch (backlog drained), on published == 0 (a claim that publishes nothing
// would otherwise spin on the same rows), and on error. The leader gate and ctx
// are re-checked before every pass: leadership can be lost mid-drain.
func (p *PendingJobPoller) drain(ctx context.Context) {
	poll := p.poll
	if poll == nil {
		poll = p.pollOnce
	}
	for {
		if ctx.Err() != nil {
			return
		}
		if p.IsLeader != nil && !p.IsLeader() {
			return // only the leader claims
		}
		claimed, published, err := poll(ctx)
		if err != nil {
			slog.Warn("poll error", "err", err)
			return
		}
		if claimed < p.cfg.BatchSize || published == 0 {
			return
		}
	}
}

// pollOnce claims a batch of jobs and submits them to the dispatcher. It
// reports how many rows the claim returned and how many of them were published
// (marked QUEUED); the run loop uses the pair to decide whether to poll again
// immediately.
func (p *PendingJobPoller) pollOnce(ctx context.Context) (int, int, error) {
	start := time.Now()
	claimed, published, err := p.claimAndPublish(ctx)
	schedMetrics.observePoll(start, err)
	return claimed, published, err
}

// claimAndPublish is the body of one poll; pollOnce wraps it with metrics.
func (p *PendingJobPoller) claimAndPublish(ctx context.Context) (int, int, error) {
	ctx, cancel := context.WithTimeout(ctx, pollTimeout)
	defer cancel()
	paused, err := p.pausedCache.PausedSubscriptionIDs(ctx)
	if err != nil {
		return 0, 0, err
	}
	// The paused set goes INTO the claim query (never nil: `<> ALL(NULL)` is
	// NULL, which would exclude every row).
	pausedIDs := make([]string, 0, len(paused))
	for id := range paused {
		pausedIDs = append(pausedIDs, id)
	}
	schedMetrics.pausedSubscriptions.Set(float64(len(pausedIDs)))
	tx, err := p.pool.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	// Rolled back on a detached, bounded context: ctx may already be cancelled
	// or past its deadline (that is exactly when the rollback matters), and a
	// rollback that cannot run would leave the connection to be torn down with
	// the claim's locks still held.
	defer func() {
		rctx, rcancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
		defer rcancel()
		_ = tx.Rollback(rctx)
	}()

	// Claim PENDING jobs ready for dispatch. SKIP LOCKED so multiple
	// scheduler instances don't contend.
	// Retry timing is owned by the dispatcher's backoff loop, not by a
	// scheduled-for column on the row. The embedded schema
	// has neither `next_retry_at` (only added in migration 011's
	// no-op-on-embedded CREATE TABLE IF NOT EXISTS) nor a scheduled-
	// filtered claim path.
	// scheduled_for gates retry backoff: /api/dispatch/process reschedules a
	// failed job to NOW()+backoff (status back to PENDING) and ACKs the queue
	// message, so the poller is the single re-dispatch driver — no queue-NACK
	// racing the poll. A NULL scheduled_for (every freshly-created job) is
	// always eligible.
	// ORDER BY ends in `id` so the claim order is TOTAL. Ties on
	// (message_group, sequence, created_at) are common — a subscription's
	// sequence is per-subscription, not per-message, so every job of a group
	// bound for one subscriber shares it — and an ordering with ties leaves the
	// rest to the plan, which across a partitioned table can interleave
	// arbitrarily. The id is a time-ordered TSID, so it both breaks the tie and
	// breaks it chronologically. The positional hold-back below needs this
	// total order to compare "earlier" at all.
	rows, err := tx.Query(ctx, claimSQL, p.cfg.BatchSize, pausedIDs)
	if err != nil {
		return 0, 0, err
	}
	var claims []dispatchClaim
	for rows.Next() {
		var c dispatchClaim
		var msgGroup *string
		var subID *string
		var poolID *string
		var clientID *string
		var queue *string
		if err := rows.Scan(&c.id, &subID, &msgGroup, &c.mode, &poolID, &clientID,
			&c.attempt, &c.target, &c.createdAt, &c.sequence, &queue); err != nil {
			rows.Close()
			return 0, 0, err
		}
		if subID != nil {
			c.subID = *subID
		}
		if msgGroup != nil {
			c.group = *msgGroup
		}
		if poolID != nil {
			c.poolID = *poolID
		}
		if clientID != nil {
			c.clientID = *clientID
		}
		if queue != nil {
			c.queue = *queue
		}
		claims = append(claims, c)
	}
	rows.Close()
	if len(claims) == 0 {
		return 0, 0, nil
	}
	schedMetrics.claimed.Add(float64(len(claims)))
	if len(claims) >= p.cfg.BatchSize {
		schedMetrics.fullBatches.Inc()
	}

	// Filter, publish, then mark QUEUED and commit — in that order, with the
	// claim's rows still locked while the batch is published.
	//
	// This used to commit the claim QUEUED first and publish afterwards. A
	// worker that died between the two (a SIGKILL, an OOM, a deploy past its
	// stop timeout) left every unpublished row QUEUED with no queue message,
	// and nothing looked at them again until stale recovery's 75 minutes were
	// up: the delivery harness lost 2 of 40 jobs to a SIGKILL that way
	// (worker-restart, delivery runs 3 and 4). Publishing first means a worker
	// that dies mid-publish rolls the whole claim back to PENDING, and the
	// next poll (its own after a restart, or a standby's) publishes it again.
	//
	// The price is at-least-once at the queue: the jobs it did publish before
	// dying are published a second time. That costs no second delivery —
	// /api/dispatch/process claims a job before delivering it, and a copy
	// that finds the job taken or finished never delivers it — only a
	// second, redundant queue message. A commit that fails after a publish
	// has the same effect. A /process call that arrives for a job before
	// this commit waits on the row lock for it, then claims it QUEUED.
	//
	// Only the jobs the publisher reports published are marked QUEUED; the
	// rest simply stay PENDING for the next poll (no revert needed: their
	// QUEUED status was never written).
	//
	// Paused subscriptions are excluded by the claim query itself, so they can
	// never fill the LIMIT window and starve rows behind them (the claim sorts by
	// message_group, so a run of paused rows sorting early used to be re-claimed,
	// dropped and left PENDING on every tick, with nothing behind them ever
	// published). There is deliberately no second Go-side paused check: it would
	// read the very same cache snapshot the query used, so it could never differ.
	// A connection paused after the snapshot is picked up within the cache TTL,
	// exactly as before.
	//
	// What remains in Go is the blocked-group / BLOCK_ON_ERROR hold-back, which
	// is positional (a job waits behind an EARLIER failed sibling) and so stays
	// out of SQL. Held rows are left PENDING — their row locks release at
	// rollback/commit and the next poll retries them.
	byGroup := groupByMessageGroup(claims)
	candidates := make([]string, 0, len(byGroup))
	for g := range byGroup {
		candidates = append(candidates, g)
	}
	blocked, err := blockedGroups(ctx, tx, candidates)
	if err != nil {
		return 0, 0, err
	}

	var queued []string
	var createdAt []time.Time // createdAt[i] is queued[i]'s created_at
	var tokens []DispatchJobToken
	skippedBlocked := 0
	for group, jobs := range byGroup {
		// A FAILED/ERROR sibling holds back this group's BLOCK_ON_ERROR
		// jobs — they must not jump past the failure, and the operator
		// resolving it (retry/cancel/complete) releases them on the next
		// poll. IMMEDIATE and NEXT_ON_ERROR jobs keep flowing: neither
		// mode promises to stop for a failed sibling.
		dispatchable := filterByDispatchMode(jobs, blocked)
		if held := len(jobs) - len(dispatchable); held > 0 {
			slog.Debug("message group blocked, holding ordered jobs",
				"group", group, "held", held, "dispatching", len(dispatchable))
			skippedBlocked += held
		}
		for _, c := range dispatchable {
			queued = append(queued, c.id)
			createdAt = append(createdAt, c.createdAt)
			tokens = append(tokens, DispatchJobToken{
				JobID:        c.id,
				MessageGroup: c.group,
				TargetURL:    c.target,
				Mode:         c.mode,
				PoolCode:     p.poolCodes.Resolve(ctx, c.poolID, c.clientID),
				// Carried unresolved: the publisher turns them into the
				// destination queue (tenant from the client, priority from the
				// subscription), which the pool code says nothing about.
				ClientID:       c.clientID,
				SubscriptionID: c.subID,
				// The job's OWN priority claim — takes precedence over the
				// subscription's at publish time (R4). "" when unset.
				Queue: c.queue,
			})
		}
	}

	schedMetrics.skippedHeld.Add(float64(skippedBlocked))
	if len(tokens) == 0 {
		// Nothing dispatchable: the claim is released (rolled back by the
		// deferred Rollback), every row still PENDING.
		p.warnIfStarved(len(claims), skippedBlocked, 0, len(pausedIDs))
		return len(claims), 0, nil
	}

	// Publish while the claim is still locked and uncommitted — see above.
	unpublished := p.dispatcher.PublishClaim(ctx, tokens)
	schedMetrics.unpublished.Add(float64(len(unpublished)))
	notPublished := make(map[string]struct{}, len(unpublished))
	for _, id := range unpublished {
		notPublished[id] = struct{}{}
	}
	published := make([]string, 0, len(queued))
	var minCreated, maxCreated time.Time
	for i, id := range queued {
		if _, skip := notPublished[id]; skip {
			continue
		}
		at := createdAt[i]
		if len(published) == 0 || at.Before(minCreated) {
			minCreated = at
		}
		if len(published) == 0 || at.After(maxCreated) {
			maxCreated = at
		}
		published = append(published, id)
	}
	if len(published) == 0 {
		// Nothing reached the broker; rolling back leaves every row PENDING
		// for the next poll.
		p.warnIfStarved(len(claims), skippedBlocked, len(tokens), len(pausedIDs))
		return len(claims), 0, nil
	}
	// Mark + commit run detached from ctx. The jobs are already on the broker:
	// a shutdown (or the poll deadline) cancelling ctx between the publish and
	// the commit would roll the whole claim back to PENDING and publish every
	// job a second time. Bounded separately so a stuck database still cannot
	// hang the poller.
	fctx, fcancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
	defer fcancel()
	// created_at bounds let the created_at-partitioned table prune to the
	// partitions the published rows actually span.
	if _, err := tx.Exec(fctx,
		`UPDATE msg_dispatch_jobs SET status = 'QUEUED', updated_at = NOW()
		  WHERE id = ANY($1)
		    AND created_at >= $2 AND created_at <= $3`,
		published, minCreated, maxCreated); err != nil {
		slog.Warn("marking published dispatch jobs QUEUED failed; they will be published again",
			"published", len(published), "err", err)
		return 0, 0, err
	}
	if err := tx.Commit(fctx); err != nil {
		// Published but still PENDING: the next poll publishes them again,
		// and /process delivers each once.
		slog.Warn("committing published dispatch jobs failed; they will be published again",
			"published", len(published), "err", err)
		return 0, 0, err
	}
	schedMetrics.published.Add(float64(len(published)))
	if len(queued) > 0 || skippedBlocked > 0 {
		slog.Debug("poll tick",
			"queued", len(queued),
			"skipped_blocked", skippedBlocked)
	}
	return len(claims), len(published), nil
}

// starveWarnEvery rate-limits the starvation warning.
const starveWarnEvery = time.Minute

// warnIfStarved logs when a FULL claim published nothing. That is the signature
// of starvation: the LIMIT window is filled with rows that cannot be published
// (held back behind a failed sibling, or rejected by the broker), so nothing
// behind them is ever reached, and without this it is completely silent.
// Paused-subscription rows no longer count — the claim query excludes them — so
// the paused figure is the size of the excluded set, for context. At most once
// per starveWarnEvery.
func (p *PendingJobPoller) warnIfStarved(claimed, held, publishFailed, pausedSubscriptions int) {
	if claimed < p.cfg.BatchSize || !p.starveWarnDue(time.Now()) {
		return
	}
	slog.Warn("dispatch poller claimed a full batch and published none of it; jobs behind it may be starved",
		"claimed", claimed,
		"held_skipped", held,
		"publish_failed", publishFailed,
		"paused_subscriptions_excluded", pausedSubscriptions)
}

// starveWarnDue reports whether a starvation warning may be logged at now, and
// records it when so.
func (p *PendingJobPoller) starveWarnDue(now time.Time) bool {
	if !p.starveWarnedAt.IsZero() && now.Sub(p.starveWarnedAt) < starveWarnEvery {
		return false
	}
	p.starveWarnedAt = now
	return true
}

// dispatchClaim is one PENDING row claimed by the poll query. group, subID,
// poolID and clientID are "" when the column is NULL.
//
// poolID and clientID are the INPUTS to pool-code resolution, not the code
// itself: msg_dispatch_jobs stores dispatch_pool_id, while the code lives on
// msg_dispatch_pools and the client identifier on tnt_clients. They are
// resolved through PoolCodeResolver rather than joined, so the claim's
// FOR UPDATE SKIP LOCKED keeps locking msg_dispatch_jobs alone.
type dispatchClaim struct {
	id, subID, group, mode, poolID, clientID, target, queue string
	attempt, sequence                                       int32
	createdAt                                               time.Time
}

// key is the claim's position in its group's delivery order — the same
// (sequence, created_at, id) the claim query sorts by.
func (c dispatchClaim) key() jobKey {
	return jobKey{sequence: c.sequence, createdAt: c.createdAt, id: c.id}
}

// jobKey totally orders the jobs of one message group. "Earlier" has to be a
// real comparison rather than mere set membership: a group is held back by a
// job IN FRONT of the candidate, and a group that held itself back — the
// backed-off job blocked by its own presence — would never dispatch again.
type jobKey struct {
	sequence  int32
	createdAt time.Time
	id        string
}

// before reports whether k comes before other in delivery order.
func (k jobKey) before(other jobKey) bool {
	if k.sequence != other.sequence {
		return k.sequence < other.sequence
	}
	if !k.createdAt.Equal(other.createdAt) {
		return k.createdAt.Before(other.createdAt)
	}
	return k.id < other.id
}

// messageGroupKey maps a claim's message_group to its grouping key: jobs
// without a group bucket under "default".
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

// filterByDispatchMode keeps the claims whose mode allows dispatch given
// the blocked groups. Only BLOCK_ON_ERROR — "strict FIFO; a failed job
// blocks the group until resolved" — is held back by a failed sibling.
// IMMEDIATE carries no ordering at all, and NEXT_ON_ERROR is ordered but
// explicitly "the group moves on" past a failure, so neither is blocked.
// Unknown modes parse leniently to IMMEDIATE and therefore dispatch.
// filterByDispatchMode applies the BLOCK_ON_ERROR hold-back: a job waits while
// an EARLIER job in its group is holding the group up.
//
// Holding means the earlier job has not got through and is not going to on its
// own right now — it failed, or it is sitting out a retry backoff. Both keep
// their place at the front of the group: a backed-off job is still the next
// message that must be delivered, so nothing behind it may go past while it
// waits. Delivering its successors and letting it rejoin afterwards is exactly
// the reordering BLOCK_ON_ERROR exists to prevent.
//
// The comparison is POSITIONAL, not set membership. "This group contains a
// backed-off job" would include the backed-off job itself once its backoff
// expired, and the group would never move again.
//
// IMMEDIATE and NEXT_ON_ERROR keep flowing — neither promises to stop for a
// sibling.
func filterByDispatchMode(claims []dispatchClaim, holders map[string]jobKey) []dispatchClaim {
	kept := make([]dispatchClaim, 0, len(claims))
	for _, c := range claims {
		if common.ParseDispatchMode(c.mode) == common.DispatchBlockOnError {
			if holder, ok := holders[messageGroupKey(c.group)]; ok && holder.before(c.key()) {
				continue
			}
		}
		kept = append(kept, c)
	}
	return kept
}

// blockedGroups returns the subset of candidate groups that currently
// hold a FAILED or ERROR job — one batch query per
// poll. A NULL message_group can never
// block: `= ANY` never matches NULL, so a failed ungrouped job does not
// hold back the "default" bucket. Preserve that exactly — only a row
// whose message_group is literally 'default' blocks ungrouped jobs.
func blockedGroups(ctx context.Context, tx pgx.Tx, groups []string) (map[string]jobKey, error) {
	holders := make(map[string]jobKey)
	if len(groups) == 0 {
		return holders, nil
	}
	// DISTINCT ON gives the EARLIEST holder per group, which is the only one
	// that matters: anything behind it is held by it too.
	rows, err := tx.Query(ctx,
		`SELECT DISTINCT ON (message_group) message_group, sequence, created_at, id
		   FROM msg_dispatch_jobs
		  WHERE message_group = ANY($1)
		    AND (`+dispatchjob.GroupHoldingStatusSQL+`)
		  ORDER BY message_group, sequence, created_at, id`,
		groups)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var g string
		var k jobKey
		if err := rows.Scan(&g, &k.sequence, &k.createdAt, &k.id); err != nil {
			return nil, err
		}
		holders[g] = k
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return holders, nil
}

// DispatchJobToken is the value the poller hands the dispatcher. It
// carries just enough to publish to the queue without re-reading the
// job row.
type DispatchJobToken struct {
	JobID        string
	MessageGroup string
	TargetURL    string
	// PoolCode is the RESOLVED, client-namespaced code (see PoolCodeResolver):
	// {clientIdentifier}-{poolCode} for a client-owned pool, the bare code for a
	// platform-level one, {clientIdentifier}-DEFAULT-POOL when the job has no
	// pool, and DEFAULT-POOL when it has neither. Carrying it is what puts a
	// subscription's configured concurrency and rate limit into force.
	//
	// Treat it as opaque — never split it back apart.
	PoolCode string
	// Mode is the job's raw dispatch mode. The router needs it to decide
	// whether the message group is ordered: an absent mode parses to IMMEDIATE,
	// which sends the message down the concurrent path and silently discards
	// the FIFO guarantee the group exists to provide.
	Mode string
	// ClientID and SubscriptionID are the job's own foreign keys, carried
	// unresolved for the publisher to turn into a destination queue: the
	// client gives the tenant segment, the subscription the priority. Both may
	// be empty — a client-less job publishes under the platform tenant, and a
	// job with no subscription at the DEFAULT priority.
	ClientID       string
	SubscriptionID string
	// Queue is the job's OWN stored priority claim (msg_dispatch_jobs.queue,
	// raw and unresolved, "" when unset). Takes precedence over the
	// subscription's at publish time (docs/spec/dispatch-job-priority.md R4).
	Queue string
}
