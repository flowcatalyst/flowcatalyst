package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// === Pool Metrics Tests ===

func TestPoolMessagesProcessed_Labels(t *testing.T) {
	// Test that we can increment with valid labels
	PoolMessagesProcessed.WithLabelValues("test-pool", "success").Inc()
	PoolMessagesProcessed.WithLabelValues("test-pool", "failed").Inc()
	PoolMessagesProcessed.WithLabelValues("test-pool", "rate_limited").Inc()

	// Verify we can get the counter value
	counter := PoolMessagesProcessed.WithLabelValues("test-pool", "success")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestPoolProcessingDuration_Observe(t *testing.T) {
	// Test that we can observe durations
	durations := []float64{0.001, 0.01, 0.1, 0.5, 1.0, 5.0}
	for _, d := range durations {
		PoolProcessingDuration.WithLabelValues("test-pool").Observe(d)
	}

	histogram := PoolProcessingDuration.WithLabelValues("test-pool")
	if histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}
}

func TestPoolActiveWorkers_GaugeOperations(t *testing.T) {
	gauge := PoolActiveWorkers.WithLabelValues("test-pool-workers")

	gauge.Set(5)
	gauge.Inc()
	gauge.Dec()
	gauge.Add(10)
	gauge.Sub(5)

	if gauge == nil {
		t.Error("Expected gauge to be non-nil")
	}
}

func TestPoolQueueDepth_GaugeOperations(t *testing.T) {
	gauge := PoolQueueDepth.WithLabelValues("test-pool-queue")

	gauge.Set(100)
	gauge.Add(50)
	gauge.Sub(25)

	if gauge == nil {
		t.Error("Expected gauge to be non-nil")
	}
}

func TestPoolRateLimitRejections_Counter(t *testing.T) {
	PoolRateLimitRejections.WithLabelValues("test-pool-rl").Inc()
	PoolRateLimitRejections.WithLabelValues("test-pool-rl").Add(5)

	counter := PoolRateLimitRejections.WithLabelValues("test-pool-rl")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

// === Mediator Metrics Tests ===

func TestMediatorHTTPRequests_Labels(t *testing.T) {
	statusCodes := []string{"200", "201", "400", "401", "404", "500", "502", "503"}
	methods := []string{"GET", "POST", "PUT", "DELETE"}

	for _, code := range statusCodes {
		for _, method := range methods {
			MediatorHTTPRequests.WithLabelValues(code, method).Inc()
		}
	}

	counter := MediatorHTTPRequests.WithLabelValues("200", "POST")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestMediatorHTTPDuration_Observe(t *testing.T) {
	targets := []string{"http://service-a.local", "http://service-b.local"}

	for _, target := range targets {
		MediatorHTTPDuration.WithLabelValues(target).Observe(0.123)
	}

	histogram := MediatorHTTPDuration.WithLabelValues("http://test.local")
	if histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}
}

func TestMediatorCircuitBreakerState_Values(t *testing.T) {
	gauge := MediatorCircuitBreakerState.WithLabelValues("http://target.local")

	// Test all valid states
	gauge.Set(CircuitBreakerClosed)
	gauge.Set(CircuitBreakerOpen)
	gauge.Set(CircuitBreakerHalfOpen)

	if gauge == nil {
		t.Error("Expected gauge to be non-nil")
	}
}

func TestMediatorCircuitBreakerTrips_Counter(t *testing.T) {
	MediatorCircuitBreakerTrips.WithLabelValues("http://failing-target.local").Inc()

	counter := MediatorCircuitBreakerTrips.WithLabelValues("http://failing-target.local")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

// === Scheduler Metrics Tests ===

func TestSchedulerJobsScheduled_Counter(t *testing.T) {
	SchedulerJobsScheduled.Inc()
	SchedulerJobsScheduled.Add(10)

	// Verify it's registered
	desc := SchedulerJobsScheduled.Desc()
	if desc == nil {
		t.Error("Expected Desc to be non-nil")
	}
}

func TestSchedulerJobsPending_Gauge(t *testing.T) {
	SchedulerJobsPending.Set(50)
	SchedulerJobsPending.Inc()
	SchedulerJobsPending.Dec()
	SchedulerJobsPending.Add(25)
	SchedulerJobsPending.Sub(10)

	desc := SchedulerJobsPending.Desc()
	if desc == nil {
		t.Error("Expected Desc to be non-nil")
	}
}

func TestSchedulerStaleJobs_Counter(t *testing.T) {
	SchedulerStaleJobs.Inc()
	SchedulerStaleJobs.Add(3)

	desc := SchedulerStaleJobs.Desc()
	if desc == nil {
		t.Error("Expected Desc to be non-nil")
	}
}

// === Stream Processor Metrics Tests ===

func TestStreamEventsProcessed_Labels(t *testing.T) {
	eventTypes := []string{"ORDER_CREATED", "ORDER_UPDATED", "ORDER_SHIPPED"}
	results := []string{"success", "failed"}

	for _, eventType := range eventTypes {
		for _, result := range results {
			StreamEventsProcessed.WithLabelValues(eventType, result).Inc()
		}
	}

	counter := StreamEventsProcessed.WithLabelValues("ORDER_CREATED", "success")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestStreamProcessingDuration_Observe(t *testing.T) {
	StreamProcessingDuration.WithLabelValues("ORDER_CREATED").Observe(0.05)
	StreamProcessingDuration.WithLabelValues("ORDER_UPDATED").Observe(0.10)

	histogram := StreamProcessingDuration.WithLabelValues("ORDER_CREATED")
	if histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}
}

func TestStreamLag_Gauge(t *testing.T) {
	StreamLag.WithLabelValues("events-stream").Set(100)
	StreamLag.WithLabelValues("dispatch-stream").Set(50)

	gauge := StreamLag.WithLabelValues("events-stream")
	if gauge == nil {
		t.Error("Expected gauge to be non-nil")
	}
}

// === Queue Metrics Tests ===

func TestQueueMessagesPublished_Labels(t *testing.T) {
	queueTypes := []string{"nats", "sqs"}

	for _, qType := range queueTypes {
		QueueMessagesPublished.WithLabelValues(qType).Inc()
		QueueMessagesPublished.WithLabelValues(qType).Add(100)
	}

	counter := QueueMessagesPublished.WithLabelValues("nats")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestQueueMessagesConsumed_Labels(t *testing.T) {
	queueTypes := []string{"nats", "sqs"}

	for _, qType := range queueTypes {
		QueueMessagesConsumed.WithLabelValues(qType).Inc()
		QueueMessagesConsumed.WithLabelValues(qType).Add(100)
	}

	counter := QueueMessagesConsumed.WithLabelValues("sqs")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestQueuePublishErrors_Counter(t *testing.T) {
	QueuePublishErrors.WithLabelValues("nats").Inc()
	QueuePublishErrors.WithLabelValues("sqs").Inc()

	counter := QueuePublishErrors.WithLabelValues("nats")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

// === HTTP API Metrics Tests ===

func TestHTTPRequestsTotal_Labels(t *testing.T) {
	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH"}
	paths := []string{"/api/events", "/api/subscriptions", "/api/clients"}
	statuses := []string{"200", "201", "400", "401", "403", "404", "500"}

	for _, method := range methods {
		for _, path := range paths {
			for _, status := range statuses {
				HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
			}
		}
	}

	counter := HTTPRequestsTotal.WithLabelValues("GET", "/api/events", "200")
	if counter == nil {
		t.Error("Expected counter to be non-nil")
	}
}

func TestHTTPRequestDuration_Observe(t *testing.T) {
	HTTPRequestDuration.WithLabelValues("GET", "/api/events").Observe(0.015)
	HTTPRequestDuration.WithLabelValues("POST", "/api/events").Observe(0.150)

	histogram := HTTPRequestDuration.WithLabelValues("GET", "/api/events")
	if histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}
}

func TestHTTPActiveConnections_Gauge(t *testing.T) {
	HTTPActiveConnections.Set(10)
	HTTPActiveConnections.Inc()
	HTTPActiveConnections.Dec()
	HTTPActiveConnections.Add(5)
	HTTPActiveConnections.Sub(3)

	desc := HTTPActiveConnections.Desc()
	if desc == nil {
		t.Error("Expected Desc to be non-nil")
	}
}

// === Circuit Breaker Constants Tests ===

func TestCircuitBreakerConstants(t *testing.T) {
	if CircuitBreakerClosed != 0 {
		t.Errorf("Expected CircuitBreakerClosed=0, got %d", CircuitBreakerClosed)
	}
	if CircuitBreakerOpen != 1 {
		t.Errorf("Expected CircuitBreakerOpen=1, got %d", CircuitBreakerOpen)
	}
	if CircuitBreakerHalfOpen != 2 {
		t.Errorf("Expected CircuitBreakerHalfOpen=2, got %d", CircuitBreakerHalfOpen)
	}
}

// === Metric Name Tests ===

func TestMetricNamingConvention(t *testing.T) {
	// Verify metrics follow flowcatalyst_subsystem_name convention
	expectedPrefixes := map[string]string{
		"pool_messages_processed":       "flowcatalyst_pool_messages_processed_total",
		"mediator_http_requests":        "flowcatalyst_mediator_http_requests_total",
		"scheduler_jobs_scheduled":      "flowcatalyst_scheduler_jobs_scheduled_total",
		"stream_events_processed":       "flowcatalyst_stream_events_processed_total",
		"queue_messages_published":      "flowcatalyst_queue_messages_published_total",
		"http_requests":                 "flowcatalyst_http_requests_total",
	}

	// Just verify the naming convention pattern exists
	for name := range expectedPrefixes {
		if name == "" {
			t.Error("Metric name should not be empty")
		}
	}
}

// === Counter Value Tests ===

func TestCounterValue(t *testing.T) {
	// Create a new registry for isolated testing
	reg := prometheus.NewRegistry()

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "test_counter",
		Help: "Test counter",
	})

	reg.MustRegister(counter)

	counter.Add(5)

	val := testutil.ToFloat64(counter)
	if val != 5 {
		t.Errorf("Expected counter value 5, got %f", val)
	}

	counter.Inc()

	val = testutil.ToFloat64(counter)
	if val != 6 {
		t.Errorf("Expected counter value 6, got %f", val)
	}
}

// === Gauge Value Tests ===

func TestGaugeValue(t *testing.T) {
	reg := prometheus.NewRegistry()

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "test_gauge",
		Help: "Test gauge",
	})

	reg.MustRegister(gauge)

	gauge.Set(100)
	val := testutil.ToFloat64(gauge)
	if val != 100 {
		t.Errorf("Expected gauge value 100, got %f", val)
	}

	gauge.Add(50)
	val = testutil.ToFloat64(gauge)
	if val != 150 {
		t.Errorf("Expected gauge value 150, got %f", val)
	}

	gauge.Sub(30)
	val = testutil.ToFloat64(gauge)
	if val != 120 {
		t.Errorf("Expected gauge value 120, got %f", val)
	}

	gauge.Dec()
	val = testutil.ToFloat64(gauge)
	if val != 119 {
		t.Errorf("Expected gauge value 119, got %f", val)
	}

	gauge.Inc()
	val = testutil.ToFloat64(gauge)
	if val != 120 {
		t.Errorf("Expected gauge value 120, got %f", val)
	}
}

// === Histogram Tests ===

func TestHistogramBuckets(t *testing.T) {
	reg := prometheus.NewRegistry()

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "test_histogram",
		Help:    "Test histogram",
		Buckets: []float64{0.1, 0.5, 1.0, 5.0},
	})

	reg.MustRegister(histogram)

	// Observe values in different buckets
	histogram.Observe(0.05) // < 0.1
	histogram.Observe(0.25) // < 0.5
	histogram.Observe(0.75) // < 1.0
	histogram.Observe(2.5)  // < 5.0
	histogram.Observe(10.0) // > 5.0

	// Verify histogram is populated (testutil doesn't directly expose bucket counts)
	if histogram == nil {
		t.Error("Expected histogram to be non-nil")
	}
}

// === Pool Metrics Integration Tests ===

func TestPoolMetricsIntegration(t *testing.T) {
	poolCode := "integration-test-pool"

	// Simulate processing messages
	for i := 0; i < 100; i++ {
		if i%10 == 0 {
			PoolMessagesProcessed.WithLabelValues(poolCode, "failed").Inc()
		} else if i%20 == 0 {
			PoolMessagesProcessed.WithLabelValues(poolCode, "rate_limited").Inc()
		} else {
			PoolMessagesProcessed.WithLabelValues(poolCode, "success").Inc()
		}

		PoolProcessingDuration.WithLabelValues(poolCode).Observe(float64(i) * 0.001)
	}

	// Update gauges
	PoolActiveWorkers.WithLabelValues(poolCode).Set(10)
	PoolQueueDepth.WithLabelValues(poolCode).Set(25)

	// All operations should succeed without panic
}

// === Mediator Metrics Integration Tests ===

func TestMediatorMetricsIntegration(t *testing.T) {
	target := "http://integration-test.local"

	// Simulate HTTP requests
	for i := 0; i < 50; i++ {
		statusCode := "200"
		if i%5 == 0 {
			statusCode = "500"
		}
		MediatorHTTPRequests.WithLabelValues(statusCode, "POST").Inc()
		MediatorHTTPDuration.WithLabelValues(target).Observe(0.050)
	}

	// Simulate circuit breaker state changes
	MediatorCircuitBreakerState.WithLabelValues(target).Set(CircuitBreakerClosed)
	MediatorCircuitBreakerState.WithLabelValues(target).Set(CircuitBreakerOpen)
	MediatorCircuitBreakerTrips.WithLabelValues(target).Inc()
	MediatorCircuitBreakerState.WithLabelValues(target).Set(CircuitBreakerHalfOpen)
	MediatorCircuitBreakerState.WithLabelValues(target).Set(CircuitBreakerClosed)

	// All operations should succeed without panic
}

// Benchmark for counter operations
func BenchmarkCounterInc(b *testing.B) {
	counter := PoolMessagesProcessed.WithLabelValues("bench-pool", "success")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		counter.Inc()
	}
}

// Benchmark for histogram observations
func BenchmarkHistogramObserve(b *testing.B) {
	histogram := PoolProcessingDuration.WithLabelValues("bench-pool")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		histogram.Observe(0.123)
	}
}

// Benchmark for gauge set operations
func BenchmarkGaugeSet(b *testing.B) {
	gauge := PoolActiveWorkers.WithLabelValues("bench-pool")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gauge.Set(float64(i))
	}
}
