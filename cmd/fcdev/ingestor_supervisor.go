package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ingProc is a running child process, as the supervisor sees it (a fake in
// tests).
type ingProc interface {
	Pid() int
	Done() <-chan struct{} // closed when the process has exited
	Terminate() error      // ask it to stop (SIGTERM)
	Kill() error           // end it now
}

// errIngestorGaveUp is returned when the child kept exiting past the cap.
var errIngestorGaveUp = errors.New("the ingestor runtime kept exiting; giving up")

// ingSupervisor keeps one child running: it restarts it with a growing
// backoff when it exits unexpectedly, up to a cap, and stops it (terminate,
// then kill after a grace period) when ctx ends.
type ingSupervisor struct {
	launch      func() (ingProc, error)
	onStarted   func(ctx context.Context, p ingProc) // optional; runs in a goroutine per start
	maxRestarts int
	backoff     func(restart int) time.Duration // restart is 1-based
	stableAfter time.Duration                   // a run this long resets the restart count
	grace       time.Duration
	log         *slog.Logger
	now         func() time.Time
	// exitCause, when set, says why the child exited (from its log), so the
	// developer sees it where fcdev reports the exit. Empty when unknown.
	exitCause func() string
}

func defaultIngestorBackoff(restart int) time.Duration {
	d := time.Duration(1<<min(restart-1, 5)) * 2 * time.Second // 2s, 4s, 8s, 16s, 32s, 64s
	return min(d, time.Minute)
}

func (s *ingSupervisor) run(ctx context.Context) error {
	if s.now == nil {
		s.now = time.Now
	}
	restarts := 0
	for {
		started := s.now()
		p, err := s.launch()
		if err == nil {
			s.log.Info("ingestor runtime started", "pid", p.Pid())
			runCtx, stopProbe := context.WithCancel(ctx)
			if s.onStarted != nil {
				go s.onStarted(runCtx, p)
			}
			select {
			case <-ctx.Done():
				stopProbe()
				s.stop(p)
				return nil
			case <-p.Done():
				stopProbe()
			}
			attrs := []any{"pid", p.Pid(), "after", s.now().Sub(started).Round(time.Second).String()}
			if s.exitCause != nil {
				if cause := s.exitCause(); cause != "" {
					attrs = append(attrs, "cause", cause)
				}
			}
			s.log.Warn("ingestor runtime exited", attrs...)
		} else {
			if ctx.Err() != nil {
				return nil
			}
			s.log.Warn("ingestor runtime did not start", "err", err)
		}

		if s.now().Sub(started) >= s.stableAfter {
			restarts = 0
		}
		restarts++
		if restarts > s.maxRestarts {
			s.log.Error("ingestor runtime kept exiting; not restarting it again (see its log); fcdev continues without it",
				"restarts", s.maxRestarts)
			return errIngestorGaveUp
		}
		wait := s.backoff(restarts)
		s.log.Warn("restarting the ingestor runtime", "in", wait.String(), "restart", fmt.Sprintf("%d of %d", restarts, s.maxRestarts))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return nil
		case <-t.C:
		}
	}
}

// stop ends p: terminate, then kill when it has not exited within the grace.
func (s *ingSupervisor) stop(p ingProc) {
	select {
	case <-p.Done():
		return
	default:
	}
	s.log.Info("stopping the ingestor runtime", "pid", p.Pid())
	_ = p.Terminate()
	t := time.NewTimer(s.grace)
	defer t.Stop()
	select {
	case <-p.Done():
	case <-t.C:
		s.log.Warn("the ingestor runtime did not stop in time; killing it", "pid", p.Pid())
		_ = p.Kill()
		<-p.Done()
	}
}
