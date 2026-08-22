package router

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestDeferredDelayCurve(t *testing.T) {
	// No hint from the target: pure ramp from 5s, capped at 60s.
	assert.Equal(t, 5*time.Second, deferredDelay(0, 0))
	assert.Equal(t, 10*time.Second, deferredDelay(1, 0))
	assert.Equal(t, 40*time.Second, deferredDelay(3, 0))
	assert.Equal(t, time.Minute, deferredDelay(4, 0), "80s ramp hits the 60s cap")
	assert.Equal(t, time.Minute, deferredDelay(12, 0))

	// Target-requested delaySeconds floors the ramp…
	assert.Equal(t, 30*time.Second, deferredDelay(0, 30))
	assert.Equal(t, 40*time.Second, deferredDelay(3, 30), "ramp above the floor wins")
	// …but never lifts the deferred cap.
	assert.Equal(t, time.Minute, deferredDelay(0, 300))
}

func TestRetryDelayKeepsTheErrorCurve(t *testing.T) {
	// Error/429/circuit-open backoff is unchanged: 100ms ramp, 5-minute cap,
	// server hint as a floor.
	assert.Equal(t, 100*time.Millisecond, retryDelay(0, 0))
	assert.Equal(t, 30*time.Second, retryDelay(0, 30))
	assert.Equal(t, 5*time.Minute, retryDelay(12, 0))
	assert.Equal(t, 4*time.Minute, retryDelay(0, 240), "hints above 60s still honoured here")
}
