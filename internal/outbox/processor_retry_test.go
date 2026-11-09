package outbox

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// memRepo is an in-memory outbox table with the Postgres backend's semantics: claims in
// (message_group NULLS LAST, created_at, id) order, honouring the ClaimFilter.
type memRepo struct {
	mu   sync.Mutex
	rows map[string]*memRow
}

type memRow struct {
	item    Item
	status  common.OutboxStatus
	retries int
	msg     string
}

func newMemRepo(items ...Item) *memRepo {
	m := &memRepo{rows: map[string]*memRow{}}
	for _, it := range items {
		m.rows[it.ID] = &memRow{item: it, status: common.OutboxPending}
	}
	return m
}

func (m *memRepo) put(it Item, status common.OutboxStatus, retries int, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[it.ID] = &memRow{item: it, status: status, retries: retries, msg: msg}
}

func (m *memRepo) row(id string) (memRow, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return memRow{}, false
	}
	return *r, true
}

func (m *memRepo) remaining() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}

func (m *memRepo) ClaimPending(_ context.Context, n int, skip ClaimFilter) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var cand []*memRow
	for _, r := range m.rows {
		if r.status != common.OutboxPending || slices.Contains(skip.IDs, r.item.ID) {
			continue
		}
		if grouped(r.item) && slices.Contains(skip.Groups, *r.item.MessageGroup) {
			continue
		}
		if !grouped(r.item) && slices.Contains(skip.Types, r.item.ItemType) {
			continue
		}
		cand = append(cand, r)
	}
	groupKey := func(it Item) string {
		if grouped(it) {
			return "0" + *it.MessageGroup
		}
		return "1" // NULLS LAST
	}
	slices.SortFunc(cand, func(a, b *memRow) int {
		return cmp.Or(strings.Compare(groupKey(a.item), groupKey(b.item)),
			a.item.CreatedAt.Compare(b.item.CreatedAt), strings.Compare(a.item.ID, b.item.ID))
	})
	var out []Item
	for _, r := range cand[:min(n, len(cand))] {
		r.status = common.OutboxInProgress
		it := r.item
		it.AttemptCount = r.retries
		out = append(out, it)
	}
	return out, nil
}

func (m *memRepo) MarkSuccess(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		delete(m.rows, id)
	}
	return nil
}

func (m *memRepo) MarkFailed(_ context.Context, ids []string, st common.OutboxStatus, msg string, requeue bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		r := m.rows[id]
		r.retries++
		r.msg = msg
		r.status = st
		if requeue {
			r.status = common.OutboxPending
		}
	}
	return nil
}

func (m *memRepo) Release(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if r := m.rows[id]; r != nil && r.status == common.OutboxInProgress {
			r.status = common.OutboxPending
		}
	}
	return nil
}

func (m *memRepo) Requeue(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if r := m.rows[id]; r != nil {
			r.status, r.retries, r.msg = common.OutboxPending, 0, ""
		}
	}
	return nil
}

func (m *memRepo) MarkSkipped(_ context.Context, ids []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, id := range ids {
		if r := m.rows[id]; r != nil && !strings.HasPrefix(r.msg, SkippedMarker) {
			r.msg = SkippedMarker + strings.TrimPrefix(r.msg, BlockingMarker)
		}
	}
	return nil
}

func refused(st common.OutboxStatus) bool {
	return st == common.OutboxBadRequest || st == common.OutboxForbidden
}

func (m *memRepo) MarkLegacyRefusals(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if refused(r.status) && StripMarkers(r.msg) == r.msg {
			r.msg = SkippedBeforeUpgradeMarker + r.msg
			n++
		}
	}
	return n, nil
}

func (m *memRepo) RequeueRetryableFailures(context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if r.status != common.OutboxInProgress && r.status.IsRetryable() {
			r.status = common.OutboxPending
			n++
		}
	}
	return n, nil
}

func (m *memRepo) BlockingFailures(context.Context) ([]Item, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	heads := map[string]Item{}
	for _, r := range m.rows {
		if !grouped(r.item) || !refused(r.status) || !strings.HasPrefix(r.msg, BlockingMarker) {
			continue
		}
		g := *r.item.MessageGroup
		if h, ok := heads[g]; !ok || cmp.Or(r.item.CreatedAt.Compare(h.CreatedAt), strings.Compare(r.item.ID, h.ID)) < 0 {
			it := r.item
			it.Status, it.StatusMessage = r.status, r.msg
			heads[g] = it
		}
	}
	out := make([]Item, 0, len(heads))
	for _, h := range heads {
		out = append(out, h)
	}
	return out, nil
}

func (m *memRepo) RecoverStuck(context.Context, time.Duration) (int, error) { return 0, nil }
func (m *memRepo) Healthy(context.Context) bool                             { return true }
func (m *memRepo) InitSchema(context.Context) error                         { return nil }

// rowsFor writes n rows of a group (or ungrouped, group ""), one millisecond apart, ids <prefix>-01...
func rowsFor(prefix, group string, n int, at time.Time) []Item {
	out := make([]Item, n)
	for i := range n {
		it := pushItem(fmt.Sprintf("%s-%02d", prefix, i+1), group)
		it.CreatedAt = at.Add(time.Duration(i) * time.Millisecond)
		out[i] = it
	}
	return out
}

// pushIDs reads the pushIds of a batch request.
func pushIDs(r *http.Request) []string {
	var body struct {
		Items []struct {
			PushID string `json:"pushId"`
		} `json:"items"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	ids := make([]string, len(body.Items))
	for i, it := range body.Items {
		ids[i] = it.PushID
	}
	return ids
}

func writeResults(w http.ResponseWriter, statuses ...string) {
	res := make([]map[string]string, len(statuses))
	for i, s := range statuses {
		res[i] = map[string]string{"status": s}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
}

func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRetryBackoffSchedule(t *testing.T) {
	base, ceiling := time.Second, 5*time.Minute
	want := []time.Duration{1, 2, 4, 8, 16, 32, 64, 128, 256}
	for i, w := range want {
		if got := retryBackoff(i+1, base, ceiling, 0); got != w*time.Second {
			t.Errorf("failure %d: backoff %v, want %v", i+1, got, w*time.Second)
		}
	}
	for _, n := range []int{10, 11, 64, 1000, 1 << 30} {
		if got := retryBackoff(n, base, ceiling, 0); got != ceiling {
			t.Errorf("failure %d: backoff %v, want the cap %v", n, got, ceiling)
		}
	}
	// jitter takes off at most a quarter
	for _, n := range []int{1, 3, 20} {
		top := retryBackoff(n, base, ceiling, 0)
		got := retryBackoff(n, base, ceiling, 0.999)
		if got > top || got < top*3/4 {
			t.Errorf("failure %d with jitter: %v outside [%v, %v]", n, got, top*3/4, top)
		}
	}
	if got := retryBackoff(1, 0, 0, 0); got != defaultRetryBackoffBase {
		t.Errorf("an unset base backs off %v, want %v", got, defaultRetryBackoffBase)
	}
	if c := DefaultConfig(); c.RetryBackoffBase != time.Second || c.RetryBackoffMax != 5*time.Minute {
		t.Errorf("default backoff %v..%v, want 1s..5m", c.RetryBackoffBase, c.RetryBackoffMax)
	}
}

// However many times a row has been tried, a retryable failure (5xx, transport, per-item
// INTERNAL_ERROR, 401) puts it back to PENDING: never a terminal status, never a blocked group.
func TestRetryableFailureIsNeverTerminal(t *testing.T) {
	for name, respond := range map[string]http.HandlerFunc{
		"503":            func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
		"500":            func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
		"401":            func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusUnauthorized) },
		"INTERNAL_ERROR": func(w http.ResponseWriter, _ *http.Request) { writeResults(w, "INTERNAL_ERROR") },
		"GATEWAY_ERROR":  func(w http.ResponseWriter, _ *http.Request) { writeResults(w, "GATEWAY_ERROR") },
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(respond)
			defer srv.Close()
			repo := &recordRepo{}
			cfg := DefaultConfig()
			cfg.PlatformURL = srv.URL
			cfg.MaxRetries = 3 // the old cap: it must not matter
			p := NewProcessor(cfg, repo)

			const attempts = 40
			for i := range attempts {
				grp := pushItem("g-1", "shop:a")
				grp.AttemptCount = i
				if p.dispatch(context.Background(), grp) {
					t.Fatal("a failure is not a success")
				}
				solo := pushItem("u-1", "")
				solo.AttemptCount = i
				p.dispatchBatch(context.Background(), []Item{solo})
				p.holdMu.Lock()
				clear(p.holds) // the hold passes
				p.holdMu.Unlock()
			}
			if len(repo.requeues) != 2*attempts {
				t.Fatalf("%d failures recorded, want %d", len(repo.requeues), 2*attempts)
			}
			for i, rq := range repo.requeues {
				if !rq {
					t.Fatalf("failure %d was left in a terminal status", i+1)
				}
			}
			if !p.groups.IsActive("shop:a") {
				t.Fatal("a retryable failure blocked the group")
			}
			if _, failed := p.Totals(); failed != 0 || p.Retried() != 2*attempts {
				t.Fatalf("failed=%d retried=%d", failed, p.Retried())
			}
		})
	}
}

// Each consecutive retryable failure holds the group longer, the hold is visible, a success
// forgets the failures, and a failure while held (a straggler) does not escalate.
func TestRetryableFailureBacksOffAndRecovers(t *testing.T) {
	var mu sync.Mutex
	fail := true
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeResults(w, "SUCCESS")
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.RetryBackoffBase = time.Hour
	cfg.RetryBackoffMax = 4 * time.Hour
	p := NewProcessor(cfg, &recordRepo{})
	item := pushItem("g-1", "shop:a")

	for n, top := range []time.Duration{time.Hour, 2 * time.Hour, 4 * time.Hour, 4 * time.Hour} {
		start := time.Now()
		p.dispatch(context.Background(), item)
		holds := p.Holds()
		if len(holds) != 1 || holds[0].Key != "group:shop:a" || holds[0].ConsecutiveFailures != n+1 {
			t.Fatalf("after failure %d: holds %+v", n+1, holds)
		}
		if got := holds[0].Until.Sub(start); got > top+time.Second || got < top*3/4 {
			t.Fatalf("after failure %d: held %v, want %v less jitter", n+1, got, top)
		}
		if !p.held(item) || p.held(pushItem("x", "shop:b")) {
			t.Fatal("only the failed group is held")
		}
		// a straggler failing while the group is held does not escalate
		p.dispatch(context.Background(), item)
		if h := p.Holds(); h[0].ConsecutiveFailures != n+1 {
			t.Fatalf("a failure during the hold escalated it: %+v", h)
		}
		p.holdMu.Lock()
		clear(p.holds) // the hold passes
		p.holdMu.Unlock()
	}
	mu.Lock()
	fail = false
	mu.Unlock()
	if !p.dispatch(context.Background(), item) {
		t.Fatal("delivery after the outage")
	}
	p.holdMu.Lock()
	n := len(p.failures)
	p.holdMu.Unlock()
	if n != 0 {
		t.Fatal("a success forgets the group's failures")
	}
}

// A 429 still holds for its Retry-After without spending an attempt or counting a failure.
func TestA429HoldIsNotARetryableFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "30")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)
	p.dispatch(context.Background(), pushItem("g-1", "shop:a"))
	h := p.Holds()
	if len(h) != 1 || h[0].ConsecutiveFailures != 0 || time.Until(h[0].Until) < 29*time.Second {
		t.Fatalf("holds %+v: a 429 holds for its Retry-After and counts no failure", h)
	}
	if len(repo.failed) != 0 || p.Retried() != 0 {
		t.Fatalf("failed=%v retried=%d: a 429 spends no attempt", repo.failed, p.Retried())
	}
}

// A whole request failing holds the type; one row failing in a batch that otherwise went through
// holds that row alone, so it does not slow its whole type.
func TestUngroupedRetryHoldsTheTypeOrTheRow(t *testing.T) {
	var mu sync.Mutex
	partial := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if partial {
			writeResults(w, "SUCCESS", "INTERNAL_ERROR", "SUCCESS")
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)
	batch := []Item{pushItem("u-1", ""), pushItem("u-2", ""), pushItem("u-3", "")}

	p.dispatchBatch(context.Background(), batch)
	if h := p.Holds(); len(h) != 1 || h[0].Key != "type:PUSH" {
		t.Fatalf("holds %+v, want the PUSH type held", h)
	}
	p.holdMu.Lock()
	clear(p.holds)
	p.holdMu.Unlock()

	mu.Lock()
	partial = true
	mu.Unlock()
	p.dispatchBatch(context.Background(), batch)
	if h := p.Holds(); len(h) != 1 || h[0].Key != "item:u-2" {
		t.Fatalf("holds %+v, want u-2 alone held", h)
	}
	if !p.held(pushItem("u-2", "")) || p.held(pushItem("u-9", "")) {
		t.Fatal("u-2 is held, another PUSH row is not")
	}
	f := p.claimFilter(time.Now())
	if !slices.Equal(f.IDs, []string{"u-2"}) || len(f.Types) != 0 {
		t.Fatalf("claim filter %+v, want the row u-2 alone", f)
	}
	for _, rq := range repo.requeues {
		if !rq {
			t.Fatal("a retryable failure was left terminal")
		}
	}
}

// The claim passes over everything the processor would only put back.
func TestClaimFilterNamesHeldAndInactive(t *testing.T) {
	p := NewProcessor(DefaultConfig(), &stubRepo{})
	p.hold(pushItem("a-1", "held"), time.Hour)
	p.hold(pushItem("u-1", ""), time.Hour)
	p.holdUntil(itemHoldKey(pushItem("u-7", "")), time.Now().Add(time.Hour))
	p.holdUntil("group:gone", time.Now().Add(-time.Second))
	p.PauseGroup("paused")
	p.groups.Block("blocked", "b-1", "400")
	f := p.claimFilter(time.Now())
	slices.Sort(f.Groups)
	if !slices.Equal(f.Groups, []string{"blocked", "held", "paused"}) ||
		!slices.Equal(f.Types, []common.OutboxItemType{common.OutboxItemPush}) || !slices.Equal(f.IDs, []string{"u-7"}) {
		t.Fatalf("claim filter %+v", f)
	}
}

// Through Run against an in-memory table: rows of a group fail retryably (a 503, a per-item
// INTERNAL_ERROR) a few times, and no row of a group is ever sent while an earlier row of the
// group is undelivered — with BlockOnError on, and off.
func TestRetryKeepsEachGroupInOrder(t *testing.T) {
	for _, blockOnError := range []bool{true, false} {
		t.Run(fmt.Sprintf("BlockOnError=%v", blockOnError), func(t *testing.T) {
			at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
			written := map[string][]string{}
			var all []Item
			for _, g := range []string{"a", "b", "c"} {
				rows := rowsFor(g, "shop:"+g, 8, at)
				for _, r := range rows {
					written["shop:"+g] = append(written["shop:"+g], r.ID)
				}
				all = append(all, rows...)
			}
			all = append(all, rowsFor("u", "", 5, at)...)
			repo := newMemRepo(all...)
			groupOf := map[string]string{}
			for _, it := range all {
				if grouped(it) {
					groupOf[it.ID] = *it.MessageGroup
				}
			}
			failuresLeft := map[string]int{"a-02": 3, "b-01": 2, "c-05": 4, "u-03": 2}

			var mu sync.Mutex
			delivered := map[string]bool{}
			var violations []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ids := pushIDs(r)
				mu.Lock()
				defer mu.Unlock()
				statuses := make([]string, len(ids))
				for i, id := range ids {
					if g, ok := groupOf[id]; ok {
						for _, earlier := range written[g] {
							if earlier == id {
								break
							}
							if !delivered[earlier] {
								violations = append(violations, id+" sent before "+earlier)
							}
						}
					}
					if failuresLeft[id] > 0 {
						failuresLeft[id]--
						if id == "a-02" { // the whole request fails
							w.WriteHeader(http.StatusServiceUnavailable)
							return
						}
						statuses[i] = "INTERNAL_ERROR"
						continue
					}
					statuses[i] = "SUCCESS"
					delivered[id] = true
				}
				writeResults(w, statuses...)
			}))
			defer srv.Close()

			cfg := DefaultConfig()
			cfg.PlatformURL = srv.URL
			cfg.PollInterval = 2 * time.Millisecond
			cfg.BatchSize = 5
			cfg.BlockOnError = blockOnError
			cfg.RetryBackoffBase = 5 * time.Millisecond
			cfg.RetryBackoffMax = 20 * time.Millisecond
			p := NewProcessor(cfg, repo)
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			go func() { p.Run(ctx); close(done) }()
			waitFor(t, 15*time.Second, "every row delivered", func() bool { return repo.remaining() == 0 })
			cancel()
			<-done

			mu.Lock()
			defer mu.Unlock()
			if len(violations) > 0 {
				t.Fatalf("rows overtook an undelivered earlier row: %v", violations)
			}
			if len(delivered) != len(all) {
				t.Fatalf("%d of %d rows delivered", len(delivered), len(all))
			}
		})
	}
}

// Start-up: the rows an older version left in a retryable failure status go back to PENDING, and
// the stored non-retryable failures block their groups; nothing is claimed before that worked.
func TestPrepareSweepsAndRebuildsBlockedGroups(t *testing.T) {
	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	g := rowsFor("g", "shop:g", 3, at)
	h := rowsFor("h", "shop:h", 2, at)
	k := rowsFor("k", "shop:k", 2, at)
	legacy := rowsFor("old", "shop:old", 1, at)[0]
	refusedBefore := rowsFor("pre", "shop:pre", 2, at) // refused by an earlier version: no marker
	repo := newMemRepo(g[1], g[2], h[1], k[1], refusedBefore[1])
	repo.put(g[0], common.OutboxBadRequest, 1, BlockingMarker+"400 bad")
	repo.put(h[0], common.OutboxForbidden, 1, BlockingMarker+"403")
	repo.put(k[0], common.OutboxBadRequest, 1, SkippedMarker+"400 skipped earlier")
	repo.put(legacy, common.OutboxGatewayError, 3, "503")
	repo.put(refusedBefore[0], common.OutboxBadRequest, 1, "400 old")

	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := pushIDs(r)
		mu.Lock()
		sent = append(sent, ids...)
		mu.Unlock()
		st := make([]string, len(ids))
		for i := range st {
			st[i] = "SUCCESS"
		}
		writeResults(w, st...)
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.PollInterval = 2 * time.Millisecond
	p := NewProcessor(cfg, repo)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	waitFor(t, 5*time.Second, "the swept, skipped and skipped-before-upgrade groups' rows delivered", func() bool {
		_, oldLeft := repo.row(legacy.ID)
		_, kLeft := repo.row(k[1].ID)
		_, preLeft := repo.row(refusedBefore[1].ID)
		return !oldLeft && !kLeft && !preLeft
	})
	time.Sleep(50 * time.Millisecond) // a few more polls
	cancel()
	<-done

	for _, it := range []Item{g[1], g[2], h[1]} {
		if r, ok := repo.row(it.ID); !ok || r.status != common.OutboxPending {
			t.Fatalf("%s moved past its group's stored failure (row %+v, present %v)", it.ID, r, ok)
		}
	}
	if r, _ := repo.row(refusedBefore[0].ID); r.status != common.OutboxBadRequest || r.msg != SkippedBeforeUpgradeMarker+"400 old" {
		t.Fatalf("the earlier version's refusal is %+v, want BAD_REQUEST marked skipped before upgrade", r)
	}
	blocked := map[string]string{}
	for _, b := range p.BlockedGroups() {
		blocked[b.Group] = b.BlockedItemID
		if b.Group == "shop:g" && b.Error != "400 bad" {
			t.Fatalf("blocked group shows %q, want the platform's text without the marker", b.Error)
		}
	}
	if len(blocked) != 2 || blocked["shop:g"] != g[0].ID || blocked["shop:h"] != h[0].ID {
		t.Fatalf("blocked %v, want shop:g on %s and shop:h on %s", blocked, g[0].ID, h[0].ID)
	}
	mu.Lock()
	defer mu.Unlock()
	if slices.Contains(sent, g[1].ID) || slices.Contains(sent, h[1].ID) {
		t.Fatalf("sent %v: a blocked group's rows were sent", sent)
	}
}

// A failed preparation claims nothing, and is retried at the next poll.
func TestPrepareFailureClaimsNothing(t *testing.T) {
	repo := &stubRepo{prepareErr: errors.New("db down")}
	claims := &claimOnce{Repository: repo, items: []Item{pushItem("g-1", "shop:a")}}
	p := NewProcessor(DefaultConfig(), claims)
	p.tick(context.Background())
	if len(claims.filters) != 0 || p.prepared {
		t.Fatal("claimed before the start-up preparation succeeded")
	}
	repo.prepareErr = nil
	repo.blocking = []Item{func() Item { it := pushItem("p-1", "shop:a"); it.StatusMessage = "400"; return it }()}
	p.tick(context.Background())
	if len(claims.filters) != 1 || !p.prepared || repo.prepares != 2 {
		t.Fatalf("filters=%d prepared=%v prepares=%d", len(claims.filters), p.prepared, repo.prepares)
	}
	if !slices.Contains(claims.filters[0].Groups, "shop:a") {
		t.Fatalf("the claim did not pass over the rebuilt blocked group: %+v", claims.filters[0])
	}
}

// With BlockOnError off, stored failures block nothing (the group carries on past them).
func TestPrepareWithoutBlockOnErrorBlocksNothing(t *testing.T) {
	repo := &stubRepo{blocking: []Item{pushItem("p-1", "shop:a")}}
	cfg := DefaultConfig()
	cfg.BlockOnError = false
	p := NewProcessor(cfg, repo)
	if err := p.prepare(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(p.BlockedGroups()) != 0 {
		t.Fatal("BlockOnError off blocked a group")
	}
}

type failingDecisions struct{ stubRepo }

func (failingDecisions) Requeue(context.Context, []string) error { return errors.New("db down") }

func (failingDecisions) MarkSkipped(context.Context, []string) error { return errors.New("db down") }

// An unblock or skip that cannot be stored leaves the group blocked: it never moves past the row
// in memory only to block again after a restart.
func TestUnstoredOperatorDecisionKeepsTheBlock(t *testing.T) {
	p := NewProcessor(DefaultConfig(), &failingDecisions{})
	p.groups.Block("g", "p-1", "400")
	if ok, err := p.UnblockGroup(context.Background(), "g"); !ok || err == nil {
		t.Fatalf("unblock = %v, %v; want the error", ok, err)
	}
	if ok, err := p.SkipGroup(context.Background(), "g"); !ok || err == nil {
		t.Fatalf("skip = %v, %v; want the error", ok, err)
	}
	if p.groups.IsActive("g") {
		t.Fatal("the group was unblocked although the decision was not stored")
	}
	if ok, _ := p.SkipGroup(context.Background(), "not-blocked"); ok {
		t.Fatal("skip of a group that is not blocked")
	}
}

// releaseHook runs onRelease once, at the first Release: something changing half-way through a
// claim.
type releaseHook struct {
	*recordRepo
	onRelease func()
}

func (r *releaseHook) Release(ctx context.Context, ids []string) error {
	_ = r.recordRepo.Release(ctx, ids)
	if f := r.onRelease; f != nil {
		r.onRelease = nil
		f()
	}
	return nil
}

// A group put back once in a claim stays put back for the rest of it, even when its hold is lifted
// or it is resumed half-way through: its later row is never sent ahead of the put-back one.
func TestPutBackHoldsForTheRestOfTheClaim(t *testing.T) {
	for name, setup := range map[string]func(p *Processor) func(){
		"hold lifted": func(p *Processor) func() {
			p.hold(pushItem("g-1", "shop:a"), time.Hour)
			return func() {
				p.holdMu.Lock()
				clear(p.holds)
				p.holdMu.Unlock()
			}
		},
		"group resumed": func(p *Processor) func() {
			p.PauseGroup("shop:a")
			return func() { p.ResumeGroup("shop:a") }
		},
	} {
		t.Run(name, func(t *testing.T) {
			var mu sync.Mutex
			calls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				mu.Lock()
				calls++
				mu.Unlock()
				writeResults(w, "SUCCESS")
			}))
			defer srv.Close()
			cfg := DefaultConfig()
			cfg.PlatformURL = srv.URL
			rec := &recordRepo{}
			hook := &releaseHook{recordRepo: rec}
			p := NewProcessor(cfg, &claimOnce{Repository: hook, items: []Item{pushItem("g-1", "shop:a"), pushItem("g-2", "shop:a")}})
			hook.onRelease = setup(p)
			p.tick(context.Background())
			time.Sleep(50 * time.Millisecond) // a wrongly dispatched row would be sent by now
			mu.Lock()
			defer mu.Unlock()
			rec.mu.Lock()
			defer rec.mu.Unlock()
			if calls != 0 || !slices.Equal(rec.released, []string{"g-1", "g-2"}) {
				t.Fatalf("calls=%d released=%v: g-2 went ahead of the put-back g-1", calls, rec.released)
			}
		})
	}
}

// A refusal by this version is stored behind BlockingMarker, grouped or not; the in-memory block
// and StripMarkers show the platform's own text.
func TestRefusalIsStoredWithTheBlockingMarker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ids := pushIDs(r)
		st := make([]string, len(ids))
		for i := range st {
			st[i] = "BAD_REQUEST"
		}
		res := make([]map[string]string, len(ids))
		for i := range st {
			res[i] = map[string]string{"status": st[i], "error": "TOPIC_INVALID"}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": res})
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)
	p.dispatch(context.Background(), pushItem("g-1", "shop:a"))
	p.dispatchBatch(context.Background(), []Item{pushItem("u-1", "")})
	if !slices.Equal(repo.messages, []string{BlockingMarker + "TOPIC_INVALID", BlockingMarker + "TOPIC_INVALID"}) ||
		slices.Contains(repo.requeues, true) {
		t.Fatalf("messages=%q requeues=%v", repo.messages, repo.requeues)
	}
	if b := p.BlockedGroups(); len(b) != 1 || b[0].Error != "TOPIC_INVALID" {
		t.Fatalf("blocked %+v", b)
	}
	for in, want := range map[string]string{
		BlockingMarker + "x": "x", SkippedMarker + "x": "x", SkippedBeforeUpgradeMarker + "x": "x",
		SkippedMarker + BlockingMarker + "x": "x", "x": "x", "": "", "[other] x": "[other] x",
	} {
		if got := StripMarkers(in); got != want {
			t.Errorf("StripMarkers(%q) = %q, want %q", in, got, want)
		}
	}
}
