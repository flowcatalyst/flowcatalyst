package outbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// recordRepo records what the processor does with a row.
type recordRepo struct {
	stubRepo
	mu        sync.Mutex
	released  []string
	failed    []string
	requeues  []bool   // MarkFailed's requeue, per call
	messages  []string // MarkFailed's msg, per call
	succeeded []string
}

func (r *recordRepo) MarkSuccess(_ context.Context, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.succeeded = append(r.succeeded, ids...)
	return nil
}

func (r *recordRepo) Release(_ context.Context, ids []string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.released = append(r.released, ids...)
	return nil
}

func (r *recordRepo) MarkFailed(_ context.Context, ids []string, _ common.OutboxStatus, msg string, requeue bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failed = append(r.failed, ids...)
	r.requeues = append(r.requeues, requeue)
	r.messages = append(r.messages, msg)
	return nil
}

func pushItem(id, group string) Item {
	it := Item{ID: id, ItemType: common.OutboxItemPush, Payload: json.RawMessage(`{"topic":"shop:a","type":"x","data":{},"pushId":"` + id + `"}`)}
	if group != "" {
		it.MessageGroup = &group
	}
	return it
}

func TestRetryAfterHeader(t *testing.T) {
	for in, want := range map[string]time.Duration{
		"": defaultRetryAfter, "abc": defaultRetryAfter, "-3": defaultRetryAfter,
		"0": time.Second, "7": 7 * time.Second, "999999": maxRetryAfter,
	} {
		if got := retryAfter(in); got != want {
			t.Errorf("Retry-After %q = %v, want %v", in, got, want)
		}
	}
}

// A 429 is back-pressure: the row goes back to PENDING without an attempt, the group is held until
// the platform's Retry-After, and a held group's rows are released untouched at the next poll.
func TestProcessorDefersA429WithoutSpendingAttempts(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)

	item := pushItem("p1", "shop:a")
	if p.dispatch(context.Background(), item) {
		t.Fatal("a 429 is not a success")
	}
	if len(repo.failed) != 0 || len(repo.released) != 1 || repo.released[0] != "p1" {
		t.Fatalf("failed=%v released=%v: a 429 releases the row and fails nothing", repo.failed, repo.released)
	}
	if !p.groups.IsActive("shop:a") {
		t.Fatal("a 429 must not block the group")
	}
	if !p.held(item) {
		t.Fatal("the group is held for the Retry-After")
	}
	if p.held(pushItem("other", "shop:b")) {
		t.Fatal("another group is not held")
	}
	if p.held(pushItem("ungrouped", "")) {
		t.Fatal("an ungrouped row's type is not held by a group")
	}

	// the next poll: a held group's claimed rows are released without a call
	before := calls
	p.hold(item, time.Hour)
	repo2 := &recordRepo{}
	p.repo = &claimOnce{Repository: repo2, items: []Item{pushItem("p2", "shop:a")}}
	p.tick(context.Background())
	if calls != before {
		t.Fatalf("a held group was dispatched (%d calls, was %d)", calls, before)
	}
	if len(repo2.released) != 1 || repo2.released[0] != "p2" {
		t.Fatalf("released=%v, want p2 released untouched", repo2.released)
	}
}

type claimOnce struct {
	Repository
	items   []Item
	filters []ClaimFilter
}

func (c *claimOnce) ClaimPending(_ context.Context, _ int, skip ClaimFilter) ([]Item, error) {
	out := c.items
	c.items = nil
	c.filters = append(c.filters, skip)
	return out, nil
}

// Ungrouped rows are held by type, and the batch's rows are all released.
func TestProcessorDefersAnUngroupedBatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	repo := &recordRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, repo)

	p.dispatchBatch(context.Background(), []Item{pushItem("a", ""), pushItem("b", "")})
	if len(repo.failed) != 0 || len(repo.released) != 2 {
		t.Fatalf("failed=%v released=%v", repo.failed, repo.released)
	}
	if !p.held(pushItem("c", "")) {
		t.Fatal("the PUSH type is held")
	}
	time.Sleep(1100 * time.Millisecond)
	if p.held(pushItem("c", "")) {
		t.Fatal("the hold passes after the Retry-After")
	}
}

func TestPushItemTypeRoutesToTheGateway(t *testing.T) {
	if got := common.OutboxItemPush.APIPath(); got != "/api/push/batch" {
		t.Fatalf("APIPath = %q", got)
	}
	if ty, ok := common.ParseOutboxItemType("PUSH"); !ok || ty != common.OutboxItemPush {
		t.Fatal("PUSH does not parse")
	}
}

// A claim's rows come back in any order (UPDATE ... RETURNING), and rows written in the same
// instant tie on created_at: a group's rows still go out as written, by (created_at, id).
func TestProcessorSendsAGroupInWrittenOrderWhateverTheClaimOrder(t *testing.T) {
	var mu sync.Mutex
	var sent []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		for _, it := range body.Items {
			sent = append(sent, it.PushID)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"status":"SUCCESS"}]}`))
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, &recordRepo{})

	at := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	item := func(id string, created time.Time) Item {
		it := pushItem(id, "shop:a")
		it.CreatedAt = created
		return it
	}
	// written in the order 01, 02, 03, 04 (01-03 in the same instant), claimed back to front
	p.repo = &claimOnce{Repository: &recordRepo{}, items: []Item{
		item("0AAAAAAAAAA04", at.Add(time.Millisecond)),
		item("0AAAAAAAAAA03", at), item("0AAAAAAAAAA02", at), item("0AAAAAAAAAA01", at),
	}}
	p.tick(context.Background())
	deadline := time.Now().Add(5 * time.Second)
	for {
		mu.Lock()
		n := len(sent)
		mu.Unlock()
		if n == 4 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	want := []string{"0AAAAAAAAAA01", "0AAAAAAAAAA02", "0AAAAAAAAAA03", "0AAAAAAAAAA04"}
	if len(sent) != 4 {
		t.Fatalf("sent %v, want %v", sent, want)
	}
	for i := range want {
		if sent[i] != want[i] {
			t.Fatalf("sent %v, want %v", sent, want)
		}
	}
}

// A group that had a row put back in a claim stays put back for the rest of that claim, so a hold
// that passes half-way through cannot send a later row ahead of an earlier one.
func TestProcessorKeepsAGroupTogetherWithinAClaim(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"status":"SUCCESS"}]}`))
	}))
	defer srv.Close()
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	p := NewProcessor(cfg, &recordRepo{})
	first := pushItem("0AAAAAAAAAA01", "shop:a")
	p.hold(first, time.Hour)
	repo := &recordRepo{}
	p.repo = &claimOnce{Repository: repo, items: []Item{first, pushItem("0AAAAAAAAAA02", "shop:a")}}
	// the hold is lifted after the first row was looked at: simulated by the put-back rule alone,
	// since the second row is checked against the same claim
	p.tick(context.Background())
	if calls != 0 || len(repo.released) != 2 {
		t.Fatalf("calls=%d released=%v: both rows of the held group go back", calls, repo.released)
	}
}
