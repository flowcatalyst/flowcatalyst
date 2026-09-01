package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestStallDetectorReportsOncePerEpisode pins the dedup that had to come with
// routing stall reports into the WarningService. The detector re-scans every
// tick, so without dedup one wedged message adds a warning a minute — and
// since the active-warning count drives the DEGRADED threshold (20), a handful
// of long deliveries would put the router into DEGRADED purely for doing
// contractual work.
func TestStallDetectorReportsOncePerEpisode(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	tr := NewInFlightTracker()
	msg := common.Message{ID: "msg_wedged", PoolCode: "P"}
	im := common.NewInFlightMessage(&msg, "b1", "q", "", "rh")
	require.Equal(t, RegisterNew, tr.Register(im))
	im.StartedAt = time.Now().Add(-time.Hour) // well past any threshold

	d := NewStallDetector(DefaultStallConfig(), tr, nil, nil)
	d.SetWarnings(ws)

	d.tick(context.Background())
	require.Len(t, ws.All(), 1, "a stalled message must be reported")
	d.tick(context.Background())
	d.tick(context.Background())
	assert.Len(t, ws.All(), 1, "re-scanning must not re-report the same episode")

	// Once it leaves the pipeline the id is forgotten, so a later stall of the
	// same message reports again rather than being silenced for ever.
	tr.Remove(msg.ID, "b1")
	d.tick(context.Background())
	im2 := common.NewInFlightMessage(&msg, "b1", "q", "", "rh")
	require.Equal(t, RegisterNew, tr.Register(im2))
	im2.StartedAt = time.Now().Add(-time.Hour)
	d.tick(context.Background())
	assert.Len(t, ws.All(), 2, "a fresh stall episode must report again")
}

// The stall thresholds are derived from the mediation contract, not picked
// freely: warning below one attempt flags deliveries that are going to
// succeed, and force-NACKing inside the three-attempt budget hands the broker
// a second copy of one that is still running.
func TestStallThresholdsRespectTheMediationContract(t *testing.T) {
	attempt := DefaultMediatorConfig().Timeout
	cfg := DefaultStallConfig()

	assert.Greater(t, cfg.StallThresholdSeconds, uint64(attempt.Seconds()),
		"warning below one full attempt flags deliveries that are going to succeed")
	assert.Greater(t, cfg.ForceNackAfterSeconds, uint64(3*attempt.Seconds()),
		"force-NACK inside the 3-attempt budget would duplicate a live delivery")
	assert.False(t, cfg.ForceNackStalled,
		"force-NACK must stay opt-in under a long mediation contract")
}

// TestSaturatedPoolIsReportedOncePerEpisode covers the condition that had no
// signal at all: a pool holding zero free slots with one very old delivery is,
// at low concurrency, blocking every message group it owns. A healthy busy
// pool looks identical on activeWorkers alone, so the age is what separates
// them — and the grace is one full attempt, since a delivery cannot legitimately
// outlive the client timeout.
func TestSaturatedPoolIsReportedOncePerEpisode(t *testing.T) {
	ws := NewWarningService(WarningServiceConfig{})
	l := NewLifecycleManager(DefaultLifecycleConfig(), ws, NewHealthService(DefaultHealthServiceConfig(), ws))
	over := uint64((poolSaturationGrace + time.Minute).Milliseconds())

	// Busy but turning over: not a wedge.
	l.reportSaturatedPools([]PoolStats{{PoolCode: "fast", Concurrency: 1, ActiveWorkers: 1, OldestMediatingMs: 500}})
	assert.Empty(t, ws.All(), "a saturated pool that is turning over must not warn")

	// Free slots, but one long delivery: contractual, not a wedge.
	l.reportSaturatedPools([]PoolStats{{PoolCode: "roomy", Concurrency: 4, ActiveWorkers: 1, OldestMediatingMs: over}})
	assert.Empty(t, ws.All(), "a pool with free slots is not blocked")

	// No free slots AND older than a whole attempt: wedged.
	wedged := []PoolStats{{PoolCode: "stuck", Concurrency: 1, ActiveWorkers: 1, OldestMediatingMs: over}}
	l.reportSaturatedPools(wedged)
	require.Len(t, ws.All(), 1)
	l.reportSaturatedPools(wedged)
	assert.Len(t, ws.All(), 1, "an ongoing episode must not re-warn every tick")

	// Recovered, then wedged again → reports again.
	l.reportSaturatedPools([]PoolStats{{PoolCode: "stuck", Concurrency: 1, ActiveWorkers: 0}})
	l.reportSaturatedPools(wedged)
	assert.Len(t, ws.All(), 2, "a fresh episode must report again")
}
