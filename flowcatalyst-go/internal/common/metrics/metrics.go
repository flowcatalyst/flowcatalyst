package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	// Pool metrics

	// PoolMessagesProcessed tracks total messages processed by pool
	PoolMessagesProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "messages_processed_total",
			Help:      "Total messages processed by dispatch pool",
		},
		[]string{"pool_code", "result"}, // result: success, failed, rate_limited
	)

	// PoolProcessingDuration tracks message processing duration
	PoolProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "processing_duration_seconds",
			Help:      "Time to process a message",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"pool_code"},
	)

	// PoolActiveWorkers tracks number of active workers
	PoolActiveWorkers = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "active_workers",
			Help:      "Number of active workers in the pool",
		},
		[]string{"pool_code"},
	)

	// PoolQueueDepth tracks queue depth (pending messages)
	PoolQueueDepth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "queue_depth",
			Help:      "Number of messages pending in the pool queue",
		},
		[]string{"pool_code"},
	)

	// PoolRateLimitRejections tracks rate limit rejections
	PoolRateLimitRejections = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "rate_limit_rejections_total",
			Help:      "Total messages rejected due to rate limiting",
		},
		[]string{"pool_code"},
	)

	// PoolAvailablePermits tracks available concurrency permits
	PoolAvailablePermits = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "available_permits",
			Help:      "Available concurrency permits in the pool",
		},
		[]string{"pool_code"},
	)

	// PoolMessageGroupCount tracks active message groups
	PoolMessageGroupCount = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pool",
			Name:      "message_group_count",
			Help:      "Number of active message groups in the pool",
		},
		[]string{"pool_code"},
	)

	// Mediator metrics

	// MediatorHTTPRequests tracks HTTP requests made by the mediator
	MediatorHTTPRequests = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "mediator",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests made by the mediator",
		},
		[]string{"status_code", "method"},
	)

	// MediatorHTTPDuration tracks HTTP request duration
	MediatorHTTPDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "mediator",
			Name:      "http_duration_seconds",
			Help:      "HTTP request duration",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10},
		},
		[]string{"target"},
	)

	// MediatorCircuitBreakerState tracks circuit breaker state
	// 0 = closed (healthy), 1 = open (tripped), 2 = half-open (testing)
	MediatorCircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "mediator",
			Name:      "circuit_breaker_state",
			Help:      "Circuit breaker state (0=closed, 1=open, 2=half-open)",
		},
		[]string{"target"},
	)

	// MediatorCircuitBreakerTrips tracks circuit breaker trip events
	MediatorCircuitBreakerTrips = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "mediator",
			Name:      "circuit_breaker_trips_total",
			Help:      "Total circuit breaker trip events",
		},
		[]string{"target"},
	)

	// Scheduler metrics

	// SchedulerJobsScheduled tracks total jobs scheduled
	SchedulerJobsScheduled = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "scheduler",
			Name:      "jobs_scheduled_total",
			Help:      "Total jobs scheduled for dispatch",
		},
	)

	// SchedulerJobsPending tracks pending jobs
	SchedulerJobsPending = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "scheduler",
			Name:      "jobs_pending",
			Help:      "Number of jobs pending dispatch",
		},
	)

	// SchedulerStaleJobs tracks stale jobs recovered
	SchedulerStaleJobs = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "scheduler",
			Name:      "stale_jobs_recovered_total",
			Help:      "Total stale jobs recovered",
		},
	)

	// Stream processor metrics

	// StreamEventsProcessed tracks events processed by stream processor
	StreamEventsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "stream",
			Name:      "events_processed_total",
			Help:      "Total events processed by stream processor",
		},
		[]string{"event_type", "result"}, // result: success, failed
	)

	// StreamProcessingDuration tracks event processing duration
	StreamProcessingDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "stream",
			Name:      "processing_duration_seconds",
			Help:      "Time to process an event",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"event_type"},
	)

	// StreamLag tracks stream consumer lag
	StreamLag = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "stream",
			Name:      "consumer_lag",
			Help:      "Number of messages behind in the stream",
		},
		[]string{"stream_name"},
	)

	// Queue metrics

	// QueueMessagesPublished tracks messages published to queue
	QueueMessagesPublished = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "queue",
			Name:      "messages_published_total",
			Help:      "Total messages published to queue",
		},
		[]string{"queue_type"}, // nats, sqs
	)

	// QueueMessagesConsumed tracks messages consumed from queue
	QueueMessagesConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "queue",
			Name:      "messages_consumed_total",
			Help:      "Total messages consumed from queue",
		},
		[]string{"queue_type"}, // nats, sqs
	)

	// QueuePublishErrors tracks queue publish errors
	QueuePublishErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "queue",
			Name:      "publish_errors_total",
			Help:      "Total queue publish errors",
		},
		[]string{"queue_type"},
	)

	// Consumer health metrics

	// ConsumerRestarts tracks consumer restart attempts
	ConsumerRestarts = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "consumer",
			Name:      "restarts_total",
			Help:      "Total consumer restart attempts due to stall detection",
		},
	)

	// ConsumerStallEvents tracks consumer stall events
	ConsumerStallEvents = promauto.NewCounter(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "consumer",
			Name:      "stall_events_total",
			Help:      "Total consumer stall events detected",
		},
	)

	// Pipeline metrics (for leak detection, matching Java gauges)

	// PipelineMapSize tracks the size of the in-pipeline map
	PipelineMapSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pipeline",
			Name:      "map_size",
			Help:      "Number of messages currently in the processing pipeline",
		},
	)

	// PipelineTotalCapacity tracks total pool capacity
	PipelineTotalCapacity = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "pipeline",
			Name:      "total_capacity",
			Help:      "Total capacity across all processing pools",
		},
	)

	// Outbox processor metrics

	// OutboxItemsProcessed tracks total outbox items processed
	OutboxItemsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "items_processed_total",
			Help:      "Total outbox items processed",
		},
		[]string{"type", "status"}, // type: event, dispatch_job; status: completed, failed, retried
	)

	// OutboxBufferSize tracks current buffer size
	OutboxBufferSize = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "buffer_size",
			Help:      "Current size of the outbox buffer",
		},
	)

	// OutboxActiveProcessors tracks active message group processors
	OutboxActiveProcessors = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "active_processors",
			Help:      "Number of active message group processors",
		},
	)

	// OutboxPollDuration tracks outbox polling duration
	OutboxPollDuration = promauto.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "poll_duration_seconds",
			Help:      "Time to poll and process an outbox batch",
			Buckets:   prometheus.DefBuckets,
		},
	)

	// OutboxAPIDuration tracks API call duration for outbox item delivery
	OutboxAPIDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "api_duration_seconds",
			Help:      "Time to deliver outbox items via API",
			Buckets:   []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30},
		},
		[]string{"type"}, // event, dispatch_job
	)

	// OutboxRecoveredItems tracks items recovered from stuck PROCESSING state
	OutboxRecoveredItems = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "recovered_items_total",
			Help:      "Total items recovered from stuck PROCESSING state",
		},
		[]string{"type"}, // event, dispatch_job
	)

	// OutboxLeaderElectionState tracks leader election status
	// 0 = follower, 1 = leader
	OutboxLeaderElectionState = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "leader_election_state",
			Help:      "Leader election state (0=follower, 1=leader)",
		},
	)

	// OutboxInFlightItems tracks total items in-flight (buffer + message group queues)
	OutboxInFlightItems = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "outbox",
			Name:      "in_flight_items",
			Help:      "Total items in-flight (buffer + processing queues)",
		},
	)

	// HTTP API metrics

	// HTTPRequestsTotal tracks HTTP API requests
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "flowcatalyst",
			Subsystem: "http",
			Name:      "requests_total",
			Help:      "Total HTTP API requests",
		},
		[]string{"method", "path", "status"},
	)

	// HTTPRequestDuration tracks HTTP API request duration
	HTTPRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "flowcatalyst",
			Subsystem: "http",
			Name:      "request_duration_seconds",
			Help:      "HTTP API request duration",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// HTTPActiveConnections tracks active HTTP connections
	HTTPActiveConnections = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "flowcatalyst",
			Subsystem: "http",
			Name:      "active_connections",
			Help:      "Number of active HTTP connections",
		},
	)
)

// CircuitBreakerState constants
const (
	CircuitBreakerClosed   = 0
	CircuitBreakerOpen     = 1
	CircuitBreakerHalfOpen = 2
)
