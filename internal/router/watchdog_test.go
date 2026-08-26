package router

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// testNotifier is a Notifier with no webhook and a batch big enough that
// nothing auto-flushes, so a test can read back what was raised.
func testNotifier() *Notifier { return NewNotifier("", 1000, time.Hour) }

func warningsRaised(n *Notifier) []Warning {
	n.mu.Lock()
	defer n.mu.Unlock()
	return append([]Warning(nil), n.queue...)
}

func trackedFor(t *testing.T, tr *InFlightTracker, id string, age time.Duration, attempts uint) {
	t.Helper()
	im := common.NewInFlightMessage(&common.Message{ID: id, PoolCode: "P"}, "bk-"+id, "q1", "batch", "rh-"+id)
	im.StartedAt = time.Now().Add(-age)
	im.LastSeenAt = im.StartedAt
	im.Attempts = attempts
	require.Equal(t, RegisterNew, tr.Register(im))
}

// --- stall detector ---

func TestStallDetectorWarnsAndForceNacksIdleMessages(t *testing.T) {
	tr := NewInFlightTracker()
	trackedFor(t, tr, "stuck", 10*time.Minute, 0)
	trackedFor(t, tr, "fresh", time.Second, 0)

	notifier := testNotifier()
	var nacked []string
	cfg := StallConfig{
		Enabled: true, StallThresholdSeconds: 300,
		ForceNackStalled: true, ForceNackAfterSeconds: 300, NackDelaySeconds: 30,
	}
	d := NewStallDetector(cfg, tr, notifier, func(_ context.Context, _, _ string, _ uint32) error {
		nacked = append(nacked, "stuck")
		return nil
	})

	d.tick(context.Background())

	assert.Equal(t, []string{"stuck"}, nacked, "only the message past the threshold is force-NACKed")
	assert.Len(t, warningsRaised(notifier), 1)
	_, stillTracked := tr.Lookup("stuck")
	assert.False(t, stillTracked, "a force-NACKed message is released so it isn't NACKed again next tick")
	_, fresh := tr.Lookup("fresh")
	assert.True(t, fresh, "a young message is left alone")
}

// A message being retried in-pipeline used to be skipped outright, so the one
// failure that can persist for minutes — an endpoint deferring every attempt —
// was the one nothing ever reported. It must warn now, but must still never be
// force-NACKed: yanking it away from a live retry would hand the broker a
// second copy while this one is still running.
func TestStallDetectorWarnsAboutLongRetriesButNeverNacksThem(t *testing.T) {
	tr := NewInFlightTracker()
	trackedFor(t, tr, "retrying", 10*time.Minute, 4)

	notifier := testNotifier()
	nacked := 0
	cfg := StallConfig{
		Enabled: true, StallThresholdSeconds: 300,
		ForceNackStalled: true, ForceNackAfterSeconds: 300, NackDelaySeconds: 30,
	}
	d := NewStallDetector(cfg, tr, notifier, func(context.Context, string, string, uint32) error {
		nacked++
		return nil
	})

	d.tick(context.Background())

	require.Len(t, warningsRaised(notifier), 1, "a long-retrying message must be reported")
	assert.Contains(t, warningsRaised(notifier)[0].Message, "retrying in-pipeline")
	assert.Equal(t, 0, nacked, "a message a worker is actively retrying must never be force-NACKed")
	_, stillTracked := tr.Lookup("retrying")
	assert.True(t, stillTracked, "and its entry must survive — the retry still owns it")
}

func TestStallDetectorForceNackDisabledStillWarns(t *testing.T) {
	tr := NewInFlightTracker()
	trackedFor(t, tr, "stuck", 10*time.Minute, 0)
	notifier := testNotifier()
	d := NewStallDetector(StallConfig{Enabled: true, StallThresholdSeconds: 300}, tr, notifier, nil)

	d.tick(context.Background())

	assert.Len(t, warningsRaised(notifier), 1)
	_, stillTracked := tr.Lookup("stuck")
	assert.True(t, stillTracked, "with force-NACK off the entry stays put")
}

// --- queue-health monitor ---

// metricsConsumer reports a scripted queue depth, one value per tick.
type metricsConsumer struct {
	queue.Consumer
	name   string
	depths []uint64
	i      int
}

func (c *metricsConsumer) Identifier() string { return c.name }
func (c *metricsConsumer) Metrics(context.Context) (*queue.Metrics, error) {
	d := c.depths[min(c.i, len(c.depths)-1)]
	c.i++
	return &queue.Metrics{QueueIdentifier: c.name, PendingMessages: d}, nil
}

func TestQueueHealthWarnsOnBacklog(t *testing.T) {
	notifier := testNotifier()
	cfg := DefaultQueueHealthConfig()
	m := NewQueueHealthMonitor(cfg, notifier)

	quiet := &metricsConsumer{name: "q-quiet", depths: []uint64{cfg.BacklogThreshold}}
	deep := &metricsConsumer{name: "q-deep", depths: []uint64{cfg.BacklogThreshold + 1}}
	m.tick(context.Background(), []queue.Consumer{quiet, deep})

	raised := warningsRaised(notifier)
	require.Len(t, raised, 1, "only a queue OVER the threshold warns")
	assert.Contains(t, raised[0].Message, "q-deep")
	assert.Equal(t, WarningCategoryQueueHealth, raised[0].Category)
}

// Sustained growth is the more interesting signal: a queue can sit below the
// backlog threshold and still be losing ground.
func TestQueueHealthWarnsOnSustainedGrowthOnly(t *testing.T) {
	notifier := testNotifier()
	cfg := QueueHealthConfig{
		Enabled: true, BacklogThreshold: 1_000_000,
		GrowthThreshold: 100, GrowthPeriodsThreshold: 3,
	}
	m := NewQueueHealthMonitor(cfg, notifier)
	c := &metricsConsumer{name: "q-growing", depths: []uint64{0, 200, 400, 600, 800}}

	for range 3 { // 0 → 200 → 400: two growth periods, one short of the threshold
		m.tick(context.Background(), []queue.Consumer{c})
	}
	assert.Empty(t, warningsRaised(notifier), "growth must be sustained before it warns")

	m.tick(context.Background(), []queue.Consumer{c}) // third consecutive period
	raised := warningsRaised(notifier)
	require.Len(t, raised, 1)
	assert.Contains(t, raised[0].Message, "consecutive periods")

	// A period that doesn't grow resets the run (600 is the depth the last
	// tick recorded, so this period adds nothing).
	flat := &metricsConsumer{name: "q-growing", depths: []uint64{600}}
	m.tick(context.Background(), []queue.Consumer{flat})
	m.mu.Lock()
	periods := m.history["q-growing"].consecutiveGrowthPeriods
	m.mu.Unlock()
	assert.Zero(t, periods, "a flat period resets the growth run")
}
