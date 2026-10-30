package outboxpgx

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Config configures the outbox Sink.
type Config struct {
	// TableName is the outbox table. Default: "outbox_messages".
	TableName string
	// ClientID is stored in every row's client_id column. May be empty. The
	// outbox processor does not read it.
	ClientID string
	// ClientCode is the client's identifier slug, sent as clientCode in
	// event and audit payloads so the platform attributes them to that
	// client. The platform refuses a code the credential cannot reach (403
	// for the whole batch); empty leaves it out, and the platform then scopes
	// by the credential (an anchor writes platform-scoped events, a
	// single-client credential its own client). A domain event implementing
	// outbox.ClientCoded overrides it.
	ClientCode string
	// AuditEnabled writes audit log rows alongside every event. The
	// platform always audits its control plane writes; consumer apps
	// should only enable this for admin / human-initiated operations,
	// not for every transactional event.
	AuditEnabled bool
	// ApplicationCode, when set, is written as each audit row's
	// applicationCode, attributing it to the application. The platform
	// requires it when the credential the outbox processor uses is
	// application-scoped (an application's own service account), and
	// refuses the row otherwise.
	ApplicationCode string
	// Now is the clock for created_at/updated_at. Default time.Now.
	Now func() time.Time
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		TableName:    outbox.DefaultTable,
		AuditEnabled: false,
	}
}

// Sink writes domain events, audit logs and dispatch jobs to the
// outbox_messages table. Satisfies usecasepgx.Sink. The rows and their
// payloads are built by package outbox (the shared outbox contract).
type Sink struct {
	cfg Config
}

// NewSink builds a Sink. If cfg.TableName is empty, defaults to
// "outbox_messages".
func NewSink(cfg Config) *Sink {
	if cfg.TableName == "" {
		cfg.TableName = outbox.DefaultTable
	}
	return &Sink{cfg: cfg}
}

// Compile-time check that *Sink satisfies usecasepgx.Sink and writes
// dispatch jobs.
var _ usecasepgx.DispatchJobSink = (*Sink)(nil)

func (s *Sink) rowConfig() outbox.Config {
	return outbox.Config{
		ClientID: s.cfg.ClientID, ClientCode: s.cfg.ClientCode,
		ApplicationCode: s.cfg.ApplicationCode, Now: s.cfg.Now,
	}
}

// WriteEvent inserts an EVENT row into outbox_messages. The row id is the
// event id (also the payload's deduplicationId).
func (s *Sink) WriteEvent(ctx context.Context, tx *usecasepgx.DbTx, event usecase.DomainEvent) error {
	row, err := outbox.EventRow(s.rowConfig(), event)
	if err != nil {
		return err
	}
	return s.insert(ctx, tx.Inner(), row)
}

// WriteAudit inserts an AUDIT_LOG row into outbox_messages, if audit
// logging is enabled. No-op otherwise.
func (s *Sink) WriteAudit(ctx context.Context, tx *usecasepgx.DbTx, event usecase.DomainEvent, command any) error {
	if !s.cfg.AuditEnabled {
		return nil
	}
	row, err := outbox.AuditRow(s.rowConfig(), event, command)
	if err != nil {
		return err
	}
	return s.insert(ctx, tx.Inner(), row)
}

// WriteDispatchJobs inserts the DISPATCH_JOB rows of jobs, in order. It is
// what usecaseop's Plan.Dispatch (and usecasepgx.WithDispatchJobs) write
// through, after the event and before the audit row. Satisfies
// usecasepgx.DispatchJobSink.
func (s *Sink) WriteDispatchJobs(ctx context.Context, tx *usecasepgx.DbTx, jobs []outbox.DispatchJob) error {
	for _, job := range jobs {
		if err := s.WriteDispatchJob(ctx, tx.Inner(), job); err != nil {
			return err
		}
	}
	return nil
}

// WriteDispatchJob validates job and inserts its DISPATCH_JOB row (the
// platform's outbox processor delivers it to POST /api/dispatch-jobs/batch).
// The row id is the job id (generated when the job has none).
//
// This is the lower-level path, for a transaction the application runs
// itself. Inside a use case, prefer declaring the job on the plan
// (usecaseop's Plan.Dispatch): the plan writes it in the use case's
// transaction in the contract's order (after the event, before the audit
// row), refuses an invalid job before anything is written, and gives a job
// without a correlation id the event's.
func (s *Sink) WriteDispatchJob(ctx context.Context, tx pgx.Tx, job outbox.DispatchJob) error {
	row, err := outbox.DispatchJobRow(s.rowConfig(), job)
	if err != nil {
		return err
	}
	return s.insert(ctx, tx, row)
}

func (s *Sink) insert(ctx context.Context, tx pgx.Tx, row outbox.Row) error {
	stmt, err := outbox.InsertStatement(s.cfg.TableName, outbox.PlaceholderDollar)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, stmt, row.Args()...)
	return err
}
