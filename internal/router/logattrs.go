package router

import (
	"context"
	"log/slog"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Correlation keys every per-message log line in the router carries. One
// fixed spelling, so a single grep (or one log-store query) follows a message
// from the poll that received it, through its pool and group, to the
// mediator's call and the broker ack.
const (
	logKeyMessageID = "message_id"
	logKeyGroup     = "group"
	logKeyPool      = "pool"
	logKeyQueue     = "queue"
)

// messageAttrs is the correlation set for one message in one pool: message
// id, message group ("" when ungrouped), pool code and source queue.
func messageAttrs(pool string, qm common.QueuedMessage) []any {
	return []any{
		logKeyMessageID, qm.Message.ID,
		logKeyGroup, qm.Message.GroupID(),
		logKeyPool, pool,
		logKeyQueue, qm.QueueIdentifier,
	}
}

// messageLogger is the per-message logger: the default logger with the
// correlation set attached, so every line about this message carries it
// without each call site having to remember all four keys.
func messageLogger(pool string, qm common.QueuedMessage) *slog.Logger {
	return slog.Default().With(messageAttrs(pool, qm)...)
}

type messageLoggerKey struct{}

// withMessageLogger puts the per-message logger on ctx, for code below the
// pool that sees only the bare common.Message (the mediator) and so cannot
// name the pool or queue itself.
func withMessageLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, messageLoggerKey{}, l)
}

// mediationLogger is the logger for a line about msg below the pool: the
// per-message logger on ctx when the pool put one there, else the default
// logger with just the message id (a direct Mediate call has no pool or
// queue to name).
func mediationLogger(ctx context.Context, msg *common.Message) *slog.Logger {
	if l, ok := ctx.Value(messageLoggerKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return slog.Default().With(logKeyMessageID, msg.ID)
}
