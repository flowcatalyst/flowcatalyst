package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// tickRetention is how long cron-tick claims are kept before being pruned.
const tickRetention = 30 * 24 * time.Hour

// minWake floors the poller's sleep so a tight schedule cannot spin it.
const minWake = 200 * time.Millisecond

// poller scans ACTIVE jobs and inserts a QUEUED instance for the latest due
// cron slot per job, then sleeps until the next due slot across all jobs
// (bounded by PollInterval, which is how quickly new/edited jobs are noticed).
type poller struct {
	cfg       Config
	jobs      *scheduledjob.Repository
	instances *scheduledjob.InstanceRepository
	isLeader  func() bool
	// wake nudges the dispatcher after a new instance is queued so a firing
	// is delivered at its slot rather than up to DispatchInterval later.
	wake chan<- struct{}

	now       func() time.Time // test seam; nil = time.Now
	lastPrune time.Time
	warned    map[string]bool // job_id|cron already warned about as unparseable
}

func (p *poller) run(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("scheduled-job poller stopped")
			return
		case <-timer.C:
		}
		wait := p.cfg.PollInterval
		if p.isLeader() { // only the leader polls; the tick claim in the DB backstops it
			next, err := p.tick(ctx)
			if err != nil {
				slog.Warn("scheduled-job poller tick error", "err", err)
			} else if !next.IsZero() {
				wait = min(max(time.Until(next), minWake), p.cfg.PollInterval)
			}
		}
		timer.Reset(wait)
	}
}

// tick scans ACTIVE jobs, computes the latest due slot per job, and inserts a
// QUEUED instance for it. Window is (last_fired ?? created_at, now]; only the
// latest slot fires (skip-missed). Each slot is claimed in the database
// (InsertCronTick), so it fires at most once even with two pollers. Returns the
// earliest upcoming slot across all jobs (zero when there is none).
func (p *poller) tick(ctx context.Context) (time.Time, error) {
	jobs, err := p.jobs.FindActive(ctx)
	if err != nil {
		return time.Time{}, err
	}
	now := time.Now().UTC()
	if p.now != nil {
		now = p.now().UTC()
	}
	var next time.Time
	for i := range jobs {
		j := &jobs[i]
		p.warnUnparseable(j)
		after := j.CreatedAt
		if j.LastFiredAt != nil {
			after = *j.LastFiredAt
		}
		if slot, ok := scheduledjob.LatestSlotInWindow(j.Crons, j.Timezone, after, now); ok {
			p.fire(ctx, j, slot, now)
		}
		if n, ok := scheduledjob.NextSlotAfter(j.Crons, j.Timezone, now); ok && (next.IsZero() || n.Before(next)) {
			next = n
		}
	}
	p.prune(ctx, now)
	if !next.IsZero() {
		next = next.Add(20 * time.Millisecond) // land just after the slot, not just before
	}
	return next, nil
}

func (p *poller) fire(ctx context.Context, j *scheduledjob.ScheduledJob, slot, now time.Time) {
	// The job's Concurrent flag is deliberately NOT gated here: overlapping
	// runs are the receiving service's concern (see ScheduledJob.Concurrent).
	inst := &scheduledjob.ScheduledJobInstance{
		ID:               tsid.Generate(tsid.ScheduledJobInstance),
		ScheduledJobID:   j.ID,
		ClientID:         ids.StringPtr(j.ClientID),
		JobCode:          j.Code,
		TriggerKind:      scheduledjob.TriggerCron,
		ScheduledFor:     &slot,
		FiredAt:          now,
		Status:           scheduledjob.InstanceStatusQueued,
		DeliveryAttempts: 0,
		CreatedAt:        now,
	}
	inserted, err := p.instances.InsertCronTick(ctx, inst)
	if err != nil {
		slog.Warn("scheduled-job poller: insert instance failed", "job_id", j.ID, "err", err)
		return
	}
	if !inserted {
		slog.Info("scheduled-job poller: tick already claimed; not firing again",
			"job_id", j.ID, "job_code", j.Code, "scheduled_for", slot)
	}
	// Advance last_fired_at monotonically so the next tick's window opens
	// strictly after this slot (also when another poller claimed the tick).
	if err := p.jobs.MarkFired(ctx, j.ID, slot); err != nil {
		slog.Warn("scheduled-job poller: mark_fired failed", "job_id", j.ID, "err", err)
	}
	if inserted && p.wake != nil {
		select {
		case p.wake <- struct{}{}:
		default:
		}
	}
}

// warnUnparseable logs (once per process) a stored cron expression that can
// never fire — a legacy 7-field expression, say — so it isn't silently dead.
func (p *poller) warnUnparseable(j *scheduledjob.ScheduledJob) {
	for _, c := range j.Crons {
		if scheduledjob.ValidateCron(c) == nil {
			continue
		}
		key := j.ID + "|" + c
		if p.warned == nil {
			p.warned = make(map[string]bool)
		}
		if !p.warned[key] {
			p.warned[key] = true
			slog.Warn("scheduled job has a cron expression that can never fire (must be 6-field, seconds first); edit the job",
				"job_id", j.ID, "job_code", j.Code, "cron", c)
		}
	}
}

func (p *poller) prune(ctx context.Context, now time.Time) {
	if now.Sub(p.lastPrune) < time.Hour {
		return
	}
	p.lastPrune = now
	if _, err := p.instances.PruneTicks(ctx, now.Add(-tickRetention)); err != nil {
		slog.Warn("scheduled-job poller: prune ticks failed", "err", err)
	}
}
