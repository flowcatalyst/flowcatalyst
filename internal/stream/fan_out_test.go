package stream

import (
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestBuildJobs_CopiesSubscriptionQueueVerbatim pins R2
// (docs/spec/dispatch-job-priority.md): fan-out writes the raising
// subscription's queue value onto the raised job's own queue column
// verbatim — including a subscription with no value set, which must write
// none, not a defaulted value. Mutant (T5): leave the column nil for every
// job and rely on the publish-time lookup instead — this test must fail
// under it, since it asserts the HIGH_PRIORITY subscription's job carries
// the value directly.
func TestBuildJobs_CopiesSubscriptionQueueVerbatim(t *testing.T) {
	hi := "HIGH_PRIORITY"
	now := time.Now().UTC()
	events := []claimedEvent{
		{ID: "evt1", EventType: "test:evt", Source: "src", CreatedAt: now},
	}
	subs := []cachedSubscription{
		{
			ID:                "sub_hi",
			Target:            "https://sub.test/hi",
			Mode:              common.DispatchImmediate,
			Queue:             &hi,
			EventTypePatterns: []string{"test:evt"},
		},
		{
			ID:                "sub_lo",
			Target:            "https://sub.test/lo",
			Mode:              common.DispatchImmediate,
			Queue:             nil,
			EventTypePatterns: []string{"test:evt"},
		},
	}

	jobs := buildJobs(events, subs)
	if len(jobs) != 2 {
		t.Fatalf("buildJobs returned %d jobs, want 2 (one per matching subscription)", len(jobs))
	}

	byRaiser := map[string]*newJob{}
	for i := range jobs {
		byRaiser[jobs[i].SubscriptionID] = &jobs[i]
	}

	hiJob := byRaiser["sub_hi"]
	if hiJob == nil {
		t.Fatal("no job raised from sub_hi")
	}
	if hiJob.Queue == nil || *hiJob.Queue != "HIGH_PRIORITY" {
		t.Errorf("sub_hi's job.Queue = %v, want a pointer to \"HIGH_PRIORITY\"", hiJob.Queue)
	}

	loJob := byRaiser["sub_lo"]
	if loJob == nil {
		t.Fatal("no job raised from sub_lo")
	}
	if loJob.Queue != nil {
		t.Errorf("sub_lo's job.Queue = %q, want nil — a subscription with no queue set writes none onto the job", *loJob.Queue)
	}
}
