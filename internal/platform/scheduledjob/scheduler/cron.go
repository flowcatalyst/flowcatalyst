// Package scheduler is the cron-firing loop for ScheduledJob aggregates.
// Separate from internal/platform/scheduler, which schedules dispatch jobs.
//
// Mirrors fc-platform/src/scheduled_job/scheduler/. The poller wakes
// every PollInterval, queries ACTIVE jobs whose next cron slot is in
// the past, writes msg_scheduled_job_instances rows (the firing
// history), and POSTs the firing payload to the configured target URL.
// All of that is direct infrastructure-processing (no UoW per
// docs/conventions.md §3).
package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/robfig/cron/v3"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Config controls the cron-firing loop.
type Config struct {
	PollInterval time.Duration
	BatchSize    int
}

// DefaultConfig is poll-every-30s, batch=200.
func DefaultConfig() Config {
	return Config{PollInterval: 30 * time.Second, BatchSize: 200}
}

// Dispatcher is the interface the cron scheduler uses to POST the
// firing webhook. The platform server wires this against an *http.Client.
type Dispatcher interface {
	Dispatch(ctx context.Context, job *scheduledjob.ScheduledJob, instanceID string) (statusCode int, err error)
}

// CronScheduler is the cron-firing loop.
type CronScheduler struct {
	cfg        Config
	pool       *pgxpool.Pool
	jobs       *scheduledjob.Repository
	dispatcher Dispatcher

	parser cron.Parser
}

// New wires a CronScheduler.
func New(cfg Config, pool *pgxpool.Pool, jobs *scheduledjob.Repository, dispatcher Dispatcher) *CronScheduler {
	return &CronScheduler{
		cfg:        cfg,
		pool:       pool,
		jobs:       jobs,
		dispatcher: dispatcher,
		// Standard 5-field cron (minute hour dom month dow). Use
		// cron.SecondOptional or cron.Descriptor if the team wants
		// extended syntax — match what the Rust `cron` crate accepts.
		parser: cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow),
	}
}

// Run drives the loop until ctx is cancelled.
func (s *CronScheduler) Run(ctx context.Context) {
	tick := time.NewTicker(s.cfg.PollInterval)
	defer tick.Stop()
	slog.Info("cron scheduler starting", "interval", s.cfg.PollInterval)
	for {
		select {
		case <-ctx.Done():
			slog.Info("cron scheduler stopped")
			return
		case <-tick.C:
			if err := s.tick(ctx); err != nil {
				slog.Warn("cron scheduler tick error", "err", err)
			}
		}
	}
}

// tick scans ACTIVE jobs and fires any whose next cron slot has elapsed.
func (s *CronScheduler) tick(ctx context.Context) error {
	jobs, err := s.jobs.FindActive(ctx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()

	var wg sync.WaitGroup
	for i := range jobs {
		j := jobs[i]
		next := s.nextFireSlot(&j, now)
		if next.IsZero() || next.After(now) {
			continue
		}
		// Fire this slot. Concurrent fan-out across jobs is safe; per-job
		// concurrency is the SDK's responsibility (see entity.Concurrent).
		wg.Add(1)
		go func(job scheduledjob.ScheduledJob, slot time.Time) {
			defer wg.Done()
			if err := s.fire(ctx, &job, slot); err != nil {
				slog.Warn("scheduled job fire failed", "job_id", job.ID, "err", err)
			}
		}(j, next)
	}
	wg.Wait()
	return nil
}

// nextFireSlot picks the earliest scheduled slot across the job's cron
// expressions strictly after last_fired_at (or now if not yet fired).
// Returns the zero Time if no expression parses.
func (s *CronScheduler) nextFireSlot(j *scheduledjob.ScheduledJob, now time.Time) time.Time {
	loc, err := time.LoadLocation(j.Timezone)
	if err != nil {
		loc = time.UTC
	}
	after := now
	if j.LastFiredAt != nil && j.LastFiredAt.After(after) {
		after = *j.LastFiredAt
	}
	var earliest time.Time
	for _, expr := range j.Crons {
		sched, err := s.parser.Parse(expr)
		if err != nil {
			continue
		}
		next := sched.Next(after.In(loc))
		if next.IsZero() {
			continue
		}
		if earliest.IsZero() || next.Before(earliest) {
			earliest = next
		}
	}
	return earliest
}

// fire writes a msg_scheduled_job_instances row, dispatches via HTTP,
// then updates the row + the job's last_fired_at. Infrastructure path —
// no UoW.
func (s *CronScheduler) fire(ctx context.Context, j *scheduledjob.ScheduledJob, slot time.Time) error {
	instanceID := tsid.Generate(tsid.ScheduledJobInstance)
	now := time.Now().UTC()

	// 1. Pre-insert the instance row as PENDING.
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO msg_scheduled_job_instances
		     (id, scheduled_job_id, slot_at, status, attempt_count, created_at, updated_at)
		 VALUES ($1, $2, $3, 'PENDING', 0, $4, $4)`,
		instanceID, j.ID, slot, now); err != nil {
		return err
	}

	// 2. Dispatch via the configured Dispatcher (HTTP POST).
	status, dispErr := s.dispatcher.Dispatch(ctx, j, instanceID)
	finalStatus := "DELIVERED"
	errorMsg := ""
	if dispErr != nil {
		finalStatus = "DELIVERY_FAILED"
		errorMsg = dispErr.Error()
	}

	// 3. Update the instance row.
	if _, err := s.pool.Exec(ctx,
		`UPDATE msg_scheduled_job_instances
		    SET status = $1, attempt_count = 1,
		        last_status_code = $2, error_message = $3,
		        delivered_at = $4, updated_at = $4
		  WHERE id = $5`,
		finalStatus, status, nilIfEmpty(errorMsg), now, instanceID); err != nil {
		return err
	}

	// 4. Update the job's last_fired_at so we don't re-fire this slot.
	j.MarkFired(slot)
	if _, err := s.pool.Exec(ctx,
		`UPDATE msg_scheduled_jobs SET last_fired_at = $1, updated_at = $2 WHERE id = $3`,
		slot, time.Now().UTC(), j.ID); err != nil {
		return err
	}
	return nil
}

func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
