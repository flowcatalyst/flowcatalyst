package scheduler

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// fakeJob is one row of the fake dispatch-job table.
type fakeJob struct {
	id, group, status, mode string
	seq                     int32
	ver                     int  // row version: bumped by every move, as updated_at is
	claimed                 bool // claimed_at IS NOT NULL on the job's queue row
}

// fakeStore stands in for msg_dispatch_queue: it answers claims the way the
// claim statement does (unclaimed PENDING rows in (group NULLS LAST, sequence,
// id) order, stamping them claimed), releases claims, marks rows QUEUED guarded
// on PENDING (which leaves the queue), and records what was published, in order.
type fakeStore struct {
	mu         sync.Mutex
	jobs       []*fakeJob
	log        []string // ids published, in publish order (duplicates included)
	claimCalls int
	claims     [][]string // ids each claim returned
	releases   [][]string // ids each release call named
	releaseErr func(call int) error

	// publishFn, when set, replaces the default publisher (which succeeds and
	// logs). It must call logPublished for what it accepts.
	publishFn func(ctx context.Context, toks []DispatchJobToken) []string
}

func newFakeStore(jobs ...*fakeJob) *fakeStore {
	for _, j := range jobs {
		if j.status == "" {
			j.status = "PENDING"
		}
	}
	slices.SortStableFunc(jobs, func(a, b *fakeJob) int {
		switch {
		case a.group == "" && b.group != "":
			return 1
		case a.group != "" && b.group == "":
			return -1
		}
		if c := strings.Compare(a.group, b.group); c != 0 {
			return c
		}
		if a.seq != b.seq {
			return int(a.seq - b.seq)
		}
		return strings.Compare(a.id, b.id)
	})
	return &fakeStore{jobs: jobs}
}

func (s *fakeStore) claimRows(_ context.Context, limit int, paused []string) ([]dispatchClaim, error) {
	if paused == nil {
		return nil, errors.New("nil exclusion array passed to the claim")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.claimCalls++
	var out []dispatchClaim
	var ids []string
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, j := range s.jobs {
		if len(out) == limit {
			break
		}
		if j.status != "PENDING" || j.claimed {
			continue
		}
		j.claimed = true
		mode := j.mode
		if mode == "" {
			mode = "IMMEDIATE"
		}
		out = append(out, dispatchClaim{
			id: j.id, group: j.group, mode: mode,
			sequence: j.seq, createdAt: base.Add(time.Duration(i) * time.Second),
			updatedAt: base.Add(time.Duration(j.ver) * time.Minute),
		})
		ids = append(ids, j.id)
	}
	s.claims = append(s.claims, ids)
	return out, nil
}

// releaseClaims clears the claim of the named jobs.
func (s *fakeStore) releaseClaims(_ context.Context, ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases = append(s.releases, append([]string(nil), ids...))
	if s.releaseErr != nil {
		if err := s.releaseErr(len(s.releases)); err != nil {
			return err
		}
	}
	for _, id := range ids {
		for _, j := range s.jobs {
			if j.id == id {
				j.claimed = false
			}
		}
	}
	return nil
}

// releaseOrphans clears every claim not in excluding.
func (s *fakeStore) releaseOrphans(_ context.Context, excluding []string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ex := map[string]struct{}{}
	for _, id := range excluding {
		ex[id] = struct{}{}
	}
	var n int64
	for _, j := range s.jobs {
		if _, keep := ex[j.id]; j.claimed && !keep {
			j.claimed = false
			n++
		}
	}
	return n, nil
}

func (s *fakeStore) claimedCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, j := range s.jobs {
		if j.claimed {
			n++
		}
	}
	return n
}

func (s *fakeStore) releaseCalls() [][]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([][]string(nil), s.releases...)
}

func (s *fakeStore) logPublished(ids ...string) {
	s.mu.Lock()
	s.log = append(s.log, ids...)
	s.mu.Unlock()
}

func (s *fakeStore) publish(ctx context.Context, toks []DispatchJobToken) []string {
	s.mu.Lock()
	fn := s.publishFn
	s.mu.Unlock()
	if fn != nil {
		return fn(ctx, toks)
	}
	for _, t := range toks {
		s.logPublished(t.JobID)
	}
	return nil
}

func (s *fakeStore) markQueued(_ context.Context, ids []string, versions []time.Time, _, _ time.Time) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var rows int64
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, id := range ids {
		for _, j := range s.jobs {
			if j.id == id && j.status == "PENDING" && base.Add(time.Duration(j.ver)*time.Minute).Equal(versions[i]) {
				j.status = "QUEUED"
				j.claimed = false
				j.ver++
				rows++
			}
		}
	}
	return rows, nil
}

func (s *fakeStore) pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, j := range s.jobs {
		if j.status == "PENDING" {
			n++
		}
	}
	return n
}

func (s *fakeStore) status(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, j := range s.jobs {
		if j.id == id {
			return j.status
		}
	}
	return ""
}

func (s *fakeStore) published() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.log)
}

func (s *fakeStore) claimCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.claimCalls
}

// newTestEngine is the poller + lanes against the fake store: nothing touches a
// database or a broker.
func newTestEngine(cfg Config, s *fakeStore) *PendingJobPoller {
	p := newPoller(cfg)
	p.claimRows = s.claimRows
	p.holdBack = func(context.Context, []string) (map[string]jobKey, error) { return map[string]jobKey{}, nil }
	p.pausedIDs = func(context.Context) (map[string]struct{}, error) { return map[string]struct{}{}, nil }
	p.poolCode = func(context.Context, string, string) string { return "" }
	p.publish = s.publish
	p.markQueued = s.markQueued
	p.releaseClaimRows = s.releaseClaims
	p.releaseOrphans = s.releaseOrphans
	return p
}

// waitIdle blocks until every permit is back: no job is between a claim and a
// lane finishing it.
func waitIdle(t *testing.T, p *PendingJobPoller) {
	t.Helper()
	require.Eventually(t, func() bool { return len(p.permits) == 0 }, 10*time.Second, time.Millisecond,
		"the engine never went idle: %d permit(s) still held", len(p.permits))
}

// settleOnce is a synchronous "claim once and wait until the lanes are idle":
// it runs the lanes for exactly one claim.
func settleOnce(t *testing.T, p *PendingJobPoller, ctx context.Context) claimResult {
	t.Helper()
	lctx, cancel := context.WithCancel(context.Background())
	wg := p.startLanes(lctx)
	defer func() {
		cancel()
		wg.Wait()
	}()
	res := p.claimOnce(ctx)
	require.NoError(t, res.err)
	waitIdle(t, p)
	return res
}

func tokenIDs(toks []DispatchJobToken) []string {
	ids := make([]string, len(toks))
	for i, tk := range toks {
		ids[i] = tk.JobID
	}
	return ids
}
