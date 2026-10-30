package outbox

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/flowcatalyst/go-sdk/usecase"
)

// DefaultTable is the outbox table every SDK writes.
const DefaultTable = "outbox_messages"

// InsertSQL is the outbox INSERT, byte for byte the framework's
// (conformance/sql/outbox/insert.sql), with ? placeholders. The arguments
// are Row.Args. retry_count and error_message are left to their column
// defaults.
const InsertSQL = "INSERT INTO outbox_messages (\n" +
	"  id, type, message_group, payload, status, created_at, updated_at,\n" +
	"  client_id, payload_size, headers\n" +
	")\n" +
	"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"

// Placeholder is a driver's bind-parameter style.
type Placeholder int

const (
	// PlaceholderQuestion is ? (MySQL, SQLite).
	PlaceholderQuestion Placeholder = iota
	// PlaceholderDollar is $1, $2, ... (Postgres: pgx, lib/pq).
	PlaceholderDollar
)

var tableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)?$`)

// InsertStatement returns InsertSQL for table (DefaultTable when empty) in
// the given placeholder style. table must be a plain or schema-qualified
// identifier.
func InsertStatement(table string, style Placeholder) (string, error) {
	if table == "" {
		table = DefaultTable
	}
	if !tableNamePattern.MatchString(table) {
		return "", fmt.Errorf("outbox: invalid table name %q", table)
	}
	stmt := strings.Replace(InsertSQL, "INSERT INTO "+DefaultTable+" (", "INSERT INTO "+table+" (", 1)
	if style == PlaceholderDollar {
		var b strings.Builder
		n := 0
		for _, r := range stmt {
			if r == '?' {
				n++
				b.WriteString("$" + strconv.Itoa(n))
				continue
			}
			b.WriteRune(r)
		}
		stmt = b.String()
	}
	return stmt, nil
}

// Row is one outbox_messages row as the writers store it: status 0 (pending),
// created_at = updated_at = the write time in UTC, headers NULL. An empty
// MessageGroup or ClientID is stored as NULL.
type Row struct {
	ID           string
	Type         string
	MessageGroup string
	Payload      []byte
	ClientID     string
	CreatedAt    time.Time
}

// Args returns the INSERT arguments in InsertSQL's column order.
func (r Row) Args() []any {
	t := r.CreatedAt.UTC()
	return []any{
		r.ID, r.Type, nullable(r.MessageGroup), string(r.Payload), 0, t, t,
		nullable(r.ClientID), len(r.Payload), nil,
	}
}

func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// Config is what the writers need beyond the row's own data.
type Config struct {
	// ClientID is stored in every row's client_id column (the outbox
	// processor does not read it; it is for the application's own
	// partitioning). May be empty.
	ClientID string
	// ClientCode is the client's identifier slug, written as clientCode in
	// event and audit payloads so the platform attributes them to that
	// client (a domain event implementing ClientCoded overrides it). Empty
	// leaves it out: the platform then scopes by the credential (an anchor
	// writes platform-scoped rows, a single-client credential its client).
	ClientCode string
	// ApplicationCode is written as each audit row's applicationCode. The
	// platform requires it of an application-scoped credential.
	ApplicationCode string
	// Now is the clock for created_at/updated_at. Default time.Now.
	Now func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now().UTC()
	}
	return time.Now().UTC()
}

// EventRow builds the EVENT row of a domain event: the row id is the event id
// (also the payload's deduplicationId), message_group the event's message
// group.
func EventRow(cfg Config, event usecase.DomainEvent) (Row, error) {
	payload, err := EventPayload(event, cfg.ClientCode)
	if err != nil {
		return Row{}, err
	}
	return Row{
		ID: event.EventID(), Type: TypeEvent, MessageGroup: event.MessageGroup(),
		Payload: payload, ClientID: cfg.ClientID, CreatedAt: cfg.now(),
	}, nil
}

// AuditRow builds the AUDIT_LOG row of (event, command): a generated id
// (13-character TSID), no message group.
func AuditRow(cfg Config, event usecase.DomainEvent, command any) (Row, error) {
	payload, err := AuditPayload(event, command, cfg.ApplicationCode, cfg.ClientCode)
	if err != nil {
		return Row{}, err
	}
	return Row{
		ID: newRowID(), Type: TypeAuditLog, Payload: payload,
		ClientID: cfg.ClientID, CreatedAt: cfg.now(),
	}, nil
}

// DispatchJobRow builds the DISPATCH_JOB row of a job: the row id is the
// job id, message_group the job's message group.
func DispatchJobRow(cfg Config, job DispatchJob) (Row, error) {
	j, payload, err := DispatchJobPayload(job)
	if err != nil {
		return Row{}, err
	}
	return Row{
		ID: j.ID, Type: TypeDispatchJob, MessageGroup: j.MessageGroup,
		Payload: payload, ClientID: cfg.ClientID, CreatedAt: cfg.now(),
	}, nil
}
