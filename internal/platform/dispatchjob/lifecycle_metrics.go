package dispatchjob

import (
	"encoding/json"

	"github.com/prometheus/client_golang/prometheus"
)

// MetricsRegistry holds the dispatch-job lifecycle's Prometheus series, in a
// registry of its own like every other subsystem's. The server mounts it on
// the metrics listener's /metrics unconditionally (every process that runs a
// writer of the table, scheduler or not, owns these counters).
//
//	fc_dispatch_job_transition_refused_total{transition}  transitions that matched no row
//	fc_dispatch_queue_reconcile_repaired_total{kind}      queue rows the reconcile sweep repaired
//
// A refusal is the normal answer when a transition's guard does its job: a late
// delivery callback finding the job already settled, a duplicate settled hook,
// a mark-QUEUED racing a reschedule, an operator cancel of a job that is not
// FAILED. It is not an error — but a sustained rate on one transition is
// worth looking at. Sweeps (stale recovery, the reaper) match nothing as their
// normal outcome and are not counted.
var MetricsRegistry = prometheus.NewRegistry()

var transitionRefused = newTransitionRefused(MetricsRegistry)

func newTransitionRefused(reg prometheus.Registerer) *prometheus.CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_dispatch_job_transition_refused_total",
		Help: "Dispatch job lifecycle transitions that matched no row (job absent, or not in a status the transition may move from).",
	}, []string{"transition"})
	// Materialise every label so a zero shows up in a scrape.
	for _, t := range allTransitionNames {
		v.WithLabelValues(t.Name)
	}
	reg.MustRegister(v)
	return v
}

// reconcileRepaired counts what the queue reconcile sweep had to repair, by kind
// (inserted / deleted / refreshed). Non-zero means a bug or an older binary is
// writing msg_dispatch_queue.
var reconcileRepaired = newReconcileRepaired(MetricsRegistry)

func newReconcileRepaired(reg prometheus.Registerer) *prometheus.CounterVec {
	v := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "fc_dispatch_queue_reconcile_repaired_total",
		Help: "msg_dispatch_queue rows the reconcile sweep repaired, by kind (inserted: PENDING job without a row; deleted: row of a missing or non-PENDING job; refreshed: row that no longer mirrored its job).",
	}, []string{"kind"})
	for _, k := range []string{"inserted", "deleted", "refreshed"} {
		v.WithLabelValues(k)
	}
	reg.MustRegister(v)
	return v
}

// metadataJSON renders the job's metadata for the jsonb column; nil becomes
// `[]` (the column's own default).
func metadataJSON(m []Metadata) json.RawMessage {
	b, _ := json.Marshal(metadataOrEmpty(m))
	return b
}
