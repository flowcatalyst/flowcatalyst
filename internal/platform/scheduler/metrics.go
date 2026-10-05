package scheduler

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsRegistry holds the dispatch scheduler's Prometheus series, in a
// registry of its own like every other subsystem's (router, function runner).
// The server mounts it on the metrics listener's /metrics when the scheduler is
// enabled.
//
//	fc_scheduler_jobs_claimed_total                    jobs the claim query returned
//	fc_scheduler_jobs_published_total                  jobs the broker accepted (a lane then marks them QUEUED)
//	fc_scheduler_jobs_unpublished_total                jobs the publisher failed to publish (left PENDING)
//	fc_scheduler_jobs_skipped_held_total               jobs held back behind a failed sibling (BLOCK_ON_ERROR)
//	fc_scheduler_full_batch_claims_total               claims that filled the batch (a backlog is draining)
//	fc_scheduler_poll_errors_total                     polls that failed
//	fc_scheduler_poll_duration_seconds                 one poll: permits held to jobs handed to the lanes
//	fc_scheduler_claim_duration_seconds                the claim query alone
//	fc_scheduler_lane_publish_duration_seconds{lane}   one lane batch's publish
//	fc_scheduler_buffer_in_use                         permits held (jobs between claim and a lane finishing them)
//	fc_scheduler_inflight_jobs                         size of the in-flight id set (claims this process holds)
//	fc_scheduler_jobs_dropped_poisoned_total           jobs a lane dropped to keep their group in order (left PENDING)
//	fc_scheduler_jobs_withheld_doomed_total            claimed jobs not submitted: behind a doomed in-flight job of their group (left PENDING)
//	fc_scheduler_mark_queued_not_updated_total         published jobs the QUEUED update skipped: already past PENDING
//	fc_scheduler_last_successful_poll_timestamp_seconds
//	fc_scheduler_paused_subscriptions                  subscriptions excluded from the claim as paused
//	fc_scheduler_claim_release_failures_total          claims that could not be given back to the queue (retried / swept)
//	fc_scheduler_stale_claims_released_total{reason}   claims of dead processes released (leader_start, periodic)
//	fc_scheduler_stale_jobs_recovered_total{status}    stale QUEUED / PROCESSING jobs returned to PENDING
//	fc_scheduler_queue_backlog_rows                    unclaimed, due rows in msg_dispatch_queue (leader, every 15s)
//	fc_scheduler_queue_oldest_enqueued_age_seconds     age of the oldest of them (0 when none)
//
// There is no "skipped paused" counter: paused subscriptions are excluded by
// the claim query itself, so a paused row is never seen, only the size of the
// excluded set (the gauge above).
var MetricsRegistry = prometheus.NewRegistry()

var schedMetrics = newSchedulerMetrics(MetricsRegistry)

type schedulerMetrics struct {
	claimed, published, unpublished, skippedHeld, fullBatches, pollErrors prometheus.Counter
	pollDuration, claimDuration                                           prometheus.Histogram
	lanePublish                                                           *prometheus.HistogramVec
	droppedPoisoned, markNotUpdated, withheldDoomed                       prometheus.Counter
	lastSuccess, pausedSubscriptions, bufferInUse, inflight               prometheus.Gauge
	claimReleaseFailures                                                  prometheus.Counter
	staleClaimsReleased, staleJobsRecovered                               *prometheus.CounterVec
	queueBacklogRows, queueOldestAge                                      prometheus.Gauge
}

func newSchedulerMetrics(reg prometheus.Registerer) *schedulerMetrics {
	counter := func(name, help string) prometheus.Counter {
		return prometheus.NewCounter(prometheus.CounterOpts{Name: name, Help: help})
	}
	m := &schedulerMetrics{
		claimed:     counter("fc_scheduler_jobs_claimed_total", "Dispatch jobs returned by the claim query."),
		published:   counter("fc_scheduler_jobs_published_total", "Dispatch jobs the broker accepted (a lane then marks them QUEUED)."),
		unpublished: counter("fc_scheduler_jobs_unpublished_total", "Dispatch jobs the publisher failed to publish; left PENDING to be claimed again."),
		skippedHeld: counter("fc_scheduler_jobs_skipped_held_total", "Dispatch jobs held back behind an earlier failed or backed-off job of their BLOCK_ON_ERROR group."),
		fullBatches: counter("fc_scheduler_full_batch_claims_total", "Claims that filled the whole batch (a backlog deeper than one batch)."),
		pollErrors:  counter("fc_scheduler_poll_errors_total", "Polls that ended in an error."),
		pollDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "fc_scheduler_poll_duration_seconds",
			Help:    "Duration of one scheduler poll, permits held to the jobs handed to the lanes.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
		}),
		claimDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "fc_scheduler_claim_duration_seconds",
			Help:    "Duration of the claim query.",
			Buckets: []float64{.001, .0025, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30},
		}),
		lanePublish: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "fc_scheduler_lane_publish_duration_seconds",
			Help:    "Duration of one lane batch's publish.",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"lane"}),
		droppedPoisoned: counter("fc_scheduler_jobs_dropped_poisoned_total", "Dispatch jobs a lane dropped unpublished to keep their message group in order; left PENDING."),
		markNotUpdated:  counter("fc_scheduler_mark_queued_not_updated_total", "Published dispatch jobs the QUEUED update skipped because the job had already moved past PENDING."),
		withheldDoomed:  counter("fc_scheduler_jobs_withheld_doomed_total", "Claimed dispatch jobs not submitted because the claim excluded a doomed in-flight job of their group; left PENDING."),
		bufferInUse: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_buffer_in_use",
			Help: "Permits held: jobs between a claim and a lane finishing them.",
		}),
		inflight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_inflight_jobs",
			Help: "Size of the in-flight id set: jobs claimed in the queue table and not yet finished by a lane.",
		}),
		lastSuccess: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_last_successful_poll_timestamp_seconds",
			Help: "Unix time of the last poll that finished without error.",
		}),
		pausedSubscriptions: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_paused_subscriptions",
			Help: "Subscriptions whose connection is PAUSED, excluded from the claim.",
		}),
		claimReleaseFailures: counter("fc_scheduler_claim_release_failures_total", "Dispatch job claims that could not be given back to the queue table (a lane retries; the poller leaves them to the stale-claim sweep)."),
		staleClaimsReleased: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fc_scheduler_stale_claims_released_total",
			Help: "Queue claims released because no live process held them: at the start of leadership (leader_start) or by the periodic sweep (periodic: claims older than 5 minutes).",
		}, []string{"reason"}),
		staleJobsRecovered: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "fc_scheduler_stale_jobs_recovered_total",
			Help: "Stale dispatch jobs returned to PENDING by the recovery loop, by the status they were stuck in.",
		}, []string{"status"}),
		queueBacklogRows: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_queue_backlog_rows",
			Help: "Unclaimed, due rows in msg_dispatch_queue (jobs waiting for the scheduler), sampled by the leader every 15s.",
		}),
		queueOldestAge: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "fc_scheduler_queue_oldest_enqueued_age_seconds",
			Help: "Age of the oldest unclaimed, due row in msg_dispatch_queue; 0 when there is none.",
		}),
	}
	for _, r := range []string{"leader_start", "periodic"} {
		m.staleClaimsReleased.WithLabelValues(r)
	}
	for _, st := range []string{"QUEUED", "PROCESSING"} {
		m.staleJobsRecovered.WithLabelValues(st)
	}
	reg.MustRegister(m.claimed, m.published, m.unpublished, m.skippedHeld, m.fullBatches,
		m.pollErrors, m.pollDuration, m.lastSuccess, m.pausedSubscriptions,
		m.claimDuration, m.lanePublish, m.droppedPoisoned, m.markNotUpdated, m.withheldDoomed, m.bufferInUse, m.inflight,
		m.claimReleaseFailures, m.staleClaimsReleased, m.staleJobsRecovered, m.queueBacklogRows, m.queueOldestAge)
	return m
}

// observePoll records one finished poll.
func (m *schedulerMetrics) observePoll(start time.Time, err error) {
	m.pollDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		m.pollErrors.Inc()
		return
	}
	m.lastSuccess.SetToCurrentTime()
}
