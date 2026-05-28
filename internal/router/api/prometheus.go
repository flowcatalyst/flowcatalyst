package api

import (
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/flowcatalyst/flowcatalyst-go/internal/router"
)

// PrometheusHandler returns an http.Handler that emits the router's
// metrics in Prometheus text exposition format. Every call collects a
// fresh snapshot — no background goroutine, no global mutable state.
//
// Mounted by cmd/fc-router/main.go at /metrics (and /q/metrics for the
// k8s-alias namespace).
//
// Gauges (point-in-time):
//   - fc_router_pool_active_workers{pool}
//   - fc_router_pool_queue_size{pool}
//   - fc_router_pool_concurrency{pool}
//   - fc_router_pool_message_groups{pool}
//   - fc_router_pool_rate_limited{pool}        (1/0)
//   - fc_router_queue_pending_messages{queue}
//   - fc_router_queue_in_flight_messages{queue}
//   - fc_router_circuit_breaker_open{target}   (1/0)
//   - fc_router_in_flight_total
//
// Counters (cumulative):
//   - fc_router_pool_messages_total{pool,outcome=success|failure|rate_limited}
//   - fc_router_queue_messages_total{queue,outcome=polled|acked|nacked|deferred}
//
// Latency:
//   - fc_router_pool_processing_time_ms_avg{pool}
//   - fc_router_pool_processing_time_ms{pool,quantile=0.5|0.95|0.99}
func PrometheusHandler(s *State) http.Handler {
	registry := prometheus.NewRegistry()
	collector := &routerCollector{state: s}
	registry.MustRegister(collector)

	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{
		ErrorLog:      nil,
		ErrorHandling: promhttp.ContinueOnError,
	})
}

type routerCollector struct {
	state *State
}

// Describe implements prometheus.Collector. The exposition format lets
// us emit metrics with descriptions assembled on the fly, so we leave
// this as a no-op — equivalent to the "untyped collector" pattern.
func (c *routerCollector) Describe(_ chan<- *prometheus.Desc) {}

// Collect builds one snapshot per scrape. Cheap: ~tens of µs.
func (c *routerCollector) Collect(ch chan<- prometheus.Metric) {
	c.collectPools(ch)
	c.collectQueues(ch)
	c.collectBreakers(ch)
	c.collectInFlight(ch)
}

func (c *routerCollector) collectPools(ch chan<- prometheus.Metric) {
	if c.state.PoolStats == nil {
		return
	}
	stats := c.state.PoolStats.PoolStats()

	for _, s := range stats {
		labels := []string{"pool"}
		labelValues := []string{s.PoolCode}

		gauge(ch, "fc_router_pool_active_workers",
			"Currently active worker goroutines per pool.",
			float64(s.ActiveWorkers), labels, labelValues)
		gauge(ch, "fc_router_pool_queue_size",
			"Messages buffered in group queues awaiting dispatch.",
			float64(s.QueueSize), labels, labelValues)
		gauge(ch, "fc_router_pool_concurrency",
			"Configured concurrency cap.",
			float64(s.Concurrency), labels, labelValues)
		gauge(ch, "fc_router_pool_message_groups",
			"Distinct message groups currently holding buffered work.",
			float64(s.MessageGroupCount), labels, labelValues)
		gauge(ch, "fc_router_pool_rate_limited",
			"1 if the pool's rate limiter has no tokens right now.",
			boolFloat(s.IsRateLimited), labels, labelValues)

		if s.Metrics == nil {
			continue
		}
		m := s.Metrics

		counter(ch, "fc_router_pool_messages_total",
			"Cumulative count of pool delivery outcomes.",
			float64(m.TotalSuccess),
			[]string{"pool", "outcome"}, []string{s.PoolCode, "success"})
		counter(ch, "fc_router_pool_messages_total",
			"Cumulative count of pool delivery outcomes.",
			float64(m.TotalFailure),
			[]string{"pool", "outcome"}, []string{s.PoolCode, "failure"})
		counter(ch, "fc_router_pool_messages_total",
			"Cumulative count of pool delivery outcomes.",
			float64(m.TotalRateLimited),
			[]string{"pool", "outcome"}, []string{s.PoolCode, "rate_limited"})

		gauge(ch, "fc_router_pool_processing_time_ms_avg",
			"Mean processing time (all-time) in milliseconds.",
			m.ProcessingTime.AvgMs, labels, labelValues)

		for q, v := range map[string]float64{
			"0.5":  float64(m.ProcessingTime.P50Ms),
			"0.95": float64(m.ProcessingTime.P95Ms),
			"0.99": float64(m.ProcessingTime.P99Ms),
		} {
			gauge(ch, "fc_router_pool_processing_time_ms",
				"All-time processing time percentile in milliseconds.",
				v, []string{"pool", "quantile"}, []string{s.PoolCode, q})
		}
	}
}

func (c *routerCollector) collectQueues(ch chan<- prometheus.Metric) {
	if c.state.BrokerStats == nil {
		return
	}
	for _, m := range c.state.BrokerStats.GetWindowed(0) {
		labels := []string{"queue"}
		labelValues := []string{normaliseQueueID(m.QueueIdentifier)}

		gauge(ch, "fc_router_queue_pending_messages",
			"Approximate messages waiting on the broker.",
			float64(m.PendingMessages), labels, labelValues)
		gauge(ch, "fc_router_queue_in_flight_messages",
			"Approximate messages currently being processed by consumers.",
			float64(m.InFlightMessages), labels, labelValues)

		for outcome, v := range map[string]uint64{
			"polled":   m.TotalPolled,
			"acked":    m.TotalAcked,
			"nacked":   m.TotalNacked,
			"deferred": m.TotalDeferred,
		} {
			counter(ch, "fc_router_queue_messages_total",
				"Cumulative count of consumer outcomes.",
				float64(v),
				[]string{"queue", "outcome"},
				[]string{normaliseQueueID(m.QueueIdentifier), outcome})
		}
	}
}

func (c *routerCollector) collectBreakers(ch chan<- prometheus.Metric) {
	if c.state.Breakers == nil {
		return
	}
	for name, st := range c.state.Breakers.Snapshot() {
		gauge(ch, "fc_router_circuit_breaker_open",
			"1 when the breaker is OPEN, 0 otherwise.",
			boolFloat(st.State == router.CircuitOpen),
			[]string{"target"}, []string{name})
		counter(ch, "fc_router_circuit_breaker_calls_total",
			"Cumulative breaker outcomes.",
			float64(st.Successes),
			[]string{"target", "outcome"}, []string{name, "success"})
		counter(ch, "fc_router_circuit_breaker_calls_total",
			"Cumulative breaker outcomes.",
			float64(st.Failures),
			[]string{"target", "outcome"}, []string{name, "failure"})
	}
}

func (c *routerCollector) collectInFlight(ch chan<- prometheus.Metric) {
	if c.state.InFlight == nil {
		return
	}
	count := len(c.state.InFlight.Snapshot())
	gauge(ch, "fc_router_in_flight_total",
		"Total in-flight messages across all pools.",
		float64(count), nil, nil)
}

// gauge emits a single typed gauge metric.
func gauge(ch chan<- prometheus.Metric, name, help string, value float64, labels, labelValues []string) {
	desc := prometheus.NewDesc(name, help, labels, nil)
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, labelValues...)
}

// counter emits a single typed counter metric (cumulative).
func counter(ch chan<- prometheus.Metric, name, help string, value float64, labels, labelValues []string) {
	desc := prometheus.NewDesc(name, help, labels, nil)
	ch <- prometheus.MustNewConstMetric(desc, prometheus.CounterValue, value, labelValues...)
}

func boolFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

// normaliseQueueID trims AWS SQS URL prefixes so the label cardinality
// stays bounded (we want `my-queue`, not the full `https://sqs.../my-queue`).
func normaliseQueueID(id string) string {
	if i := strings.LastIndex(id, "/"); i >= 0 && i < len(id)-1 {
		return id[i+1:]
	}
	return id
}
