// Package common holds types shared across the router, queue, stream,
// outbox, and platform packages: messages, dispatch modes, mediation
// results, outbox status, configuration shapes, and warnings.
//
// JSON tags match the wire contract exactly (camelCase, omitempty for
// optionals, SCREAMING_SNAKE_CASE for enums) so wire format is
// byte-compatible.
package common

import (
	"log/slog"
	"time"
)

// MediationType is the kind of mediation (currently only HTTP).
type MediationType string

const (
	MediationTypeHTTP MediationType = "HTTP"
)

// DispatchMode controls ordering behavior within a message group.
// Shared across platform, scheduler, and router.
type DispatchMode string

const (
	DispatchImmediate    DispatchMode = "IMMEDIATE"
	DispatchNextOnError  DispatchMode = "NEXT_ON_ERROR"
	DispatchBlockOnError DispatchMode = "BLOCK_ON_ERROR"
)

// DefaultDispatchMode is what an unspecified mode means, everywhere: keep a
// message group in sequence, one at a time, and move on to the next message if
// one fails.
//
// It was IMMEDIATE, which is the only mode with no ordering at all — so any
// producer that omitted the field silently gave up sequencing, and the loss
// showed only under load, where concurrent dispatch actually gets to interleave.
// A default that quietly weakens a guarantee is the wrong way round: ordering is
// cheap to opt out of (set IMMEDIATE) and expensive to discover you never had.
const DefaultDispatchMode = DispatchNextOnError

// ParseDispatchMode maps a wire/stored value to a mode. Empty means
// unspecified, which is DefaultDispatchMode. Anything unrecognised is a
// producer bug: it also takes the default, but says so, because silently
// treating a typo as "no ordering" is how the previous default hid.
//
// Deliberately still lenient — this is the one X-06 exemption, ruled X-01:
// unknown/absent dispatch mode defaults to NEXT_ON_ERROR rather than
// rejecting. Do not convert this to the (T, bool) strict shape used by
// ParseDispatchStatus and the rest of this package's enum parsers.
func ParseDispatchMode(s string) DispatchMode {
	switch s {
	case "IMMEDIATE":
		return DispatchImmediate
	case "NEXT_ON_ERROR":
		return DispatchNextOnError
	case "BLOCK_ON_ERROR":
		return DispatchBlockOnError
	case "":
		return DefaultDispatchMode
	default:
		slog.Warn("unrecognised dispatch mode; using the default",
			"value", s, "default", string(DefaultDispatchMode))
		return DefaultDispatchMode
	}
}

// RequiresOrdering reports whether the mode demands FIFO processing.
func (d DispatchMode) RequiresOrdering() bool {
	return d == DispatchNextOnError || d == DispatchBlockOnError
}

// Message is the core message structure that flows through the system.
// Compatible with the historical MessagePointer wire shape.
type Message struct {
	ID              string        `json:"id"`
	PoolCode        string        `json:"poolCode,omitempty"`
	AuthToken       *string       `json:"authToken,omitempty"`
	SigningSecret   *string       `json:"signingSecret,omitempty"`
	MediationType   MediationType `json:"mediationType"`
	MediationTarget string        `json:"mediationTarget"`
	MessageGroupID  *string       `json:"messageGroupId,omitempty"`
	HighPriority    bool          `json:"highPriority,omitempty"`
	DispatchMode    DispatchMode  `json:"dispatchMode,omitempty"`
}

// QueuedMessage is a Message received from a queue with broker tracking.
type QueuedMessage struct {
	Message         Message
	ReceiptHandle   string
	BrokerMessageID string // empty if not provided
	QueueIdentifier string
	// BatchID is a router-assigned grouping over messages received in the
	// same poll batch. It is set by the pool's
	// poll loop, not the broker, and is informational only.
	BatchID string
	// Attempts counts how many in-pipeline mediation attempts this delivery
	// has already had (0 on first dispatch). The pool increments it on each
	// retry so it can recognise a re-dispatch (skip re-tracking) and grow the
	// backoff. Internal-only; never crosses the wire.
	Attempts uint
}

// InFlightMessage tracks a message currently being processed.
type InFlightMessage struct {
	MessageID       string
	BrokerMessageID string
	PoolCode        string
	QueueIdentifier string
	StartedAt       time.Time
	// LastSeenAt is refreshed every time the broker redelivers this message
	// (receipt-handle swap). The reaper ages entries on LastSeenAt, not
	// StartedAt: while the broker still holds the message it keeps
	// redelivering (refreshing this), so a long-buffered entry is never
	// reaped out from under the dedup; once the broker no longer has the
	// message, refreshes stop and the entry ages out.
	LastSeenAt     time.Time
	MessageGroupID string
	BatchID        string
	ReceiptHandle  string
	// Attempts is >0 once the message has failed at least once and is being
	// retried in-pipeline. The stall detector never force-NACKs such an entry
	// (a live retry owns it, and yanking it away would double-deliver), and
	// the reaper exempts it from the idle bound — but only for as long as
	// LastRetryAt says that retry is still live.
	Attempts uint
	// LastRetryAt is when this entry last recorded a new in-pipeline attempt
	// (InFlightTracker.MarkRetrying, the single mutator of Attempts). It is
	// what makes the reaper's Attempts>0 exemption FINITE.
	//
	// The exemption used to be unconditional, on the reasoning that the retry
	// budget (maxInPipelineAttempts) always ends in a release that removes the
	// entry. A drainer cancelled mid-retry never reaches that release — it
	// re-fronts the message, clears the group's working flag and returns — so
	// the entry outlived its owner and, being permanently exempt, could never
	// be reaped. Every subsequent redelivery of the message was then dropped
	// as a duplicate of a copy nobody owned, for ever.
	//
	// Zero while Attempts is 0.
	LastRetryAt time.Time
}

// GroupID returns the message group, or "" when the message is ungrouped.
func (m *Message) GroupID() string {
	if m == nil || m.MessageGroupID == nil {
		return ""
	}
	return *m.MessageGroupID
}

// NewInFlightMessage constructs a tracker.
func NewInFlightMessage(m *Message, brokerID, queueID, batchID, receipt string) *InFlightMessage {
	groupID := m.GroupID()
	now := time.Now()
	return &InFlightMessage{
		MessageID:       m.ID,
		BrokerMessageID: brokerID,
		PoolCode:        m.PoolCode,
		QueueIdentifier: queueID,
		StartedAt:       now,
		LastSeenAt:      now,
		MessageGroupID:  groupID,
		BatchID:         batchID,
		ReceiptHandle:   receipt,
	}
}

// ElapsedSeconds returns how long the message has been in flight.
func (m *InFlightMessage) ElapsedSeconds() int64 {
	return int64(time.Since(m.StartedAt).Seconds())
}

// UpdateReceiptHandle replaces the receipt handle on broker redelivery and
// refreshes LastSeenAt (the broker evidently still holds the message).
func (m *InFlightMessage) UpdateReceiptHandle(h string) {
	m.ReceiptHandle = h
	m.LastSeenAt = time.Now()
}
