// Package platformsink implements usecasepgx.Sink for the platform itself.
//
// Where the consumer-app SDK uses outboxpgx.Sink (writes to
// outbox_messages for eventual forwarding via fc-outbox-processor),
// the platform writes domain events and audit logs *directly* to
// msg_events and aud_logs. The platform IS the platform — there's
// no outbox to hop through.
//
// This Sink lives in internal/platform/, not in pkg/fcsdk/, because it's
// platform-specific (writes platform-owned tables) and never imported
// by consumer apps.
package platformsink

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Sink writes events and audit rows to the platform's own tables.
// Satisfies usecasepgx.Sink.
type Sink struct{}

// New constructs the platform sink.
func New() *Sink { return &Sink{} }

// Compile-time check that *Sink satisfies usecasepgx.Sink.
var _ usecasepgx.Sink = (*Sink)(nil)

// WriteEvent inserts the domain event into msg_events. The shape of
// the row matches the Rust fc-platform PgUnitOfWork::persist_event.
func (*Sink) WriteEvent(ctx context.Context, tx *usecasepgx.DbTx, event usecase.DomainEvent) error {
	data, err := event.ToDataJSON()
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}
	if len(data) == 0 {
		data = []byte(`{}`)
	}

	contextData, err := json.Marshal([]map[string]string{
		{"key": "principalId", "value": event.PrincipalID()},
		{"key": "aggregateType", "value": usecase.ExtractAggregateType(event.Subject())},
	})
	if err != nil {
		return fmt.Errorf("marshal context: %w", err)
	}

	dedupID := event.EventType() + "-" + event.EventID()

	now := time.Now().UTC()
	// No ON CONFLICT here — msg_events is partitioned by created_at and
	// the dedup unique index is composite (deduplication_id, created_at),
	// which Postgres can't infer from a column list. Matches Rust source
	// (crates/fc-platform/src/usecase/unit_of_work.rs): plain INSERT,
	// dedup duplicate errors bubble up as TX failures.
	_, err = tx.Inner().Exec(ctx,
		`INSERT INTO msg_events
		     (id, spec_version, type, source, subject,
		      time, data, correlation_id, causation_id,
		      deduplication_id, message_group, client_id,
		      context_data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7::jsonb, $8, $9, $10, $11, $12, $13::jsonb, $14)`,
		event.EventID(), event.SpecVersion(), event.EventType(),
		event.Source(), event.Subject(),
		eventTime(event), data, nullIfEmpty(event.CorrelationID()), nullIfEmpty(event.CausationID()),
		dedupID, nullIfEmpty(event.MessageGroup()), nil,
		contextData, now,
	)
	if err != nil {
		slog.Error("msg_events insert failed", "event_type", event.EventType(), "err", err)
		return fmt.Errorf("insert msg_events: %w", err)
	}
	return nil
}

// nullIfEmpty returns nil for empty strings so optional VARCHAR columns
// store SQL NULL instead of empty-string sentinels (matches Rust's None
// binding pattern).
func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// WriteAudit inserts an audit log row into aud_logs. Column set
// matches the schema (migrations 006 + 009): id, entity_type, entity_id,
// operation, operation_json, principal_id, application_id, client_id,
// performed_at. Mirrors the Rust source's column ordering exactly so a
// side-by-side parity diff stays clean.
func (*Sink) WriteAudit(ctx context.Context, tx *usecasepgx.DbTx, event usecase.DomainEvent, command any) error {
	cmdJSON, err := json.Marshal(command)
	if err != nil {
		return fmt.Errorf("marshal command: %w", err)
	}

	cmdName := "Unknown"
	if command != nil {
		t := reflect.TypeOf(command)
		if t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		if t.Name() != "" {
			cmdName = t.Name()
		}
	}

	_, err = tx.Inner().Exec(ctx,
		`INSERT INTO aud_logs
		     (id, entity_type, entity_id, operation,
		      operation_json, principal_id, application_id,
		      client_id, performed_at)
		 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8, $9)`,
		newAuditID(),
		usecase.ExtractAggregateType(event.Subject()),
		usecase.ExtractEntityID(event.Subject()),
		cmdName,
		cmdJSON,
		nullIfEmpty(event.PrincipalID()),
		nil, // application_id
		nil, // client_id
		eventTime(event),
	)
	if err != nil {
		slog.Error("aud_logs insert failed", "event_type", event.EventType(), "err", err)
		return fmt.Errorf("insert aud_logs: %w", err)
	}
	return nil
}

func eventTime(e usecase.DomainEvent) time.Time {
	t := e.Time()
	if t.IsZero() {
		return time.Now().UTC()
	}
	return t.UTC()
}

// newAuditID generates a 17-char TSID for the audit row, matching the
// aud_logs.id VARCHAR(17) PRIMARY KEY constraint. AuditLog's TSID prefix
// is "aud" → "aud_<13>".
func newAuditID() string {
	return tsid.Generate(tsid.AuditLog)
}
