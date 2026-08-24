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

// TestBuildMessageCarriesMode is the regression guard: the poller reads a job's
// mode and it must survive onto the queue message. It used to be dropped in
// buildMessage, so every dispatch job reached the router as IMMEDIATE and took
// the concurrent path, discarding the ordering guarantee.
//
// The pool is NOT carried yet — see DispatchJobToken — so PoolCode stays empty
// and every job still routes to DEFAULT-POOL.
func TestBuildMessageCarriesMode(t *testing.T) {
	d := routingDispatcher()

	msg := d.buildMessage(DispatchJobToken{
		JobID:        "dsp_1",
		MessageGroup: "order-42",
		TargetURL:    "https://sub.example/hook",
		Mode:         "BLOCK_ON_ERROR",
	})

	if msg.PoolCode != "" {
		t.Errorf("PoolCode = %q, want empty until pool propagation lands", msg.PoolCode)
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

// TestBuildMessageLeavesPoolUnset: until pool propagation lands, PoolCode is
// always empty and the router applies its own DEFAULT-POOL fallback.
func TestBuildMessageLeavesPoolUnset(t *testing.T) {
	d := routingDispatcher()

	msg := d.buildMessage(DispatchJobToken{JobID: "dsp_1"})

	if msg.PoolCode != "" {
		t.Errorf("PoolCode = %q, want empty so the router applies its own fallback", msg.PoolCode)
	}
	if msg.MessageGroupID != nil {
		t.Errorf("MessageGroupID = %v, want nil for an ungrouped job", msg.MessageGroupID)
	}
}
