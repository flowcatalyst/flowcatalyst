package outbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// stubRepo records Requeue and MarkSkipped calls and serves the start-up
// state it is given; everything else is a no-op.
type stubRepo struct {
	requeued []string
	skipped  []string
	// blocking and swept are what BlockingFailures / RequeueRetryableFailures
	// answer; prepareErr fails both.
	blocking   []Item
	swept      int
	prepareErr error
	prepares   int
}

func (s *stubRepo) ClaimPending(context.Context, int, ClaimFilter) ([]Item, error) { return nil, nil }
func (s *stubRepo) MarkSuccess(context.Context, []string) error                    { return nil }
func (s *stubRepo) MarkFailed(context.Context, []string, common.OutboxStatus, string, bool) error {
	return nil
}
func (s *stubRepo) Release(context.Context, []string) error { return nil }
func (s *stubRepo) Requeue(_ context.Context, ids []string) error {
	s.requeued = append(s.requeued, ids...)
	return nil
}

func (s *stubRepo) MarkSkipped(_ context.Context, ids []string) error {
	s.skipped = append(s.skipped, ids...)
	return nil
}

func (s *stubRepo) RequeueRetryableFailures(context.Context) (int, error) {
	s.prepares++
	return s.swept, s.prepareErr
}

func (s *stubRepo) MarkLegacyRefusals(context.Context) (int, error) { return 0, s.prepareErr }
func (s *stubRepo) BlockingFailures(context.Context) ([]Item, error) {
	return s.blocking, s.prepareErr
}
func (s *stubRepo) RecoverStuck(context.Context, time.Duration) (int, error) { return 0, nil }
func (s *stubRepo) Healthy(context.Context) bool                             { return true }
func (s *stubRepo) InitSchema(context.Context) error                         { return nil }

func groupedItem(id, group, status string) (Item, *httptest.Server) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"id": id, "status": status, "error": "x"}}})
	}))
	return Item{ID: id, ItemType: common.OutboxItemEvent, MessageGroup: &group, Payload: json.RawMessage(`{}`)}, srv
}

// A permanent (non-retryable) failure of a grouped item blocks its group; the
// group's items are then withheld until Unblock (re-queues the poison) or Skip
// (leaves it failed).
func TestProcessorBlocksGroupOnPermanentFailure(t *testing.T) {
	item, srv := groupedItem("itm1", "g1", "BAD_REQUEST") // BAD_REQUEST is non-retryable → permanent
	defer srv.Close()

	repo := &stubRepo{}
	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BlockOnError = true
	p := NewProcessor(cfg, repo)

	if ok := p.dispatch(context.Background(), item); ok {
		t.Fatal("permanent failure must return false")
	}
	if p.groups.IsActive("g1") {
		t.Fatal("group must be Blocked after a permanent failure")
	}
	if b := p.BlockedGroups(); len(b) != 1 || b[0].BlockedItemID != "itm1" {
		t.Fatalf("BlockedGroups = %+v, want g1 blocked on itm1", b)
	}

	// Unblock → group Running + the poison item re-queued.
	if ok, err := p.UnblockGroup(context.Background(), "g1"); !ok || err != nil {
		t.Fatalf("UnblockGroup should succeed on a blocked group: %v %v", ok, err)
	}
	if !p.groups.IsActive("g1") {
		t.Fatal("group must be Running after unblock")
	}
	if len(repo.requeued) != 1 || repo.requeued[0] != "itm1" {
		t.Fatalf("unblock must re-queue the poison; requeued=%v", repo.requeued)
	}

	// Re-block, then Skip → group Running, NO re-queue, the poison marked skipped.
	p.dispatch(context.Background(), item)
	repo.requeued = nil
	if ok, err := p.SkipGroup(context.Background(), "g1"); !ok || err != nil {
		t.Fatalf("SkipGroup should succeed on a blocked group: %v %v", ok, err)
	}
	if len(repo.skipped) != 1 || repo.skipped[0] != "itm1" {
		t.Fatalf("skip must mark the poison skipped (so a restart does not re-block); skipped=%v", repo.skipped)
	}
	if !p.groups.IsActive("g1") {
		t.Fatal("group must be Running after skip")
	}
	if len(repo.requeued) != 0 {
		t.Fatalf("skip must NOT re-queue; requeued=%v", repo.requeued)
	}
}

// A retryable failure does NOT block the group.
func TestProcessorRetryableDoesNotBlock(t *testing.T) {
	item, srv := groupedItem("itm2", "g2", "INTERNAL_ERROR") // retryable
	defer srv.Close()

	cfg := DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BlockOnError = true
	p := NewProcessor(cfg, &stubRepo{})

	p.dispatch(context.Background(), item)
	if !p.groups.IsActive("g2") {
		t.Fatal("a retryable failure must NOT block the group")
	}
}
