package stream

import (
	"errors"
	"testing"
	"time"
)

// nextSleep mirrors Rust's adaptive_sleep tiers: a full batch loops with no
// sleep so a backlog drains at full speed, a partial batch pauses briefly, an
// empty poll idles, and a Step error backs off hardest.
func TestNextSleep_AdaptiveTiers(t *testing.T) {
	cfg := ProjectorConfig{
		BatchSize:    100,
		PollInterval: 100 * time.Millisecond,
		IdleSleep:    1 * time.Second,
		ErrorSleep:   5 * time.Second,
	}
	cases := []struct {
		name string
		n    int
		err  error
		want time.Duration
	}{
		{"error backs off hardest", 0, errors.New("boom"), cfg.ErrorSleep},
		{"error wins even with rows", 50, errors.New("boom"), cfg.ErrorSleep},
		{"empty poll idles", 0, nil, cfg.IdleSleep},
		{"full batch loops immediately", 100, nil, 0},
		{"over-full batch loops immediately", 150, nil, 0},
		{"partial batch pauses", 42, nil, cfg.PollInterval},
	}
	for _, tc := range cases {
		if got := nextSleep(cfg, tc.n, tc.err); got != tc.want {
			t.Errorf("%s: nextSleep(n=%d,err=%v) = %v, want %v", tc.name, tc.n, tc.err, got, tc.want)
		}
	}
}
