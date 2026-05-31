package api

import (
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
)

// TestResolveInstanceCompletion covers both complete-instance dialects: the
// SDK's {status:SUCCESS/FAILURE} (status = completion outcome → instance
// COMPLETED) and the SPA's {status:<instance-status>, completionStatus}.
func TestResolveInstanceCompletion(t *testing.T) {
	cases := []struct {
		name           string
		status         string
		completion     string
		wantStatus     scheduledjob.InstanceStatus
		wantCompletion string
	}{
		// SDK dialect — the bug this fixes: status carries SUCCESS/FAILURE.
		{"sdk success", "SUCCESS", "", scheduledjob.InstanceStatusCompleted, "SUCCESS"},
		{"sdk failure", "FAILURE", "", scheduledjob.InstanceStatusCompleted, "FAILURE"},
		{"sdk lowercase", "success", "", scheduledjob.InstanceStatusCompleted, "SUCCESS"},
		// SPA/internal dialect — status is the instance lifecycle status.
		{"spa completed+outcome", "COMPLETED", "SUCCESS", scheduledjob.InstanceStatusCompleted, "SUCCESS"},
		{"spa failed instance", "FAILED", "", scheduledjob.InstanceStatusFailed, ""},
		{"spa delivery_failed", "DELIVERY_FAILED", "", scheduledjob.InstanceStatusDeliveryFailed, ""},
		// Empty status defaults to COMPLETED.
		{"empty defaults completed", "", "", scheduledjob.InstanceStatusCompleted, ""},
		// Explicit completionStatus wins even alongside an outcome status.
		{"explicit completion wins", "SUCCESS", "FAILURE", scheduledjob.InstanceStatusCompleted, "FAILURE"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotStatus, gotCompletion := resolveInstanceCompletion(c.status, c.completion)
			if gotStatus != c.wantStatus {
				t.Errorf("status: got %q want %q", gotStatus, c.wantStatus)
			}
			if gotCompletion != c.wantCompletion {
				t.Errorf("completion: got %q want %q", gotCompletion, c.wantCompletion)
			}
		})
	}
}
