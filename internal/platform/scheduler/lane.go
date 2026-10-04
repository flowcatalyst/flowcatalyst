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

// inflightSet is the ids of the jobs handed to lanes and not yet finished. The
// poller excludes them from its next claim.
type inflightSet struct {
	mu  sync.Mutex
	ids map[string]struct{}
}

func newInflightSet() *inflightSet { return &inflightSet{ids: make(map[string]struct{})} }

func (s *inflightSet) add(ids []string) {
	s.mu.Lock()
	for _, id := range ids {
		s.ids[id] = struct{}{}
	}
	n := len(s.ids)
	s.mu.Unlock()
	schedMetrics.inflight.Set(float64(n))
}

func (s *inflightSet) remove(ids []string) {
	s.mu.Lock()
	for _, id := range ids {
		delete(s.ids, id)
	}
	n := len(s.ids)
	s.mu.Unlock()
	schedMetrics.inflight.Set(float64(n))
}

// snapshot is a copy of the set. Never nil: it goes into `<> ALL($n)`, and a
// NULL array excludes every row.
func (s *inflightSet) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.ids))
	for id := range s.ids {
		out = append(out, id)
	}
	return out
}

func (s *inflightSet) size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.ids)
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
// must not be published ahead of j. The rule:
//
//   - each lane keeps poison[g] = generation;
//   - when a batch leaves jobs of g unpublished, AFTER removing the batch's ids
//     from the in-flight set the lane reads the current claim generation P and
//     sets poison[g] = P;
//   - a job of g whose claim generation is <= poison[g] is dropped when it
//     reaches the lane (it stays PENDING and is claimed again);
//   - a claim with generation > P incremented the counter after P was read,
//     therefore snapshotted the in-flight set after j was removed, therefore
//     includes j again, in order. Such a job passes, and the first one that
//     passes clears the mark.
//
// This needs the poller to increment the generation BEFORE it snapshots the set
// (claimHeld). Ungrouped jobs are never poisoned.
//
// A dropped job is itself an unpublished job of its group: once one is dropped
// from a batch, every later job of that group in the batch is dropped as well
// (without it, a newer claim's job could pass the check while an older, dropped
// job of the same group is still waiting to be claimed again and be published
// after it), and the group is re-poisoned at the end of the batch with a
// generation read after the whole batch left the set.
type lane struct {
	p   *PendingJobPoller
	idx int
	in  chan laneJob
	// poison is touched only by the lane's own goroutine (and by tests that call
	// process directly, never concurrently with run).
	poison    map[string]poisonEntry
	lastEvict time.Time
	label     string
}

func newLane(p *PendingJobPoller, idx int) *lane {
	return &lane{
		p:      p,
		idx:    idx,
		in:     make(chan laneJob, p.cfg.BufferCapacity),
		poison: make(map[string]poisonEntry),
		label:  strconv.Itoa(idx),
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
	l.evictPoison(time.Now())

	droppedGroups := make(map[string]struct{})
	live := make([]laneJob, 0, len(batch))
	dropped := 0
	for _, j := range batch {
		if g := j.tok.MessageGroup; g != "" {
			if _, ok := droppedGroups[g]; ok {
				dropped++
				continue
			}
			if e, ok := l.poison[g]; ok {
				if j.gen <= e.gen {
					droppedGroups[g] = struct{}{}
					dropped++
					continue
				}
				delete(l.poison, g) // the first job past the mark clears it
			}
		}
		live = append(live, j)
	}
	schedMetrics.droppedPoisoned.Add(float64(dropped))

	failed := false
	poisoned := droppedGroups
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
				slog.Warn("marking published dispatch jobs QUEUED failed; they will be published again",
					"published", len(published), "err", err)
			case int(rows) < len(published):
				// The job had already moved on (delivered, or processed and
				// rescheduled) before this update ran. Left alone.
				schedMetrics.markNotUpdated.Add(float64(len(published) - int(rows)))
			}
		}
	}

	// Settle. The order is the ordering rule: ids leave the in-flight set FIRST,
	// the generation is read AFTER, the permits go back last (a released permit
	// lets the poller claim again).
	ids := make([]string, len(batch))
	for i, j := range batch {
		ids[i] = j.tok.JobID
	}
	p.inflight.remove(ids)
	if p.hookSettle != nil {
		p.hookSettle()
	}
	if len(poisoned) > 0 {
		mark := poisonEntry{gen: p.claimGeneration.Load(), at: time.Now()}
		for g := range poisoned {
			l.poison[g] = mark
		}
	}
	// Before the permits: whoever the release wakes must already see the failure.
	if failed {
		p.laneFailed.Store(true)
	}
	p.release(len(batch))
}

// evictPoison forgets the marks of groups not seen for poisonTTL, at most once a
// minute.
func (l *lane) evictPoison(now time.Time) {
	if now.Sub(l.lastEvict) < time.Minute {
		return
	}
	l.lastEvict = now
	for g, e := range l.poison {
		if now.Sub(e.at) > poisonTTL {
			delete(l.poison, g)
		}
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
