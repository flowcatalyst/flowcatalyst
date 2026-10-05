package scheduler

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// jitter is a small lock-free pseudo-random source for sleeps and coin flips.
type jitter struct{ n atomic.Uint64 }

func (j *jitter) next() uint64 {
	z := j.n.Add(1) * 0x9E3779B97F4A7C15
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}

// pct reports true with probability p per cent.
func (j *jitter) pct(p uint64) bool { return j.next()%100 < p }

func (j *jitter) sleep(maxMicros uint64) {
	time.Sleep(time.Duration(j.next()%(maxMicros+1)) * time.Microsecond)
}

// Adversarial stress: small buffers so the poller claims about as fast as the
// lanes drain, real goroutines, random publish failures (a failure takes the
// rest of its group in the call with it, as the SQS publisher does), random mark
// failures (a job published twice), and jitter in the store and the publisher.
// Every job must eventually be published, and each group's FIRST delivery of
// every job must be in claim order (duplicates are allowed).
func TestOrdering_AdversarialStress(t *testing.T) {
	const groups, perGroup = 30, 100
	var jobs []*fakeJob
	for g := range groups {
		jobs = append(jobs, groupJobs(fmt.Sprintf("g%02d", g), perGroup)...)
	}
	s := newFakeStore(jobs...)
	var j jitter
	var publishCalls atomic.Int64

	s.publishFn = func(_ context.Context, toks []DispatchJobToken) []string {
		publishCalls.Add(1)
		j.sleep(200)
		failAt := -1
		if j.pct(3) {
			failAt = int(j.next() % uint64(len(toks)))
		}
		var unpublished []string
		failed := map[string]bool{}
		for i, tk := range toks {
			if i == failAt {
				failed[tk.MessageGroup] = true
			}
			if failed[tk.MessageGroup] {
				unpublished = append(unpublished, tk.JobID)
				continue
			}
			s.logPublished(tk.JobID)
		}
		return unpublished
	}
	p := newTestEngine(Config{
		PollInterval: time.Millisecond, Dispatchers: 4, BufferCapacity: 24, BatchSize: 8, LaneBatch: 5,
	}, s)
	claimRows := p.claimRows
	p.claimRows = func(ctx context.Context, limit int, paused, held []string) ([]dispatchClaim, error) {
		j.sleep(100)
		return claimRows(ctx, limit, paused, held)
	}
	restoreClaims := p.restoreClaimRows
	p.restoreClaimRows = func(ctx context.Context, refs []claimRef) error {
		j.sleep(100)
		if j.pct(2) {
			return fmt.Errorf("release failed") // the lane retries until it succeeds
		}
		return restoreClaims(ctx, refs)
	}
	p.markQueued = func(ctx context.Context, ids []string, v []time.Time, a, b time.Time) (int64, error) {
		j.sleep(100)
		if j.pct(1) {
			return 0, fmt.Errorf("mark failed")
		}
		return s.markQueued(ctx, ids, v, a, b)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); p.Run(ctx) }()
	deadline := time.After(30 * time.Second)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		select {
		case <-tick.C:
			if s.pending() == 0 {
				break wait
			}
		case <-deadline:
			cancel()
			<-done
			t.Fatalf("hang: %d of %d jobs still pending after 30s (%d publish calls, %d claims)",
				s.pending(), groups*perGroup, publishCalls.Load(), s.claimCount())
		}
	}
	cancel()
	<-done

	first := map[string]map[string]bool{}
	order := map[string][]string{}
	var mu sync.Mutex
	mu.Lock()
	defer mu.Unlock()
	for _, id := range s.published() {
		g := id[:3]
		if first[g] == nil {
			first[g] = map[string]bool{}
		}
		if !first[g][id] {
			first[g][id] = true
			order[g] = append(order[g], id)
		}
	}
	require.Len(t, order, groups)
	for g, ids := range order {
		require.Len(t, ids, perGroup, "group %s: every job delivered", g)
		for i := 1; i < len(ids); i++ {
			require.Less(t, ids[i-1], ids[i], "group %s: first delivery out of order", g)
		}
	}
}
