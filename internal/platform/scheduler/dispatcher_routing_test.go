package scheduler

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func routingDispatcher() *MessageGroupDispatcher {
	return &MessageGroupDispatcher{
		authSvc:            NewDispatchAuthService("test-secret"),
		processingEndpoint: "https://fc.example/api/dispatch/process",
	}
}

// TestBuildMessageCarriesPoolAndMode is the regression guard: the poller reads
// a job's dispatch_pool_code and mode, and both must survive onto the queue
// message. They used to be dropped in buildMessage, so every dispatch job
// reached the router with no pool (→ DEFAULT-POOL) and no mode (→ IMMEDIATE),
// discarding both the job's pool isolation and its ordering guarantee.
func TestBuildMessageCarriesPoolAndMode(t *testing.T) {
	d := routingDispatcher()

	msg := d.buildMessage(DispatchJobToken{
		JobID:        "dsp_1",
		MessageGroup: "order-42",
		TargetURL:    "https://sub.example/hook",
		PoolCode:     "HIGH-VOLUME",
		Mode:         "BLOCK_ON_ERROR",
	})

	if msg.PoolCode != "HIGH-VOLUME" {
		t.Errorf("PoolCode = %q, want HIGH-VOLUME", msg.PoolCode)
	}
	if msg.DispatchMode != common.DispatchBlockOnError {
		t.Errorf("DispatchMode = %q, want BLOCK_ON_ERROR", msg.DispatchMode)
	}
	if msg.MessageGroupID == nil || *msg.MessageGroupID != "order-42" {
		t.Errorf("MessageGroupID = %v, want order-42", msg.MessageGroupID)
	}
}

// TestBuildMessageOrderedModesReachTheFIFOPath is the behavioural half: the
// router branches on DispatchMode.RequiresOrdering() to choose between the
// concurrent path and the per-group FIFO buffer. An ordered job whose mode was
// dropped parses to IMMEDIATE and takes the concurrent path, so asserting the
// field alone would not catch a regression that re-broke ordering.
func TestBuildMessageOrderedModesReachTheFIFOPath(t *testing.T) {
	d := routingDispatcher()

	for _, tc := range []struct {
		mode         string
		wantOrdering bool
	}{
		{"BLOCK_ON_ERROR", true},
		{"NEXT_ON_ERROR", true},
		{"IMMEDIATE", false},
		{"", false}, // absent mode is still leniently IMMEDIATE
	} {
		msg := d.buildMessage(DispatchJobToken{
			JobID:        "dsp_1",
			MessageGroup: "order-42",
			Mode:         tc.mode,
		})
		if got := msg.DispatchMode.RequiresOrdering(); got != tc.wantOrdering {
			t.Errorf("mode %q: RequiresOrdering() = %v, want %v", tc.mode, got, tc.wantOrdering)
		}
	}
}

// TestBuildMessageEmptyPoolFallsBackToDefault: a job with no configured pool
// must keep publishing an empty PoolCode, which the router resolves to
// DEFAULT-POOL. Emitting a literal "DEFAULT-POOL" here would instead make the
// message name a pool that may not be configured.
func TestBuildMessageEmptyPoolFallsBackToDefault(t *testing.T) {
	d := routingDispatcher()

	msg := d.buildMessage(DispatchJobToken{JobID: "dsp_1"})

	if msg.PoolCode != "" {
		t.Errorf("PoolCode = %q, want empty so the router applies its own fallback", msg.PoolCode)
	}
	if msg.MessageGroupID != nil {
		t.Errorf("MessageGroupID = %v, want nil for an ungrouped job", msg.MessageGroupID)
	}
}
