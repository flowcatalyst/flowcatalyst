package scheduler

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

// Lane tuning. Vars only so tests can shorten them.
var (
	// shutdownGrace is how long a lane's in-progress publish may run on after
	// shutdown begins. The batch it is sending is finished (and marked) rather
	// than abandoned, which would publish it a second time at the next start; a
	// broker that has stopped answering is cut off here.
	shutdownGrace = 10 * time.Second
	// publishTimeout bounds one lane batch's publish while running. The SQS
	// publisher also bounds each call it makes.
	publishTimeout = 2 * time.Minute
	// poisonTTL is how long a group's poison mark survives without the group
	// being seen again.
	poisonTTL = 10 * time.Minute
)

// laneJob is one claimed job on its way to a lane: the token to publish and the
// generation of the claim that produced it.
type laneJob struct {
	tok       DispatchJobToken
	gen       uint64
	createdAt time.Time
	// updatedAt is the row version the claim read; the QUEUED update is
	// optimistic on it.
	updatedAt time.Time
}

// inflightEntry is what the engine remembers of an in-flight job: its group and
// the generation of the claim that took it.
type inflightEntry struct {
	group string
	gen   uint64
}

// inflightSnapshot is the in-flight set as one claim saw it.
type inflightSnapshot struct {
	// minGen is, per group, the oldest generation among its in-flight jobs.
	minGen map[string]uint64
}

// inflightSet is the state the poller and the lanes share: the in-flight jobs
// (claimed in the queue table and not yet finished by a lane; stale-claim sweeps
// leave them alone) and the poison marks. One mutex guards both, so a claim's
// check against a poison mark sees a consistent view.
type inflightSet struct {
	mu        sync.Mutex
	jobs      map[string]inflightEntry
	poison    map[string]poisonEntry
	lastEvict time.Time
}

func newInflightSet() *inflightSet {
	return &inflightSet{jobs: make(map[string]inflightEntry), poison: make(map[string]poisonEntry)}
}

func (s *inflightSet) add(jobs []laneJob) {
	s.mu.Lock()
	for _, j := range jobs {
		s.jobs[j.tok.JobID] = inflightEntry{group: j.tok.MessageGroup, gen: j.gen}
	}
	n := len(s.jobs)
	s.mu.Unlock()
	schedMetrics.inflight.Set(float64(n))
}

func (s *inflightSet) remove(ids []string) {
	s.mu.Lock()
	for _, id := range ids {
		delete(s.jobs, id)
	}
	n := len(s.jobs)
	s.mu.Unlock()
	schedMetrics.inflight.Set(float64(n))
}

// ids lists the in-flight job ids. Never nil: it goes into `<> ALL($n)`.
func (s *inflightSet) ids() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.jobs))
	for id := range s.jobs {
		out = append(out, id)
	}
	return out
}

// snapshot copies what a claim's doomed check needs from the set.
func (s *inflightSet) snapshot() inflightSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap := inflightSnapshot{minGen: make(map[string]uint64)}
	for _, e := range s.jobs {
		if e.group == "" {
			continue
		}
		if m, ok := snap.minGen[e.group]; !ok || e.gen < m {
			snap.minGen[e.group] = e.gen
		}
	}
	return snap
}

func (s *inflightSet) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.jobs)
}

// skippedADoomedJob reports whether a claim that took snap must not submit its
// jobs of group: the snapshot held a job of the group that is doomed (its
// generation is at or below the group's poison mark, so a lane will drop it),
// and the claim excluded it — so any job of the group it returned is BEHIND the
// doomed one and would be published ahead of it.
func (s *inflightSet) skippedADoomedJob(snap inflightSnapshot, group string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, poisoned := s.poison[group]
	min, inflight := snap.minGen[group]
	return poisoned && inflight && min <= p.gen
}

// admit is called when a job of group with generation gen reaches a lane:
// false when it is poisoned and must be dropped. The first job past the mark
// clears it. A drop does NOT renew the mark.
func (s *inflightSet) admit(group string, gen uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.poison[group]
	switch {
	case !ok:
		return true
	case gen <= p.gen:
		return false
	}
	delete(s.poison, group)
	return true
}

func (s *inflightSet) poisonGroups(groups map[string]struct{}, gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	for g := range groups {
		s.poison[g] = poisonEntry{gen: gen, at: now}
	}
}

// evictPoison forgets the marks of groups not seen for poisonTTL, at most once a
// minute.
func (s *inflightSet) evictPoison(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if now.Sub(s.lastEvict) < time.Minute {
		return
	}
	s.lastEvict = now
	for g, e := range s.poison {
		if now.Sub(e.at) > poisonTTL {
			delete(s.poison, g)
		}
	}
}

// poisonEntry marks a group whose jobs of generation <= gen must not be
// published.
type poisonEntry struct {
	gen uint64
	at  time.Time
}

// lane is one dispatcher: it publishes the jobs of the groups hashed to it, in
// the order the poller sent them, and marks what it published QUEUED. A group
// lives in exactly one lane and a lane publishes in claim order, so order holds
// while everything succeeds.
//
// # Ordering under failure
//
// When job j of group g is not published, later jobs of g are already claimed —
// in this lane's channel, or in a claim the poller is running right now — and
// must not be published ahead of j. The rule (state in inflightSet):
//
//   - when a batch leaves jobs of g unpublished, the lane first RELEASES the
//     batch's unpublished claims in the queue table (claimed_at cleared), then
//     removes the batch's ids from the in-flight set, and only then reads the
//     current claim generation P and sets poison[g] = P;
//   - a job of g whose claim generation is <= poison[g] is dropped when it
//     reaches the lane (it stays PENDING, its claim is released, and it is
//     claimed again);
//   - a claim with generation > P incremented the counter after P was read,
//     therefore ran its statement after j's claim was released, therefore
//     returns j again, in order. Such a job passes, and the first one that
//     passes clears the mark. A claim that skipped j (still claimed) ran its
//     statement before the release, hence before P was read, hence its
//     generation is <= P and its jobs of g are dropped.
//
// This needs the poller to increment the generation BEFORE it runs the claim
// (claimHeld). Ungrouped jobs are never poisoned.
//
// # The claim must not skip a doomed job
//
// A job of g still waiting in a lane when g is poisoned is doomed — it will be
// dropped — yet it is still claimed in the queue table, so a later claim skips
// it and, if that claim is newer than the poison, takes the job BEHIND it, which
// would then be published ahead of the doomed one. So the poller checks every
// claim against the in-flight snapshot it took: if the snapshot held a doomed
// job of g, the claim's jobs of g are not submitted; their claims are released
// and they are claimed again once the doomed job has gone
// (inflightSet.skippedADoomedJob).
//
// A drop does NOT renew the poison. Doing so (poisoning again at the generation
// read after the drop) also closes that hole, but livelocks whenever the poller
// claims faster than a lane drains: every claim made while a batch is being
// dropped is older than that batch's renewed mark, so it is dropped in turn,
// without end. Within one batch, once a job of g is dropped the later jobs of g
// in that batch are dropped with it.
type lane struct {
	p     *PendingJobPoller
	idx   int
	in    chan laneJob
	label string
}

func newLane(p *PendingJobPoller, idx int) *lane {
	return &lane{
		p:     p,
		idx:   idx,
		in:    make(chan laneJob, p.cfg.BufferCapacity),
		label: strconv.Itoa(idx),
	}
}

// startLanes launches every lane; the returned group finishes when they have all
// exited (ctx cancelled and each finished the batch it was sending).
func (p *PendingJobPoller) startLanes(ctx context.Context) *sync.WaitGroup {
	var wg sync.WaitGroup
	for _, l := range p.lanes {
		wg.Go(func() { l.run(ctx) })
	}
	return &wg
}

// run receives one job (blocking), drains up to LaneBatch-1 more without
// blocking, and processes them as one batch. On shutdown whatever is still
// buffered is left PENDING.
func (l *lane) run(ctx context.Context) {
	batch := make([]laneJob, 0, l.p.cfg.LaneBatch)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case j := <-l.in:
			batch = append(batch[:0], j)
		}
	drain:
		for len(batch) < l.p.cfg.LaneBatch {
			select {
			case j := <-l.in:
				batch = append(batch, j)
			default:
				break drain
			}
		}
		l.process(ctx, batch)
	}
}

// process handles one batch: drop what the ordering rule drops, publish the
// rest, mark the published QUEUED, then settle the in-flight set, the poison
// marks and the permits — in that order.
func (l *lane) process(ctx context.Context, batch []laneJob) {
	p := l.p
	p.inflight.evictPoison(time.Now())

	droppedGroups := make(map[string]struct{})
	live := make([]laneJob, 0, len(batch))
	dropped := 0
	// toRelease collects the claims to give back to the queue: dropped,
	// unpublished, and published-but-not-marked jobs.
	var toRelease []string
	for _, j := range batch {
		if g := j.tok.MessageGroup; g != "" {
			if _, ok := droppedGroups[g]; ok {
				dropped++
				toRelease = append(toRelease, j.tok.JobID)
				continue
			}
			if !p.inflight.admit(g, j.gen) {
				// The mark already stands and a drop does not renew it; the
				// rest of the group in this batch follows the dropped job.
				droppedGroups[g] = struct{}{}
				dropped++
				toRelease = append(toRelease, j.tok.JobID)
				continue
			}
		}
		live = append(live, j)
	}
	schedMetrics.droppedPoisoned.Add(float64(dropped))

	failed := false
	poisoned := make(map[string]struct{})
	if len(live) > 0 {
		toks := make([]DispatchJobToken, len(live))
		for i, j := range live {
			toks[i] = j.tok
		}
		pctx, cancel := publishContext(ctx)
		start := time.Now()
		unpublished := p.publish(pctx, toks)
		cancel()
		schedMetrics.lanePublish.WithLabelValues(l.label).Observe(time.Since(start).Seconds())

		notPublished := make(map[string]struct{}, len(unpublished))
		for _, id := range unpublished {
			notPublished[id] = struct{}{}
		}
		published := make([]string, 0, len(live))
		publishedVersions := make([]time.Time, 0, len(live))
		var minCreated, maxCreated time.Time
		for _, j := range live {
			if _, bad := notPublished[j.tok.JobID]; bad {
				if g := j.tok.MessageGroup; g != "" {
					poisoned[g] = struct{}{}
				}
				toRelease = append(toRelease, j.tok.JobID)
				continue
			}
			if len(published) == 0 || j.createdAt.Before(minCreated) {
				minCreated = j.createdAt
			}
			if len(published) == 0 || j.createdAt.After(maxCreated) {
				maxCreated = j.createdAt
			}
			published = append(published, j.tok.JobID)
			publishedVersions = append(publishedVersions, j.updatedAt)
		}
		if len(notPublished) > 0 {
			failed = true
			schedMetrics.unpublished.Add(float64(len(notPublished)))
		}
		if len(published) > 0 {
			schedMetrics.published.Add(float64(len(published)))
			// Detached from ctx: the jobs are on the broker, and a shutdown
			// abandoning the update would publish every one of them again.
			mctx, mcancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
			rows, err := p.markQueued(mctx, published, publishedVersions, minCreated, maxCreated)
			mcancel()
			switch {
			case err != nil:
				failed = true
				toRelease = append(toRelease, published...)
				slog.Warn("marking published dispatch jobs QUEUED failed; they will be published again",
					"published", len(published), "err", err)
			case int(rows) < len(published):
				// The job had already moved on (delivered, or processed and
				// rescheduled) before this update ran. Left alone.
				schedMetrics.markNotUpdated.Add(float64(len(published) - int(rows)))
			}
		}
	}

	// Settle. The order is the ordering rule: the claims of what was not
	// published go back to the queue FIRST, the ids leave the in-flight set next,
	// the generation is read AFTER, the permits go back last (a released permit
	// lets the poller claim again).
	l.releaseUntilDone(ctx, toRelease)
	ids := make([]string, len(batch))
	for i, j := range batch {
		ids[i] = j.tok.JobID
	}
	p.inflight.remove(ids)
	if p.hookSettle != nil {
		p.hookSettle()
	}
	if len(poisoned) > 0 {
		p.inflight.poisonGroups(poisoned, p.claimGeneration.Load())
	}
	// Before the permits: whoever the release wakes must already see the failure.
	if failed {
		p.laneFailed.Store(true)
	}
	p.release(len(batch))
}

// releaseUntilDone gives the claims of ids back to the queue, retrying until it
// succeeds or ctx ends. It must not give up while the process runs: a claim left
// in place while the lane lets the job leave the in-flight set would let the
// group's later jobs be claimed, published and delivered ahead of it. Blocking
// here blocks only this lane (the groups hashed to it) and, through the permits,
// the poller — which is right, since the database is not answering. On shutdown
// it returns; the next leader's start-up pass releases the claim.
func (l *lane) releaseUntilDone(ctx context.Context, ids []string) {
	if len(ids) == 0 {
		return
	}
	backoff := 50 * time.Millisecond
	for {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishTimeout)
		err := l.p.releaseClaimRows(rctx, ids)
		cancel()
		if err == nil {
			return
		}
		schedMetrics.claimReleaseFailures.Add(float64(len(ids)))
		slog.Warn("releasing the claims of unpublished dispatch jobs failed; retrying", "jobs", len(ids), "err", err)
		if ctx.Err() != nil {
			return
		}
		sleepCtx(ctx, backoff)
		if ctx.Err() != nil {
			return
		}
		backoff = min(backoff*2, 5*time.Second)
	}
}

// publishContext is the context a lane publishes under: detached from ctx, so a
// shutdown does not abort the batch being sent, but cut off shutdownGrace after
// ctx ends (or publishTimeout after the start) so a dead broker cannot hold the
// process.
func publishContext(ctx context.Context) (context.Context, context.CancelFunc) {
	pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	var mu sync.Mutex
	var timer *time.Timer
	stop := context.AfterFunc(ctx, func() {
		mu.Lock()
		defer mu.Unlock()
		timer = time.AfterFunc(shutdownGrace, cancel)
	})
	return pctx, func() {
		stop()
		mu.Lock()
		if timer != nil {
			timer.Stop()
		}
		mu.Unlock()
		cancel()
	}
}
