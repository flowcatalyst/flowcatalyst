package scheduler

import (
	"context"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// defaultMessageGroup is the grouping key for jobs without a
// message_group.
const defaultMessageGroup = "default"

// claimTimeout bounds one claim (the claim statement plus the hold-back lookup).
// The claim holds no locks and no transaction across a publish, so this only
// guards a stalled database. A var only so tests can shorten it.
var claimTimeout = 30 * time.Second

// finishTimeout bounds the mark-QUEUED UPDATE a lane runs after a publish. It
// runs detached from the lane's context (see lane.process).
var finishTimeout = 10 * time.Second

// releaseAttempts and releaseBackoff bound the poller-side release of claims it
// took and did not submit. Vars only so tests can shorten them.
var (
	releaseAttempts = 3
	releaseBackoff  = 200 * time.Millisecond
)

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

// PendingJobPoller is the claiming half of the dispatch scheduler: it claims
// PENDING jobs ready to dispatch, applies the pause and block-on-error
// hold-backs, and hands the rest to the lanes (lane.go), which publish them and
// mark them QUEUED. The poller never waits for a publish; it blocks only when
// BufferCapacity jobs are already in flight.
//
//	poller (leader only) --claim--> lanes[hash(group) % N] --SendMessageBatch--> broker
//	   ^  permits (BufferCapacity)        |  bulk UPDATE status='QUEUED'
//	   +----------- restored -------------+
//
// The claim reads msg_dispatch_queue (one row per PENDING job, kept exact by the
// dispatch-job lifecycle), not the job table, and DELETES the rows it takes (two
// plain statements); a claimed job is PENDING with no queue row. There is no
// transaction and no row lock around the publish. Whatever the poller or a lane
// does not publish is RESTORED (its queue row re-created from the job table) so
// the job is claimed again, in order; a job whose process died is restored by the
// leader's start-up reconcile pass and the periodic one (queue_maintenance.go). A
// double publish
// is acceptable — the router drops a copy whose original is in its pipeline and
// the delivery callback is idempotent — a reordering is not; lane.go holds the
// rule that prevents one.
type PendingJobPoller struct {
	cfg         Config
	pool        *pgxpool.Pool
	dispatcher  *MessageGroupDispatcher
	pausedCache *PausedConnectionCache
	// poolCodes composes each job's published pool code from its
	// dispatch_pool_id + client_id. Same refresh cadence as pausedCache.
	poolCodes *PoolCodeResolver
	// IsLeader gates claiming: when non-nil and false, the poller idles.
	// Within-group ordering needs a single active scheduler (the in-flight set
	// and the lanes are in-process), so concurrent claims across replicas would
	// publish a group's jobs out of order. nil = always run (standby disabled).
	// Set by Scheduler.Run.
	IsLeader func() bool

	// starveWarnedAt is when warnIfStarved last logged; only the claiming
	// goroutine touches it.
	starveWarnedAt time.Time

	// permits bounds the jobs in the engine: one is held from just before a claim
	// until the lane has finished with the job. Acquire = send, release = receive.
	permits chan struct{}
	// inflight holds the ids of every job handed to a lane and not yet finished.
	inflight *inflightSet
	// claimGeneration orders claims against failures; see lane.go.
	claimGeneration atomic.Uint64
	// rr spreads ungrouped jobs over the lanes.
	rr atomic.Uint64
	// laneFailed is set by a lane that left a job unpublished (or failed to mark
	// it); the poll loop backs off once on seeing it.
	laneFailed atomic.Bool
	lanes      []*lane

	// Seams. The defaults talk to Postgres and the dispatcher; tests replace them.
	claimRows  func(ctx context.Context, limit int, paused, heldGroups []string) ([]dispatchClaim, error)
	holdBack   func(ctx context.Context, groups []string) (map[string]jobKey, error)
	pausedIDs  func(ctx context.Context) (map[string]struct{}, error)
	poolCode   func(ctx context.Context, poolID, clientID string) string
	publish    func(ctx context.Context, toks []DispatchJobToken) (unpublished []string)
	markQueued func(ctx context.Context, ids []string, updatedAts []time.Time, minCreated, maxCreated time.Time) (int64, error)
	// restoreClaimRows puts jobs that were claimed and not published back into
	// the queue; restoreOrphans, at the start of leadership, restores every PENDING
	// job that has no queue row and is not in excluding (this process's in-flight
	// ids): the jobs of a claimer that died.
	restoreClaimRows func(ctx context.Context, refs []claimRef) error
	restoreOrphans   func(ctx context.Context, excluding []string) (int64, error)

	// claimMu is held by a claim from its first statement to the moment its jobs
	// are in the in-flight set (or restored), so the periodic reconcile — which
	// would otherwise see a job that is claimed and not yet in flight as a PENDING
	// job without a queue row — can take it to read the in-flight set and insert.
	claimMu sync.Mutex
	// held remembers the groups a claim recently found held back.
	held *heldGroups

	// hookGenSnapshot, when set, runs between a claim's generation increment and
	// its in-flight snapshot — the one window the ordering rule depends on. Tests
	// use it to land a lane's failure handling exactly there.
	hookGenSnapshot func()
	// hookReleased, when set, runs in a lane between restoring a batch's claims to
	// the queue table and removing the batch from the in-flight set — the window in
	// which a later claim can take a restored job again.
	hookReleased func()
	// hookSettle, when set, runs in a lane between removing a batch's ids from the
	// in-flight set and reading the generation for its poison marks — the window
	// the second half of the ordering rule depends on.
	hookSettle func()
}

// NewPendingJobPoller wires the poller. The pool-code resolver shares
// pausedCache's TTL: both cache slow-moving config read on every claim.
func NewPendingJobPoller(cfg Config, pool *pgxpool.Pool, dispatcher *MessageGroupDispatcher, pausedCache *PausedConnectionCache) *PendingJobPoller {
	p := newPoller(cfg)
	p.pool = pool
	p.dispatcher = dispatcher
	p.pausedCache = pausedCache
	p.poolCodes = NewPoolCodeResolver(pool, cfg.PausedCacheTTL)
	lc := dispatchjob.NewLifecycle(pool)
	p.claimRows = func(ctx context.Context, limit int, paused, held []string) ([]dispatchClaim, error) {
		return queryClaim(ctx, lc, limit, paused, held)
	}
	p.holdBack = func(ctx context.Context, groups []string) (map[string]jobKey, error) {
		return blockedGroups(ctx, pool, groups)
	}
	p.restoreClaimRows = func(ctx context.Context, refs []claimRef) error {
		ids := make([]string, len(refs))
		created := make([]time.Time, len(refs))
		for i, r := range refs {
			ids[i], created[i] = r.id, r.createdAt
		}
		_, err := lc.RestoreClaims(ctx, ids, created)
		return err
	}
	p.restoreOrphans = func(ctx context.Context, excluding []string) (int64, error) {
		return lc.RestoreOrphans(ctx, reconcileLimit, excluding)
	}
	p.pausedIDs = pausedCache.PausedSubscriptionIDs
	p.poolCode = p.poolCodes.Resolve
	p.publish = dispatcher.PublishClaim
	p.markQueued = p.updateQueued
	return p
}

// newPoller builds the engine around a normalised config, without the
// database-backed seams.
func newPoller(cfg Config) *PendingJobPoller {
	cfg = cfg.normalized()
	p := &PendingJobPoller{
		cfg:      cfg,
		permits:  make(chan struct{}, cfg.BufferCapacity),
		inflight: newInflightSet(),
		held:     newHeldGroups(heldGroupTTL, heldGroupCap),
	}
	p.lanes = make([]*lane, cfg.Dispatchers)
	for i := range p.lanes {
		p.lanes[i] = newLane(p, i)
	}
	return p
}

// batchLimit is the most rows one claim may ask for.
func (p *PendingJobPoller) batchLimit() int {
	return min(p.cfg.BatchSize, p.cfg.BufferCapacity)
}

// acquire takes one permit, blocking until one is free or ctx ends. This is the
// poller's only wait besides its sleeps: it blocks here exactly when
// BufferCapacity jobs are in the engine.
func (p *PendingJobPoller) acquire(ctx context.Context) error {
	select {
	case p.permits <- struct{}{}:
		schedMetrics.bufferInUse.Set(float64(len(p.permits)))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// tryAcquire takes a permit if one is free right now.
func (p *PendingJobPoller) tryAcquire() bool {
	select {
	case p.permits <- struct{}{}:
		return true
	default:
		return false
	}
}

// release returns n permits.
func (p *PendingJobPoller) release(n int) {
	for range n {
		<-p.permits
	}
	schedMetrics.bufferInUse.Set(float64(len(p.permits)))
}

// Run drives the poller and its lanes until ctx is cancelled, then waits for the
// lanes to finish the batch each is sending.
func (p *PendingJobPoller) Run(ctx context.Context) {
	slog.Info("dispatch job poller starting",
		"interval", p.cfg.PollInterval, "batch_size", p.cfg.BatchSize,
		"buffer_capacity", p.cfg.BufferCapacity, "dispatchers", p.cfg.Dispatchers)
	wg := p.startLanes(ctx)
	defer func() {
		wg.Wait()
		slog.Info("dispatch job poller stopped")
	}()
	wasLeader := false
	for ctx.Err() == nil {
		if p.IsLeader != nil && !p.IsLeader() {
			wasLeader = false
			sleepCtx(ctx, p.cfg.PollInterval) // only the leader claims
			continue
		}
		if !wasLeader {
			// Starting to poll as leader: a job claimed by a process that died between
			// claiming and publishing is PENDING with no queue row. Restore every such
			// job (no age guard; the in-flight set excepted) before the first claim, on
			// the claiming goroutine so none of our own claims is mid-flight.
			if err := p.recoverOrphans(ctx); err != nil {
				slog.Warn("restoring orphaned dispatch jobs at leader start failed; will retry", "err", err)
				sleepCtx(ctx, p.cfg.PollInterval)
				continue
			}
			wasLeader = true
		}
		res := p.claimOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if res.err != nil {
			slog.Warn("poll error", "err", res.err)
		}
		// Back off when looping at once would spin on the same rows: a short
		// claim (the backlog is drained), a claim that submitted nothing (all
		// held), an error, or a lane that could not publish — a failing broker
		// leaves its rows PENDING, so without this it is retried in a hot loop.
		failed := p.laneFailed.Swap(false)
		if res.err != nil || res.claimed < res.want || res.submitted == 0 || failed {
			sleepCtx(ctx, p.cfg.PollInterval)
		}
	}
}

// sleepCtx waits d or until ctx ends.
func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// claimResult is what one claim did. want is the permits it held, claimed the
// rows the query returned, submitted the jobs handed to lanes (the rest were
// held back and stay PENDING).
type claimResult struct {
	want, claimed, submitted int
	err                      error
}

// claimOnce is one pass of the poller: take permits (blocking while the buffer is
// full), claim up to that many rows, hand the dispatchable ones to the lanes,
// give back the permits it did not use.
func (p *PendingJobPoller) claimOnce(ctx context.Context) claimResult {
	if err := p.acquire(ctx); err != nil {
		return claimResult{err: err}
	}
	want := 1
	for want < p.batchLimit() && p.tryAcquire() {
		want++
	}
	start := time.Now()
	res := p.claimHeld(ctx, want)
	p.release(want - res.submitted)
	res.want = want
	if res.err == nil || ctx.Err() == nil {
		schedMetrics.observePoll(start, res.err)
	}
	return res
}

// claimHeld claims up to want rows while holding want permits; it submits the
// dispatchable ones (each keeps a permit, released by its lane), restores the
// rest (held, withheld) to the queue so they are claimed again, and leaves the
// caller to release the permits.
func (p *PendingJobPoller) claimHeld(ctx context.Context, want int) claimResult {
	if p.IsLeader != nil && !p.IsLeader() {
		return claimResult{} // leadership lost while waiting for permits
	}
	p.claimMu.Lock()
	defer p.claimMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	paused, err := p.pausedIDs(ctx)
	if err != nil {
		return claimResult{err: err}
	}
	// The paused set goes INTO the claim query (never nil: `<> ALL(NULL)` is
	// NULL, which would exclude every row).
	pausedIDs := make([]string, 0, len(paused))
	for id := range paused {
		pausedIDs = append(pausedIDs, id)
	}
	schedMetrics.pausedSubscriptions.Set(float64(len(pausedIDs)))

	// The generation is incremented BEFORE the in-flight set is snapshotted, and
	// the snapshot BEFORE the claim statement runs. That order is what the
	// ordering rule in lane.go rests on: a lane that fails a job releases its
	// claim, then removes it from the set, then reads the counter; a claim whose
	// generation exceeds that reading started after the release, so it returns
	// the job again, in order — and a claim that skipped the job (still claimed)
	// started before the reading, so its generation is at or below the poison mark.
	gen := p.claimGeneration.Add(1)
	if p.hookGenSnapshot != nil {
		p.hookGenSnapshot()
	}
	snap := p.inflight.snapshot()

	claimStart := time.Now()
	claims, err := p.claimRows(ctx, want, pausedIDs, p.held.active(time.Now()))
	schedMetrics.claimDuration.Observe(time.Since(claimStart).Seconds())
	if err != nil {
		return claimResult{err: err}
	}
	if len(claims) == 0 {
		return claimResult{}
	}
	schedMetrics.claimed.Add(float64(len(claims)))
	if len(claims) >= want {
		schedMetrics.fullBatches.Inc()
	}

	// A claim can return a job this process still has in flight: it was restored
	// or refreshed after the copy in flight was claimed. Leave that copy alone.
	var inFlightAgain []dispatchClaim
	fresh := claims[:0:0]
	for _, c := range claims {
		if p.inflight.contains(c.id) {
			inFlightAgain = append(inFlightAgain, c)
			continue
		}
		fresh = append(fresh, c)
	}
	if len(inFlightAgain) > 0 {
		p.restoreClaims(ctx, claimRefs(inFlightAgain))
		claims = fresh
		if len(claims) == 0 {
			return claimResult{}
		}
	}

	// What stays in Go is the blocked-group / BLOCK_ON_ERROR hold-back, which is
	// positional (a job waits behind an EARLIER failed sibling) and so stays out
	// of SQL. Held rows are not submitted and stay PENDING; the next claim
	// retries them. Paused subscriptions are excluded by the claim itself, so
	// they can never fill the LIMIT window and starve the rows behind them.
	byGroup := groupByMessageGroup(claims)
	candidates := make([]string, 0, len(byGroup))
	for g := range byGroup {
		candidates = append(candidates, g)
	}
	blocked, err := p.holdBack(ctx, candidates)
	if err != nil {
		p.restoreClaims(ctx, claimRefs(claims)) // none of them will be submitted
		return claimResult{claimed: len(claims), err: err}
	}
	// A FAILED/ERROR sibling (or one sitting out a retry backoff) holds back this
	// group's BLOCK_ON_ERROR jobs; IMMEDIATE and NEXT_ON_ERROR keep flowing.
	dispatchable := filterByDispatchMode(claims, blocked)
	// Remember the groups just found held, so the next claims skip them instead of
	// filling their batch with the same held rows over and over.
	if len(dispatchable) < len(claims) {
		kept := make(map[string]struct{}, len(dispatchable))
		for _, c := range dispatchable {
			kept[c.id] = struct{}{}
		}
		now := time.Now()
		for _, c := range claims {
			if _, ok := kept[c.id]; !ok && c.group != "" {
				p.held.add(c.group, now)
			}
		}
	}
	schedMetrics.heldGroups.Set(float64(p.held.size()))
	// A claim that excluded a doomed in-flight job of a group must not submit the
	// group's jobs: they are behind it (see lane.go). They stay PENDING.
	withheld := 0
	submit := dispatchable[:0:0]
	for _, c := range dispatchable {
		if c.group != "" && p.inflight.skippedADoomedJob(snap, c.group) {
			withheld++
			continue
		}
		submit = append(submit, c)
	}
	// Everything claimed and not submitted — held back, or withheld — goes back
	// into the queue now, before the next claim: it is claimed again, in order.
	if len(submit) < len(claims) {
		sent := make(map[string]struct{}, len(submit))
		for _, c := range submit {
			sent[c.id] = struct{}{}
		}
		var back []dispatchClaim
		for _, c := range claims {
			if _, ok := sent[c.id]; !ok {
				back = append(back, c)
			}
		}
		p.restoreClaims(ctx, claimRefs(back))
	}
	dispatchable = submit
	schedMetrics.withheldDoomed.Add(float64(withheld))
	held := len(claims) - len(dispatchable) - withheld
	schedMetrics.skippedHeld.Add(float64(held))
	if held > 0 {
		slog.Debug("message group blocked, holding ordered jobs", "held", held, "dispatching", len(dispatchable))
	}
	if len(dispatchable) == 0 {
		p.warnIfStarved(len(claims), want, held, len(pausedIDs))
		return claimResult{claimed: len(claims)}
	}

	jobs := make([]laneJob, len(dispatchable))
	for i, c := range dispatchable {
		jobs[i] = laneJob{
			gen:       gen,
			createdAt: c.createdAt,
			updatedAt: c.updatedAt,
			tok: DispatchJobToken{
				JobID:        c.id,
				MessageGroup: c.group,
				Mode:         c.mode,
				PoolCode:     p.poolCode(ctx, c.poolID, c.clientID),
				// Carried unresolved: the publisher turns them into the
				// destination queue (tenant from the client, priority from the
				// subscription), which the pool code says nothing about.
				ClientID:       c.clientID,
				SubscriptionID: c.subID,
				// The job's OWN priority claim — takes precedence over the
				// subscription's at publish time (R4). "" when unset.
				Queue: c.queue,
			},
		}
	}
	// In the set BEFORE the first send: a lane may finish a job (and remove it)
	// the instant it receives it.
	p.inflight.add(jobs)
	for _, j := range jobs {
		// Never blocks: the permits bound the jobs in the engine to
		// BufferCapacity, which is every lane channel's capacity.
		p.lanes[p.laneFor(j.tok.MessageGroup)].in <- j
	}
	return claimResult{claimed: len(claims), submitted: len(jobs)}
}

// laneFor picks the lane for a message group: a stable hash, so one group is
// always published by one lane, in claim order. Ungrouped jobs carry no ordering
// and are spread round-robin.
func (p *PendingJobPoller) laneFor(group string) int {
	if group == "" {
		return int(p.rr.Add(1) % uint64(len(p.lanes)))
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(group))
	return int(h.Sum32() % uint32(len(p.lanes)))
}

// queryClaim is the default claimRows: the lifecycle's claim of the queue table
// (one statement), in delivery order.
func queryClaim(ctx context.Context, lc *dispatchjob.Lifecycle, limit int, paused, held []string) ([]dispatchClaim, error) {
	rows, err := lc.ClaimQueue(ctx, limit, paused, held)
	if err != nil {
		return nil, err
	}
	claims := make([]dispatchClaim, len(rows))
	for i, r := range rows {
		c := dispatchClaim{
			id: r.JobID, mode: r.Mode, sequence: r.Sequence,
			createdAt: r.JobCreatedAt, updatedAt: r.Version,
		}
		if r.SubscriptionID != nil {
			c.subID = *r.SubscriptionID
		}
		if r.MessageGroup != nil {
			c.group = *r.MessageGroup
		}
		if r.DispatchPoolID != nil {
			c.poolID = *r.DispatchPoolID
		}
		if r.ClientID != nil {
			c.clientID = *r.ClientID
		}
		if r.Queue != nil {
			c.queue = *r.Queue
		}
		claims[i] = c
	}
	return claims, nil
}

// claimRef names a claimed job: its id and created_at (which lets the
// created_at-partitioned job table prune on the restore).
type claimRef struct {
	id        string
	createdAt time.Time
}

func claimRefs(claims []dispatchClaim) []claimRef {
	refs := make([]claimRef, len(claims))
	for i, c := range claims {
		refs[i] = claimRef{c.id, c.createdAt}
	}
	return refs
}

// restoreClaims puts jobs this poller claimed and did not submit back into the
// queue. It retries a few times; a job that cannot be restored stays PENDING
// without a queue row until the periodic reconcile (age guard 60s) re-creates it.
// Detached from ctx: a shutdown must not strand jobs.
func (p *PendingJobPoller) restoreClaims(ctx context.Context, refs []claimRef) {
	if len(refs) == 0 {
		return
	}
	ctx = context.WithoutCancel(ctx)
	var err error
	for attempt := range releaseAttempts {
		rctx, cancel := context.WithTimeout(ctx, finishTimeout)
		err = p.restoreClaimRows(rctx, refs)
		cancel()
		if err == nil {
			return
		}
		time.Sleep(releaseBackoff * time.Duration(attempt+1))
	}
	schedMetrics.claimRestoreFailures.Add(float64(len(refs)))
	slog.Warn("restoring the unsubmitted dispatch jobs failed; the reconcile sweep will re-queue them",
		"jobs", len(refs), "err", err)
}

// recoverOrphans is the start-of-leadership pass: restore the queue rows of the
// jobs a dead claimer left PENDING with none.
func (p *PendingJobPoller) recoverOrphans(ctx context.Context) error {
	n, err := p.restoreOrphans(ctx, p.inflight.ids())
	if err != nil {
		return err
	}
	if n > 0 {
		slog.Warn("restored dispatch jobs left without a queue row by a previous process", "count", n)
	}
	return nil
}

// updateQueued is the default markQueued: one bulk UPDATE (the dispatch-job
// lifecycle's MarkQueued) on the scheduler's own pooled connection, no transaction. It is optimistic on the row version the claim
// read: a row is updated only if it is still PENDING AND its updated_at is the
// one the claim saw. The status guard alone is not enough — the router can
// deliver, and the callback process the job and reschedule it back to PENDING
// (retry, deferral, BLOCK_ON_ERROR hold), before this runs; setting QUEUED then
// would strand a job that has no message in the queue until the stale sweep.
// Every status transition on the table stamps updated_at, so any move at all
// changes the version. created_at bounds let the created_at-partitioned table
// prune to the partitions the rows span.
func (p *PendingJobPoller) updateQueued(ctx context.Context, ids []string, updatedAts []time.Time, minCreated, maxCreated time.Time) (int64, error) {
	return dispatchjob.NewLifecycle(p.pool).MarkQueued(ctx, ids, updatedAts, minCreated, maxCreated)
}

// starveWarnEvery rate-limits the starvation warning.
const starveWarnEvery = time.Minute

// warnIfStarved logs when a FULL claim submitted nothing. That is the signature
// of starvation: the LIMIT window is filled with rows that are held back behind
// a failed sibling, so nothing behind them is ever reached, and without this it
// is completely silent.
// Paused-subscription rows no longer count — the claim query excludes them — so
// the paused figure is the size of the excluded set, for context. At most once
// per starveWarnEvery.
func (p *PendingJobPoller) warnIfStarved(claimed, want, held, pausedSubscriptions int) {
	if claimed < want || !p.starveWarnDue(time.Now()) {
		return
	}
	slog.Warn("dispatch poller claimed a full batch and submitted none of it; jobs behind it may be starved",
		"claimed", claimed,
		"held_skipped", held,
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

// dispatchClaim is one queue row claimed by the poll query. group, subID,
// poolID and clientID are "" when the column is NULL. createdAt is the job's
// created_at and updatedAt the queue row's version (the job's updated_at when
// the row was written).
//
// poolID and clientID are the INPUTS to pool-code resolution, not the code
// itself: the queue row carries dispatch_pool_id, while the code lives on
// msg_dispatch_pools and the client identifier on tnt_clients. They are
// resolved through PoolCodeResolver rather than joined, so the claim stays a
// read of msg_dispatch_queue alone.
type dispatchClaim struct {
	id, subID, group, mode, poolID, clientID, queue string
	sequence                                        int32
	createdAt, updatedAt                            time.Time
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

// pgxQuerier is the one method blockedGroups needs; a pool or a transaction.
type pgxQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// blockedGroups returns, for each candidate group, the EARLIEST job holding it
// — one batch query per poll (dispatchjob.GroupHoldersSQL): a FAILED/ERROR job
// from msg_dispatch_jobs, or a PENDING job with a future scheduled_for from
// msg_dispatch_queue. A NULL message_group can never block: `= ANY` never
// matches NULL, so a failed ungrouped job does not hold back the "default"
// bucket. Preserve that exactly — only a row whose message_group is literally
// 'default' blocks ungrouped jobs.
func blockedGroups(ctx context.Context, q pgxQuerier, groups []string) (map[string]jobKey, error) {
	holders := make(map[string]jobKey)
	if len(groups) == 0 {
		return holders, nil
	}
	rows, err := q.Query(ctx, dispatchjob.GroupHoldersSQL, dispatchjob.GroupHoldingStatuses, groups)
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

// Held-group memory (claimHeld): groups a claim just found held are skipped by the
// next claims' walk for heldGroupTTL, so a batch-full of held rows at the head of
// the order does not fill every claim and starve the groups behind it.
var (
	heldGroupTTL = 5 * time.Second
	heldGroupCap = 10_000
)

type heldGroups struct {
	mu     sync.Mutex
	ttl    time.Duration
	cap    int
	expiry map[string]time.Time
}

func newHeldGroups(ttl time.Duration, capacity int) *heldGroups {
	return &heldGroups{ttl: ttl, cap: capacity, expiry: make(map[string]time.Time)}
}

// add remembers group as held from now; over the cap the oldest entry is dropped.
func (h *heldGroups) add(group string, now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.expiry[group] = now.Add(h.ttl)
	for len(h.expiry) > h.cap {
		var oldest string
		var at time.Time
		for g, e := range h.expiry {
			if oldest == "" || e.Before(at) {
				oldest, at = g, e
			}
		}
		delete(h.expiry, oldest)
	}
}

// active lists the groups still remembered at now (never nil), forgetting the rest.
func (h *heldGroups) active(now time.Time) []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, 0, len(h.expiry))
	for g, e := range h.expiry {
		if !e.After(now) {
			delete(h.expiry, g)
			continue
		}
		out = append(out, g)
	}
	return out
}

func (h *heldGroups) size() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.expiry)
}
