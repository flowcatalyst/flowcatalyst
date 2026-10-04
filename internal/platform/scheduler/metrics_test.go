package scheduler

import (
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// value reads a counter or gauge from the registry by metric name.
func value(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	families, err := reg.Gather()
	require.NoError(t, err)
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		m := f.GetMetric()[0]
		if c := m.GetCounter(); c != nil {
			return c.GetValue()
		}
		return m.GetGauge().GetValue()
	}
	t.Fatalf("metric %s not found", name)
	return 0
}

func TestObservePoll_CountsErrorsAndStampsOnlySuccess(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := newSchedulerMetrics(reg)

	m.observePoll(time.Now(), errors.New("boom"))
	assert.Equal(t, 1.0, value(t, reg, "fc_scheduler_poll_errors_total"))
	assert.Zero(t, value(t, reg, "fc_scheduler_last_successful_poll_timestamp_seconds"), "a failed poll is not a successful one")

	m.observePoll(time.Now(), nil)
	assert.Equal(t, 1.0, value(t, reg, "fc_scheduler_poll_errors_total"))
	assert.Greater(t, value(t, reg, "fc_scheduler_last_successful_poll_timestamp_seconds"), 0.0)
}
