package runner

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// metrics are the runner's Prometheus series (plan §6.2), in a registry of
// their own like every other subsystem's.
type metrics struct {
	reg         *prometheus.Registry
	invocations *prometheus.CounterVec
	duration    *prometheus.HistogramVec
}

func newMetrics() *metrics {
	m := &metrics{
		reg: prometheus.NewRegistry(),
		invocations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fc_fn_invocations_total",
			Help: "Function invocations by outcome (2xx..5xx from the guest, or a runner outcome such as busy, timeout, failed).",
		}, []string{"address", "version", "outcome"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fc_fn_duration_seconds",
			Help:    "Function invocation duration, including runner overhead.",
			Buckets: []float64{.0005, .001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		}, []string{"address", "version"}),
	}
	m.reg.MustRegister(m.invocations, m.duration)
	return m
}

func (m *metrics) observe(address string, version int, outcome string, start time.Time) {
	v := strconv.Itoa(version)
	m.invocations.WithLabelValues(address, v, outcome).Inc()
	m.duration.WithLabelValues(address, v).Observe(time.Since(start).Seconds())
}

// handler serves the counters plus point-in-time gauges read from the runner.
func (m *metrics) handler(r *Runner) http.Handler {
	gauges := prometheus.NewRegistry()
	gauges.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "fc_fn_memory_budget_bytes", Help: "Memory the runner may commit to guests.",
	}, func() float64 { return float64(r.budget.Stats().BudgetBytes) }))
	gauges.MustRegister(prometheus.NewGaugeFunc(prometheus.GaugeOpts{
		Name: "fc_fn_memory_used_bytes", Help: "Guest linear memory currently committed.",
	}, func() float64 { return float64(r.budget.Stats().UsedBytes) }))
	gauges.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "fc_fn_memory_refused_total", Help: "Guest memory charges refused by the budget.",
	}, func() float64 { return float64(r.budget.Stats().Refused) }))
	gauges.MustRegister(prometheus.NewCounterFunc(prometheus.CounterOpts{
		Name: "fc_fn_evictions_total", Help: "Versions whose compiled code was evicted (idle or memory pressure).",
	}, func() float64 { return float64(r.evictions.Load()) }))
	return promhttp.HandlerFor(prometheus.Gatherers{m.reg, gauges}, promhttp.HandlerOpts{ErrorHandling: promhttp.ContinueOnError})
}
