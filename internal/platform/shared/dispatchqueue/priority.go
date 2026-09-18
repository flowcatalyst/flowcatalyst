// Package dispatchqueue holds the naming and priority rules shared by the
// dispatch scheduler (which publishes each claimed job to a queue) and the
// router-config document the platform serves (which advertises those same
// queues). Both sides compose names here so a job's destination and the
// document can never disagree on the shape.
package dispatchqueue

import (
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Priority is a subscription's dispatch priority: which of the client's two
// queues — {prefix}-{client}-DEFAULT / {prefix}-{client}-HIGH_PRIORITY — a
// dispatch job raised from that subscription publishes to.
//
// The value is the stored form of msg_subscriptions.queue and the wire value
// of the `queue` field on subscription create/update. The zero value is "not
// set", which the column permits and which reads as PriorityDefault on the
// publish path (see ForPublishing).
type Priority string

const (
	// PriorityDefault is the normal lane, and the fallback for anything the
	// publish path cannot make sense of.
	PriorityDefault Priority = "DEFAULT"
	// PriorityHighPriority is the expedited lane.
	PriorityHighPriority Priority = "HIGH_PRIORITY"
)

// Parse is the WRITE path: it validates a queue value supplied on subscription
// create/update.
//
// Matching is case-insensitive, deliberately. The SPA's subscription create
// form already sends `queue: "default"` and requires the field, so matching
// case-sensitively would reject every create from the UI — and the SPA is
// built from this repo's frontend but embedded by both servers, so the fix
// could not be confined to one side. "default", "Default" and "DEFAULT" all
// parse to PriorityDefault; only genuinely unknown text is rejected.
//
// An empty or blank value means "no priority set" and returns the zero
// Priority with no error: the column stays nullable and create/update may omit
// the field entirely.
func Parse(raw string) (Priority, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	switch Priority(strings.ToUpper(trimmed)) {
	case PriorityDefault:
		return PriorityDefault, nil
	case PriorityHighPriority:
		return PriorityHighPriority, nil
	default:
		return "", usecase.Validation("INVALID_QUEUE", "queue must be DEFAULT or HIGH_PRIORITY")
	}
}

// ForJob is the READ counterpart of Parse for msg_dispatch_jobs.queue — the
// job's OWN priority claim, distinct from ForPublishing's subscription
// fallback. It reports whether stored named a recognised priority at all
// (ok=false for nil, blank, or unrecognised legacy text), because
// DestinationResolver's resolution order (docs/spec/dispatch-job-priority.md
// R4) needs to tell "the job asked for a priority" from "the job said
// nothing, defer to the subscription": a job that explicitly stored DEFAULT
// must still win over a HIGH_PRIORITY subscription (job wins, R4/T6) rather
// than reading as unset, but a legacy job with no usable value of its own
// must fall through to the subscription lookup (R4/T7) exactly as it did
// before this column existed.
//
// Never fails, for the same reason ForPublishing never fails: an error here
// would strand a job that happens to carry legacy text in its own column
// rather than just falling through to the subscription.
func ForJob(stored *string) (Priority, bool) {
	if stored == nil {
		return "", false
	}
	switch Priority(strings.ToUpper(strings.TrimSpace(*stored))) {
	case PriorityDefault:
		return PriorityDefault, true
	case PriorityHighPriority:
		return PriorityHighPriority, true
	default:
		return "", false
	}
}

// ForPublishing is the READ counterpart of Parse, for the publish path: which
// queue a stored msg_subscriptions.queue value routes to.
//
// Deliberately lenient where Parse is strict, and the asymmetry is the point.
// Write validation cannot reach rows that already exist: every row created
// before the `queue` field became writable holds NULL, and legacy rows hold
// arbitrary text — staging carries queue segments such as "workers-high".
// Absent, blank and unrecognised all yield PriorityDefault, and this never
// fails, because the scheduler calls it while claiming a batch: an error there
// would strand every job on the offending subscription rather than routing it
// to the normal lane.
//
// The accepted cost: a legacy "workers-high" row is treated as normal priority
// with nothing announcing the downgrade. That was chosen over warning on it or
// refusing to publish; the remedy, if it ever bites, is a migration rather
// than a change here.
func ForPublishing(stored *string) Priority {
	if stored == nil {
		return PriorityDefault
	}
	if Priority(strings.ToUpper(strings.TrimSpace(*stored))) == PriorityHighPriority {
		return PriorityHighPriority
	}
	return PriorityDefault
}
