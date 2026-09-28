package router

import (
	"sort"
	"sync/atomic"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Event-time counters for the Prometheus surface: the counters the metrics
// contract defines that a snapshot of pool state cannot give
// (fc_messages_submitted_total, fc_messages_rejected_total{reason}, the
// result label on fc_messages_processed_total, fc_consumer_polls_total,
// fc_consumer_errors_total{type}). Each is a plain atomic bumped where the
// event happens and read by the scrape; nothing here allocates per message.

// RejectReason is why a pool handed a message back, or settled it, without
// delivering it.
type RejectReason int

const (
	// RejectCapacity: the pool was full; the message was deferred to the
	// broker for its admission slot.
	RejectCapacity RejectReason = iota
	// RejectStopped: the pool was stopping or draining.
	RejectStopped
	// RejectReleased: handed back because the target was unavailable (or
	// the retry budget was spent), including the untried siblings of a
	// released head.
	RejectReleased
	// RejectBlocked: an untried sibling ACKed away behind a head that failed
	// terminally under BLOCK_ON_ERROR.
	RejectBlocked
	// RejectSuppressed: ACKed without delivery because its group is flushed.
	RejectSuppressed

	rejectReasonCount
)

var rejectReasonNames = [rejectReasonCount]string{
	RejectCapacity:   "capacity",
	RejectStopped:    "stopped",
	RejectReleased:   "released",
	RejectBlocked:    "blocked",
	RejectSuppressed: "suppressed",
}

// String is the reason's metric label value.
func (r RejectReason) String() string {
	if r < 0 || r >= rejectReasonCount {
		return "unknown"
	}
	return rejectReasonNames[r]
}

// mediationResultNames are the result label values, in MediationResult order.
var mediationResultNames = [...]string{
	common.MediationSuccess:         "SUCCESS",
	common.MediationErrorConfig:     "ERROR_CONFIG",
	common.MediationErrorProcess:    "ERROR_PROCESS",
	common.MediationErrorConnection: "ERROR_CONNECTION",
	common.MediationRateLimited:     "RATE_LIMITED",
	common.MediationCircuitOpen:     "CIRCUIT_OPEN",
	common.MediationDeferred:        "DEFERRED",
}

// poolEventCounters are one pool's event-time counters.
type poolEventCounters struct {
	submitted atomic.Uint64
	// processed counts, by mediation result, exactly the outcomes the pool's
	// success/failure totals count (MetricSuccess, MetricFailure), so the
	// per-result series sum to the fc_messages_processed_total{success}
	// series they replace.
	processed [len(mediationResultNames)]atomic.Uint64
	rejected  [rejectReasonCount]atomic.Uint64
}

func (c *poolEventCounters) reject(reason RejectReason, n int) {
	if n > 0 {
		c.rejected[reason].Add(uint64(n))
	}
}

func (c *poolEventCounters) processedOutcome(result common.MediationResult, metric DispositionMetric) {
	if metric != MetricSuccess && metric != MetricFailure {
		return
	}
	if int(result) >= 0 && int(result) < len(c.processed) {
		c.processed[result].Add(1)
	}
}

// PoolEventCounts is one pool's counters, as the scrape reads them.
type PoolEventCounts struct {
	Pool      string
	Submitted uint64
	// Processed is keyed by result label (SUCCESS, ERROR_CONFIG, ...); only
	// results with a non-zero count are present.
	Processed map[string]uint64
	// Rejected is keyed by reason label; only non-zero reasons are present.
	Rejected map[string]uint64
}

// ProcessedSuccess reports whether a Processed key is the success result.
func ProcessedSuccess(result string) bool {
	return result == mediationResultNames[common.MediationSuccess]
}

// EventCounts snapshots this pool's event-time counters.
func (p *Pool) EventCounts() PoolEventCounts {
	out := PoolEventCounts{
		Pool:      p.cfg.Code,
		Submitted: p.events.submitted.Load(),
		Processed: map[string]uint64{},
		Rejected:  map[string]uint64{},
	}
	for i := range p.events.processed {
		if n := p.events.processed[i].Load(); n > 0 {
			out.Processed[mediationResultNames[i]] = n
		}
	}
	for i := range p.events.rejected {
		if n := p.events.rejected[i].Load(); n > 0 {
			out.Rejected[RejectReason(i).String()] = n
		}
	}
	return out
}

// consumerEventCounters are one queue's poll counters. Keyed by queue name
// on the Manager, not held on the runningConsumer, so a consumer rebuilt by
// the watchdog or a reconfigure keeps counting instead of resetting.
type consumerEventCounters struct {
	polls      atomic.Uint64
	pollErrors atomic.Uint64
	pollPanics atomic.Uint64
}

// ConsumerEventCounts is one queue's poll counters.
type ConsumerEventCounts struct {
	Queue string
	Polls uint64
	// Errors is keyed by error type: "poll" (the broker call failed) and
	// "panic" (the broker client panicked). Zero types are omitted.
	Errors map[string]uint64
}

// RouterEventCounts is every event-time counter the router keeps.
type RouterEventCounts struct {
	Pools           []PoolEventCounts
	Consumers       []ConsumerEventCounts
	PanicsRecovered uint64
}

// consumerCountersFor returns the queue's counters, creating them once.
func (m *Manager) consumerCountersFor(queue string) *consumerEventCounters {
	if c, ok := m.consumerCounters.Load(queue); ok {
		return c.(*consumerEventCounters)
	}
	c, _ := m.consumerCounters.LoadOrStore(queue, &consumerEventCounters{})
	return c.(*consumerEventCounters)
}

// EventCounters snapshots every event-time counter: each pool the Manager
// tracks (including one still draining after removal), each queue that has
// ever been polled, and the process-wide recovered-panic count.
func (m *Manager) EventCounters() RouterEventCounts {
	var out RouterEventCounts
	for _, p := range m.AllPools() {
		out.Pools = append(out.Pools, p.EventCounts())
	}
	sort.Slice(out.Pools, func(i, j int) bool { return out.Pools[i].Pool < out.Pools[j].Pool })
	m.consumerCounters.Range(func(k, v any) bool {
		c := v.(*consumerEventCounters)
		cc := ConsumerEventCounts{Queue: k.(string), Polls: c.polls.Load(), Errors: map[string]uint64{}}
		if n := c.pollErrors.Load(); n > 0 {
			cc.Errors["poll"] = n
		}
		if n := c.pollPanics.Load(); n > 0 {
			cc.Errors["panic"] = n
		}
		out.Consumers = append(out.Consumers, cc)
		return true
	})
	sort.Slice(out.Consumers, func(i, j int) bool { return out.Consumers[i].Queue < out.Consumers[j].Queue })
	out.PanicsRecovered = PanicsRecovered()
	return out
}
