package scheduler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
)

// backlogInterval is how often the leader samples the PENDING backlog. A var only
// so tests can shorten it.
var backlogInterval = 30 * time.Second

// backlogSampler, leader only, sets the backlog gauges every backlogInterval. The
// count saturates at dispatchjob.PendingBacklogCap+1 (an index range of at most that
// many entries), so a huge backlog is never scanned in full; the oldest-waiting
// figure is the created_at age of the first due job in claim order.
type backlogSampler struct {
	lc *dispatchjob.Lifecycle
	// IsLeader gates every sample: nil = always run. Set by Scheduler.Run.
	IsLeader func() bool
}

func newBacklogSampler(pool *pgxpool.Pool) *backlogSampler {
	return &backlogSampler{lc: dispatchjob.NewLifecycle(pool)}
}

// Run drives the sampler until ctx is cancelled.
func (b *backlogSampler) Run(ctx context.Context) {
	tick := time.NewTicker(backlogInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			if b.IsLeader == nil || b.IsLeader() {
				b.sample(ctx)
			}
		}
	}
}

func (b *backlogSampler) sample(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, claimTimeout)
	defer cancel()
	count, oldest, err := b.lc.PendingBacklog(ctx)
	if err != nil {
		slog.Warn("dispatch backlog sample failed", "err", err)
		return
	}
	schedMetrics.queueBacklogRows.Set(float64(count))
	schedMetrics.queueOldestAge.Set(oldest.Seconds())
}
