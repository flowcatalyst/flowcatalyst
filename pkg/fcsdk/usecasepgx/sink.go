package usecasepgx

import (
	"context"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// Sink is the destination for domain events and audit logs. The
// UnitOfWork doesn't know whether events go to outbox_messages (consumer
// apps via outboxpgx.Sink) or directly to msg_events / aud_logs
// (platform code via a platform-specific sink); it just calls into this
// interface inside the transaction it has open.
//
// A Sink that also implements DispatchJobSink can write dispatch jobs
// (WithDispatchJobs / usecaseop's Plan.Dispatch).
//
// Implementations:
//   - outboxpgx.Sink — consumer apps; writes to outbox_messages
//     so fc-outbox-processor forwards events to the platform API. Also a
//     DispatchJobSink.
//   - Platform's own sink (in flowcatalyst-go) — writes directly to
//     msg_events and aud_logs. Not part of this SDK.
type Sink interface {
	// WriteEvent appends the domain event to its destination, using the
	// already-open transaction. Must not commit or roll back the tx.
	WriteEvent(ctx context.Context, tx *DbTx, event usecase.DomainEvent) error

	// WriteAudit appends an audit log row for (event, command), using
	// the same open transaction. Implementations may no-op when audit
	// logging is disabled.
	WriteAudit(ctx context.Context, tx *DbTx, event usecase.DomainEvent, command any) error
}
