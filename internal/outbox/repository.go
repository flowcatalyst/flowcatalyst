// Package outbox implements the outbox processor: it polls the
// consumer application's outbox table, batches by message group, and
// forwards to the FlowCatalyst platform API. Mirrors fc-outbox/src/*.
//
// Multi-backend: Postgres, SQLite, MySQL, MongoDB. The Repository
// interface abstracts the storage; each backend lives in its own
// subpackage and registers a factory at init time.
package outbox

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Item is one outbox row, mapped from the SDK customer outbox_messages
// table (clients/*/migrations/.../001_create_outbox_messages.sql). The DB
// columns are: type, message_group, payload (TEXT), status, retry_count,
// error_message, created_at, updated_at — plus SDK-owned columns
// (client_id, payload_size, headers) the processor reads-around and never
// writes. AttemptCount maps to retry_count; StatusMessage to error_message.
type Item struct {
	ID            string                `json:"id"`
	ItemType      common.OutboxItemType `json:"itemType"`
	MessageGroup  *string               `json:"messageGroup,omitempty"`
	Payload       json.RawMessage       `json:"payload"`
	Status        common.OutboxStatus
	StatusMessage string    `json:"statusMessage,omitempty"`
	AttemptCount  int       `json:"attemptCount"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Markers prefix the error_message of a row in a non-retryable failure status (BAD_REQUEST,
// FORBIDDEN), so the stored row says what it means for its message group. The column stays
// readable as it is; the processor strips them where it shows the text ([StripMarkers]).
const (
	// BlockingMarker: refused by this version of the processor. Only such rows block their group,
	// and the restart rebuild of blocked groups ([Repository.BlockingFailures]) reads them.
	BlockingMarker = "[blocking] "
	// SkippedMarker: an operator skipped the row (the admin API's skip). It replaces
	// BlockingMarker; the row keeps its failure status for the record.
	SkippedMarker = "[skipped by operator] "
	// SkippedBeforeUpgradeMarker: refused by a version before the blocking markers, whose group
	// had already moved on past it. Marked once, at start-up, and never blocks.
	SkippedBeforeUpgradeMarker = "[skipped before upgrade] "
)

// StripMarkers returns msg without its leading markers: the platform's own text, for display.
func StripMarkers(msg string) string {
	for {
		trimmed := msg
		for _, m := range []string{BlockingMarker, SkippedMarker, SkippedBeforeUpgradeMarker} {
			trimmed = strings.TrimPrefix(trimmed, m)
		}
		if trimmed == msg {
			return msg
		}
		msg = trimmed
	}
}

// ClaimFilter names the PENDING rows a claim must leave alone: the processor would only put them
// straight back. Without it a held or blocked group with a backlog of a batch or more would fill
// every claim and starve the other groups.
type ClaimFilter struct {
	// Groups are message groups not dispatched now: blocked, paused or held.
	Groups []string
	// Types are item types whose ungrouped rows are held.
	Types []common.OutboxItemType
	// IDs are single rows held on their own (an ungrouped row that failed while the rest of its
	// batch went through).
	IDs []string
}

// Repository is the per-backend storage interface.
type Repository interface {
	// ClaimPending claims up to batchSize PENDING items outside skip, marks them IN_PROGRESS,
	// and returns them. Each backend implements this with a backend-appropriate
	// claim semantic (FOR UPDATE SKIP LOCKED for SQL, findAndUpdate for Mongo).
	ClaimPending(ctx context.Context, batchSize int, skip ClaimFilter) ([]Item, error)
	// MarkSuccess removes the items: the upstream model DELETEs successfully
	// dispatched rows to keep the customer table bounded.
	MarkSuccess(ctx context.Context, ids []string) error
	// MarkFailed records the failure: it bumps retry_count, stores the
	// error_message, and sets the status. When requeue is true the row is
	// returned to PENDING so a later poll re-claims it; when false it keeps
	// the failure status code so it is NOT re-claimed. The processor requeues
	// every retryable failure (it never gives up on one) and keeps the status
	// of a non-retryable one, its msg prefixed with [BlockingMarker]. There is
	// no next-retry-at backoff column
	// upstream: the processor holds the row's group in memory instead.
	MarkFailed(ctx context.Context, ids []string, status common.OutboxStatus, msg string, requeue bool) error
	// Release returns the given claimed rows to PENDING WITHOUT a failure
	// penalty (no retry_count bump, no error_message), for the next poll to
	// re-claim. Used by block-on-error: when a group's item fails, the rest of
	// that group's already-claimed items are released un-dispatched so they
	// re-run in order behind the failed item rather than being delivered ahead
	// of it. Only affects rows still IN_PROGRESS.
	Release(ctx context.Context, ids []string) error
	// Requeue resets the given rows to PENDING regardless of their current
	// status, clearing retry_count + error, for a fresh attempt. Used by the
	// operational state machine's Unblock control to retry a poison item that
	// had blocked its message group.
	Requeue(ctx context.Context, ids []string) error
	// MarkSkipped marks the given failed rows skipped by an operator: their
	// error_message gets [SkippedMarker] in place of a leading [BlockingMarker]
	// (once), and their status is left alone. The skip of a group's poison
	// item, recorded so a restart does not block the group on it again.
	MarkSkipped(ctx context.Context, ids []string) error
	// MarkLegacyRefusals prefixes [SkippedBeforeUpgradeMarker] to the
	// error_message of every row in a non-retryable failure status that
	// carries none of the markers: refused by an earlier version, whose group
	// had moved on past it. Returns the number marked.
	MarkLegacyRefusals(ctx context.Context) (int, error)
	// RequeueRetryableFailures returns rows left in a retryable failure status
	// (INTERNAL_ERROR, UNAUTHORIZED, GATEWAY_ERROR) to PENDING, keeping their
	// retry_count and error_message. Versions before 2026-11-09 left a row there
	// once it had used up its retries; this processor never does, so the sweep
	// at start-up delivers them. Returns the number of rows returned.
	RequeueRetryableFailures(ctx context.Context) (int, error)
	// BlockingFailures returns, for each message group, its oldest row in a
	// non-retryable failure status (BAD_REQUEST, FORBIDDEN) whose error_message
	// starts with [BlockingMarker]. The processor blocks those groups at
	// start-up, so a restart never moves a group past its poison row. Payload
	// is not loaded; StatusMessage is the stored text, markers included.
	BlockingFailures(ctx context.Context) ([]Item, error)
	// RecoverStuck resets rows stuck in IN_PROGRESS (claimed but never
	// resolved — e.g. the processor crashed mid-dispatch) whose updated_at is
	// older than olderThan, returning them to PENDING for re-claim. Returns
	// the number recovered.
	RecoverStuck(ctx context.Context, olderThan time.Duration) (int, error)
	// Healthy reports whether the backend can be reached.
	Healthy(ctx context.Context) bool
	// InitSchema ensures the outbox table/collection exists.
	InitSchema(ctx context.Context) error
}

// ChannelPush is the Postgres notification channel an outbox writer NOTIFYs when it writes a PUSH
// row (in the same statement as the insert, so it is delivered at commit): a processor whose
// repository can listen (see [Waker]) polls at once instead of at its next tick, so a push is not
// held back by the poll interval (docs/push-gateway-spec.md, owner ruling 3). The poll stays as the
// fallback for writers that cannot notify and for a lost connection.
const ChannelPush = "fc_outbox_push"

// Waker is implemented by a repository that can tell the processor when new rows may be pending.
type Waker interface {
	// Listen calls wake whenever a writer notified [ChannelPush], and once after every
	// (re)connection so a notification missed meanwhile is caught up. It blocks until ctx ends and
	// reconnects on its own.
	Listen(ctx context.Context, wake func())
}
