package dispatchjob

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// A job raised by the event fan-out is ordered within its SUBSCRIPTION's view
// of the event's message group, not within the event's group as a whole
// (owner ruling 2026-11-09). The group stored on the job — and therefore the
// group the scheduler claims, holds and publishes by, the router buffers by,
// and the reaper and settled hook sweep by — is
//
//	<subscription id>:<event message group>
//
// so one subscriber's BLOCK_ON_ERROR failure holds only that subscriber's
// later jobs; another subscriber of the same event group keeps flowing. The
// subscription id, not its code, is the prefix: codes are unique only per
// (code, client_id) — an anchor subscription and a client's may share one,
// and both receive the client's events — and the id is on every job, so the
// prefix can always be recognised again (EventMessageGroup).
//
// Composed once, at fan-out, and stored: every downstream stage reads the
// column it always read, so none of them can disagree about what the group
// is. A job created any other way (SDK routes, functions, ingestion) keeps the
// group its creator gave it. Jobs raised before this change keep their bare
// event group; they finish under it (scripts/ops/109-dispatch-group-per-subscription.sql
// re-keys live and failed ones, optionally).

// MaxMessageGroupLen is msg_dispatch_jobs.message_group's width.
const MaxMessageGroupLen = 200

// digestMarker introduces the digest form used when the composed group would
// not fit the column (see SubscriptionMessageGroup).
const digestMarker = "sha256:"

// SubscriptionMessageGroup is the group a fan-out job carries: nil when the
// event has no group (an ungrouped job is unordered), otherwise
// "<subscriptionID>:<eventGroup>". When that would exceed MaxMessageGroupLen
// (an event group longer than ~180 characters) the event group is replaced by
// "sha256:" and its hex digest: still one group per distinct event group, and
// within the column. EventMessageGroup reports that form as not recoverable,
// so the envelope reads the event's own group instead.
func SubscriptionMessageGroup(subscriptionID string, eventGroup *string) *string {
	if eventGroup == nil || *eventGroup == "" {
		return nil
	}
	g := subscriptionID + ":" + *eventGroup
	if len(g) > MaxMessageGroupLen {
		sum := sha256.Sum256([]byte(*eventGroup))
		g = subscriptionID + ":" + digestMarker + hex.EncodeToString(sum[:])
	}
	return &g
}

// EventMessageGroup recovers the event's own message group from a job's
// stored group — what a subscriber is told in the delivery envelope, which
// must not change because the platform keys its ordering per subscription.
//
// For a job whose group carries its own subscription's prefix the prefix is
// removed. ok=false means the stored group is the digest form and the event's
// group has to be read from the event itself. Any other group (a job created
// directly, or raised before groups carried the subscription) is returned
// unchanged.
func EventMessageGroup(job *DispatchJob) (group *string, ok bool) {
	if job.MessageGroup == nil || job.SubscriptionID == nil || *job.SubscriptionID == "" {
		return job.MessageGroup, true
	}
	rest, found := strings.CutPrefix(*job.MessageGroup, *job.SubscriptionID+":")
	if !found {
		return job.MessageGroup, true
	}
	if isDigestForm(rest) {
		// The digest of a long event group — or, conceivably, a short event
		// group that merely looks like one; only the event can say which.
		return nil, false
	}
	return &rest, true
}

func isDigestForm(s string) bool {
	h, ok := strings.CutPrefix(s, digestMarker)
	if !ok || len(h) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}
