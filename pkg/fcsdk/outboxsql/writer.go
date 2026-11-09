package outboxsql

import (
	"context"
	"database/sql"
	"time"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Execer is what a Writer writes through: *sql.Tx (the usual case: the
// caller's transaction, so the rows commit with the work) or *sql.DB.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// WriterConfig configures a Writer. The fields mean what they mean on
// outboxpgx.Config.
type WriterConfig struct {
	// TableName is the outbox table. Default: "outbox_messages".
	TableName string
	// ClientID is stored in every row's client_id column. May be empty.
	ClientID string
	// ClientCode is the client's identifier slug, sent as clientCode in
	// event and audit payloads. Empty leaves it out.
	ClientCode string
	// ApplicationCode is written as each audit row's applicationCode.
	ApplicationCode string
	// Now is the clock for created_at/updated_at. Default time.Now.
	Now func() time.Time
}

// Writer writes outbox rows through database/sql. Safe for concurrent use.
type Writer struct {
	stmt string
	// pushStmt inserts a PUSH row; on Postgres it also notifies outbox.PushNotifyChannel.
	pushStmt string
	cfg      outbox.Config
}

// NewWriter builds a Writer for the dialect's placeholder style. It fails
// only on an invalid table name.
func NewWriter(dialect Dialect, cfg WriterConfig) (*Writer, error) {
	style := outbox.PlaceholderQuestion
	if dialect == DialectPostgres {
		style = outbox.PlaceholderDollar
	}
	stmt, err := outbox.InsertStatement(cfg.TableName, style)
	if err != nil {
		return nil, err
	}
	pushStmt := stmt
	if dialect == DialectPostgres {
		if pushStmt, err = outbox.PushInsertStatement(cfg.TableName); err != nil {
			return nil, err
		}
	}
	return &Writer{stmt: stmt, pushStmt: pushStmt, cfg: outbox.Config{
		ClientID: cfg.ClientID, ClientCode: cfg.ClientCode,
		ApplicationCode: cfg.ApplicationCode, Now: cfg.Now,
	}}, nil
}

// WriteEvent inserts the EVENT row of a domain event.
func (w *Writer) WriteEvent(ctx context.Context, ex Execer, event usecase.DomainEvent) error {
	row, err := outbox.EventRow(w.cfg, event)
	if err != nil {
		return err
	}
	return w.insert(ctx, ex, row)
}

// WriteAudit inserts the AUDIT_LOG row of (event, command).
func (w *Writer) WriteAudit(ctx context.Context, ex Execer, event usecase.DomainEvent, command any) error {
	row, err := outbox.AuditRow(w.cfg, event, command)
	if err != nil {
		return err
	}
	return w.insert(ctx, ex, row)
}

// WriteDispatchJob validates job and inserts its DISPATCH_JOB row.
func (w *Writer) WriteDispatchJob(ctx context.Context, ex Execer, job outbox.DispatchJob) error {
	row, err := outbox.DispatchJobRow(w.cfg, job)
	if err != nil {
		return err
	}
	return w.insert(ctx, ex, row)
}

// WritePush validates push and inserts its PUSH row. On Postgres the statement also notifies
// outbox.PushNotifyChannel, so the processor delivers the push as soon as the transaction commits;
// on other dialects the processor's poll interval is the latency.
func (w *Writer) WritePush(ctx context.Context, ex Execer, push outbox.Push) error {
	row, err := outbox.PushRow(w.cfg, push)
	if err != nil {
		return err
	}
	_, err = ex.ExecContext(ctx, w.pushStmt, row.Args()...)
	return err
}

func (w *Writer) insert(ctx context.Context, ex Execer, row outbox.Row) error {
	_, err := ex.ExecContext(ctx, w.stmt, row.Args()...)
	return err
}
