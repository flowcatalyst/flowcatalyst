//go:build integration

package scheduler

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// statementTracer records the duration of every statement a pool runs.
type statementTracer struct {
	mu      sync.Mutex
	max     time.Duration
	maxSQL  string
	count   int
	started sync.Map // ctx -> start (via context key)
}

type traceKey struct{}

func (s *statementTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, traceKey{}, [2]any{time.Now(), d.SQL})
}

func (s *statementTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	v, ok := ctx.Value(traceKey{}).([2]any)
	if !ok {
		return
	}
	d := time.Since(v[0].(time.Time))
	s.mu.Lock()
	s.count++
	if d > s.max {
		s.max, s.maxSQL = d, v[1].(string)
	}
	s.mu.Unlock()
}

func (s *statementTracer) result() (time.Duration, string, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.max, s.maxSQL, s.count
}

func tracedPool(t *testing.T, base *pgxpool.Pool, tr pgx.QueryTracer, size int32) *pgxpool.Pool {
	t.Helper()
	cfg := base.Config().Copy()
	cfg.ConnConfig.Tracer = tr
	cfg.MaxConns = size
	p, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(p.Close)
	return p
}

// orderedPublisher records, per message group, the order jobs reach the broker, and
// hands each published id to the callback-like worker.
type orderedPublisher struct {
	mu        sync.Mutex
	seen      map[string]int
	lastSeq   map[string]int
	outOfOrd  []string
	duplicate []string
	published atomic.Int64
	out       chan string
}

func (p *orderedPublisher) Publish(_ context.Context, items []PublishItem) ([]string, error) {
	p.mu.Lock()
	for _, it := range items {
		n, _ := strconv.Atoi(it.JobID[1:])
		group, seq := fmt.Sprintf("g%04d", n%500), n/500
		if p.seen[it.JobID]++; p.seen[it.JobID] > 1 {
			p.duplicate = append(p.duplicate, it.JobID)
		}
		if last, ok := p.lastSeq[group]; ok && seq <= last {
			p.outOfOrd = append(p.outOfOrd, fmt.Sprintf("%s: %d after %d", it.JobID, seq, last))
		}
		p.lastSeq[group] = max(p.lastSeq[group], seq)
	}
	p.mu.Unlock()
	p.published.Add(int64(len(items)))
	for _, it := range items {
		select {
		case p.out <- it.JobID:
		default:
		}
	}
	return nil, nil
}

// The class of test that would have caught the queue table's stall: realistic width on a
// real database in the realistic statistics state (the active partition analysed
// while no job was PENDING, then a burst of 200,000). The engine runs as in production
// (10 lanes marking batches of 100, a poller claiming 500 at a time, 1,000 jobs in
// flight) while a callback-like worker moves published jobs PENDING -> PROCESSING ->
// COMPLETED on its own pool. No statement may take longer than one second; no job may be
// lost or published twice or out of order.
func TestConcurrency_LanesPollerAndCallbacksAgainstABurst(t *testing.T) {
	const burstSize, target = 200_000, 40_000
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	tpl := buildPlanTemplate(t, "conc_tpl", true)
	base := testpg.CloneDB(t, tpl, "conc_run", PoolRuntimeParams)
	burst(t, base, burstSize, false)

	schedTracer, cbTracer := &statementTracer{}, &statementTracer{}
	schedPool := tracedPool(t, base, schedTracer, 14) // the scheduler's pool: carries the settings
	// the callback side runs on the platform's pool: no planner settings
	cbCfg := base.Config().Copy()
	delete(cbCfg.ConnConfig.RuntimeParams, "plan_cache_mode")
	delete(cbCfg.ConnConfig.RuntimeParams, "enable_sort")
	cbCfg.ConnConfig.Tracer = cbTracer
	cbCfg.MaxConns = 4
	cbPool, err := pgxpool.NewWithConfig(ctx, cbCfg)
	require.NoError(t, err)
	t.Cleanup(cbPool.Close)

	pub := &orderedPublisher{seen: map[string]int{}, lastSeq: map[string]int{}, out: make(chan string, 100_000)}
	dispatcher := NewMessageGroupDispatcher(schedPool, pub, NewDispatchAuthService("s"), "http://localhost/api/dispatch/process")
	cfg := DefaultConfig()
	poller := NewPendingJobPoller(cfg, schedPool, dispatcher, NewPausedConnectionCache(schedPool, time.Minute))

	m0 := monthStart(time.Now().UTC(), 0)
	var completed atomic.Int64
	cbCtx, cbCancel := context.WithCancel(ctx)
	var cbWG sync.WaitGroup
	lc := dispatchjob.NewLifecycle(cbPool)
	for range 2 {
		cbWG.Add(1)
		go func() {
			defer cbWG.Done()
			for {
				select {
				case <-cbCtx.Done():
					return
				case id := <-pub.out:
					n, _ := strconv.Atoi(id[1:])
					created := m0.Add(time.Duration(n) * time.Second)
					if ok, err := lc.ClaimForDelivery(cbCtx, id, created); err == nil && ok {
						if ok, err := lc.Complete(cbCtx, id, created, 1); err == nil && ok {
							completed.Add(1)
						}
					}
				}
			}
		}()
	}

	runCtx, runCancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { defer close(done); poller.Run(runCtx) }()
	start := time.Now()
	for pub.published.Load() < target && time.Since(start) < 3*time.Minute {
		time.Sleep(50 * time.Millisecond)
	}
	runCancel()
	<-done
	// let the callback worker drain what was published
	time.Sleep(2 * time.Second)
	cbCancel()
	cbWG.Wait()
	elapsed := time.Since(start)

	n := pub.published.Load()
	require.GreaterOrEqual(t, n, int64(target), "the engine did not get through %d jobs in time (published %d in %v)", target, n, elapsed)
	sMax, sSQL, sCount := schedTracer.result()
	cMax, cSQL, cCount := cbTracer.result()
	t.Logf("CONCURRENCY: %d jobs published in %v (%.0f jobs/s), %d completed by the callback worker; scheduler pool: %d statements, max %v; callback pool: %d statements, max %v",
		n, elapsed.Round(time.Millisecond), float64(n)/elapsed.Seconds(), completed.Load(), sCount, sMax.Round(time.Millisecond), cCount, cMax.Round(time.Millisecond))
	t.Logf("CONCURRENCY slowest scheduler statement: %.120q", sSQL)
	t.Logf("CONCURRENCY slowest callback statement: %.120q", cSQL)
	// A planner stall shows as a collapse in throughput before any single statement gets
	// slow (a sargable status guard on the by-key statements: 1,800 jobs/s here against
	// 9,000), so the rate is asserted too, with a wide margin for a slow machine.
	assert.Greater(t, float64(n)/elapsed.Seconds(), 4000.0, "throughput collapsed")
	// The ceiling is one second: a plan regression shows as seconds, while a single
	// statement among tens of thousands can stall a few hundred ms on a loaded CI machine.
	assert.Less(t, sMax, time.Second, "no scheduler statement may exceed 1 s: %.200q", sSQL)
	assert.Less(t, cMax, time.Second, "no callback statement may exceed 1 s: %.200q", cSQL)

	pub.mu.Lock()
	assert.Empty(t, pub.outOfOrd, "published out of order")
	assert.Empty(t, pub.duplicate, "published twice")
	published := make([]string, 0, len(pub.seen))
	for id := range pub.seen {
		published = append(published, id)
	}
	pub.mu.Unlock()
	var stillPending int
	require.NoError(t, base.QueryRow(context.Background(),
		`SELECT count(*) FROM msg_dispatch_jobs WHERE id = ANY($1::text[]) AND status = 'PENDING'`, published).Scan(&stillPending))
	assert.Zero(t, stillPending, "a published job was left PENDING (lost mark-QUEUED)")
}
