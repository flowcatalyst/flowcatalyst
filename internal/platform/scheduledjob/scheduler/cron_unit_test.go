package scheduler

import (
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
)

func TestRequestTimeout(t *testing.T) {
	cfg := DefaultConfig()
	sec := func(n int32) *scheduledjob.ScheduledJob { return &scheduledjob.ScheduledJob{TimeoutSeconds: &n} }
	cases := []struct {
		name string
		job  *scheduledjob.ScheduledJob
		want time.Duration
	}{
		{"unset uses default", &scheduledjob.ScheduledJob{}, 10 * time.Second},
		{"zero uses default", sec(0), 10 * time.Second},
		{"negative uses default", sec(-5), 10 * time.Second},
		{"job value honoured", sec(45), 45 * time.Second},
		{"above 10s default honoured", sec(120), 120 * time.Second},
		{"bounded at max", sec(100000), 300 * time.Second},
	}
	for _, tc := range cases {
		if got := cfg.requestTimeout(tc.job); got != tc.want {
			t.Errorf("%s: requestTimeout = %s, want %s", tc.name, got, tc.want)
		}
	}
	// Zero-value Config (tests, partial wiring) still yields the defaults.
	if got := (Config{}).requestTimeout(sec(100000)); got != 300*time.Second {
		t.Errorf("zero Config cap = %s, want 300s", got)
	}
	if got := cfg.inFlightGrace(); got != 360*time.Second {
		t.Errorf("inFlightGrace = %s, want 6m", got)
	}
}
