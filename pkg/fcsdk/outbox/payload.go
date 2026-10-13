// Package outbox builds the rows the FlowCatalyst outbox writers store in
// outbox_messages: the row columns, the INSERT statement, and the payload of
// each row type (EVENT, AUDIT_LOG, DISPATCH_JOB). It does no I/O; the
// drivers are outboxpgx (pgx, the usecasepgx Sink) and outboxsql
// (database/sql).
//
// The rows are the shared outbox contract: the platform's outbox processor
// sends each row's payload text unchanged, as one element of
// {"items":[...]}, to POST /api/events/batch, /api/audit-logs/batch or
// /api/dispatch-jobs/batch. The payloads use the batch endpoints' camelCase
// members and match the application framework's conformance vector
// (flowcatalyst-framework conformance/outbox/outbox.json; a copy is pinned in
// testdata/) under that vector's comparison rule, with one deliberate
// difference: an event's data and an audit row's operationData are written
// as the JSON value itself, never as a string holding encoded JSON (the
// other SDKs encode them; the platform unwraps those at ingest).
package outbox

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Row types: the outbox processor maps the type column to an endpoint by
// exact string.
const (
	TypeEvent       = "EVENT"
	TypeAuditLog    = "AUDIT_LOG"
	TypeDispatchJob = "DISPATCH_JOB"
)

// ClientCoded is implemented by a domain event that names the client it
// belongs to (the client's identifier slug). It overrides the writer's
// configured client code for that event's rows.
type ClientCoded interface {
	ClientCode() string
}

// AuditOperationNamer is implemented by a command whose audit row names the
// operation explicitly (for example "place-order"). Without it the operation
// is the command's Go type name.
type AuditOperationNamer interface {
	AuditOperation() string
}

// AuditDataProvider is implemented by a command that chooses what its audit
// row records as operationData. AuditData's value is serialised and redacted
// like a command; nil leaves operationData out. Without it the whole command
// is recorded (redacted).
type AuditDataProvider interface {
	AuditData() any
}

type contextEntry struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// eventPayload is POST /api/events/batch's item, members in the framework's
// order.
type eventPayload struct {
	SpecVersion     string          `json:"specVersion"`
	Type            string          `json:"type"`
	Source          string          `json:"source,omitempty"`
	Subject         string          `json:"subject,omitempty"`
	Data            json.RawMessage `json:"data"`
	CorrelationID   string          `json:"correlationId,omitempty"`
	CausationID     string          `json:"causationId,omitempty"`
	DeduplicationID string          `json:"deduplicationId,omitempty"`
	MessageGroup    string          `json:"messageGroup,omitempty"`
	ClientCode      string          `json:"clientCode,omitempty"`
	ContextData     []contextEntry  `json:"contextData,omitempty"`
}

// EventPayload serialises a domain event as POST /api/events/batch's item:
// specVersion, type, source (left out when empty: the platform then defaults
// it to the type's {application}:{subdomain}), subject, data (the event's
// ToDataJSON value itself), correlationId, causationId, deduplicationId (the
// event id), messageGroup, clientCode (the event's own when it implements
// ClientCoded, else clientCode; left out when empty), and contextData
// (principalId and executionId, each when present). The event time is not
// sent: the platform stamps it at ingest.
func EventPayload(event usecase.DomainEvent, clientCode string) ([]byte, error) {
	if strings.TrimSpace(event.EventType()) == "" {
		return nil, errors.New("outbox: event type is required")
	}
	if strings.TrimSpace(event.EventID()) == "" {
		return nil, errors.New("outbox: event id is required (it is the row id and the deduplicationId)")
	}
	raw, err := event.ToDataJSON()
	if err != nil {
		return nil, fmt.Errorf("outbox: event %s data: %w", event.EventType(), err)
	}
	data, err := normaliseData(raw)
	if err != nil {
		return nil, fmt.Errorf("outbox: event %s data: %w", event.EventType(), err)
	}
	specVersion := event.SpecVersion()
	if specVersion == "" {
		specVersion = "1.0"
	}
	p := eventPayload{
		SpecVersion:     specVersion,
		Type:            event.EventType(),
		Source:          event.Source(),
		Subject:         event.Subject(),
		Data:            data,
		CorrelationID:   event.CorrelationID(),
		CausationID:     event.CausationID(),
		DeduplicationID: event.EventID(),
		MessageGroup:    event.MessageGroup(),
		ClientCode:      eventClientCode(event, clientCode),
	}
	if v := event.PrincipalID(); v != "" {
		p.ContextData = append(p.ContextData, contextEntry{Key: "principalId", Value: v})
	}
	if v := event.ExecutionID(); v != "" {
		p.ContextData = append(p.ContextData, contextEntry{Key: "executionId", Value: v})
	}
	return marshal(p)
}

// auditPayload is POST /api/audit-logs/batch's item, members in the
// framework's order. source and correlationId are read by no platform today
// (the endpoint ignores them); they are written for parity with the other
// writers.
type auditPayload struct {
	EntityType      string          `json:"entityType"`
	EntityID        string          `json:"entityId"`
	Operation       string          `json:"operation"`
	OperationData   json.RawMessage `json:"operationData,omitempty"`
	PrincipalID     string          `json:"principalId,omitempty"`
	PerformedAt     string          `json:"performedAt"`
	Source          string          `json:"source,omitempty"`
	CorrelationID   string          `json:"correlationId,omitempty"`
	ApplicationCode string          `json:"applicationCode,omitempty"`
	ClientCode      string          `json:"clientCode,omitempty"`
}

// AuditPayload serialises the audit row of (event, command) as
// POST /api/audit-logs/batch's item: entityType and entityId from the event
// subject, operation (AuditOperationNamer, else the command's type name),
// operationData (the redacted command, or its AuditData; the value itself,
// left out when nil), principalId, performedAt (the event time, ISO-8601 UTC
// with 0, 3, 6 or 9 fraction digits), source, correlationId, applicationCode
// and clientCode (each left out when empty). The platform refuses a row
// without principalId, and requires applicationCode of an application-scoped
// credential.
func AuditPayload(event usecase.DomainEvent, command any, applicationCode, clientCode string) ([]byte, error) {
	opData, err := auditOperationData(command)
	if err != nil {
		return nil, err
	}
	p := auditPayload{
		EntityType:      usecase.ExtractAggregateType(event.Subject()),
		EntityID:        usecase.ExtractEntityID(event.Subject()),
		Operation:       auditOperation(command),
		OperationData:   opData,
		PrincipalID:     event.PrincipalID(),
		PerformedAt:     FormatInstant(event.Time()),
		Source:          event.Source(),
		CorrelationID:   event.CorrelationID(),
		ApplicationCode: applicationCode,
		ClientCode:      eventClientCode(event, clientCode),
	}
	return marshal(p)
}

func auditOperation(command any) string {
	if n, ok := command.(AuditOperationNamer); ok {
		if name := n.AuditOperation(); name != "" {
			return name
		}
	}
	if command == nil {
		return "Unknown"
	}
	t := reflect.TypeOf(command)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Name() == "" {
		return "Unknown"
	}
	return t.Name()
}

func auditOperationData(command any) (json.RawMessage, error) {
	subject := command
	if p, ok := command.(AuditDataProvider); ok {
		subject = p.AuditData()
		if subject == nil {
			return nil, nil
		}
		// The command's masked fields still apply to the data it chose.
		if m, ok := command.(usecase.AuditMasked); ok {
			raw, err := json.Marshal(subject)
			if err != nil {
				return nil, fmt.Errorf("outbox: audit data: %w", err)
			}
			red, err := usecase.RedactAuditJSON(raw, m.AuditMaskedFields())
			if err != nil {
				return nil, fmt.Errorf("outbox: audit data: %w", err)
			}
			return compact(red)
		}
	}
	if subject == nil {
		return nil, nil
	}
	red, err := usecase.RedactedAuditCommand(subject)
	if err != nil {
		return nil, fmt.Errorf("outbox: audit command: %w", err)
	}
	if string(bytes.TrimSpace(red)) == "null" {
		return nil, nil
	}
	return compact(red)
}

func eventClientCode(event usecase.DomainEvent, fallback string) string {
	if c, ok := event.(ClientCoded); ok {
		if code := strings.TrimSpace(c.ClientCode()); code != "" {
			return code
		}
	}
	return strings.TrimSpace(fallback)
}

// normaliseData returns the event data as one compact JSON value. Empty
// input and null become {}. A JSON string holding exactly one encoded object
// or array (the other SDKs' form) is replaced by that value, once, so the
// outbox never carries double-encoded data; any other string is kept.
func normaliseData(raw []byte) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return json.RawMessage(`{}`), nil
	}
	if !json.Valid(trimmed) {
		return nil, errors.New("ToDataJSON returned invalid JSON")
	}
	if trimmed[0] == '"' {
		var s string
		if err := json.Unmarshal(trimmed, &s); err != nil {
			return nil, err
		}
		inner := bytes.TrimSpace([]byte(s))
		if len(inner) > 0 && (inner[0] == '{' || inner[0] == '[') && json.Valid(inner) {
			return compact(inner)
		}
	}
	return compact(trimmed)
}

func compact(raw []byte) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return json.RawMessage(buf.Bytes()), nil
}

// marshal encodes v without HTML escaping (<, > and & stay as they are, as
// the other SDKs write them) and without the encoder's trailing newline.
func marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// FormatInstant formats t in UTC as ISO-8601 with the fraction in groups of
// three digits, only as many as needed (2026-10-08T10:15:30Z,
// 2026-10-08T10:15:30.123Z, 2026-10-08T10:15:30.000123Z, ...): Java's
// Instant.toString, which the other writers use.
func FormatInstant(t time.Time) string {
	t = t.UTC()
	base := t.Format("2006-01-02T15:04:05")
	ns := t.Nanosecond()
	switch {
	case ns == 0:
		return base + "Z"
	case ns%1_000_000 == 0:
		return fmt.Sprintf("%s.%03dZ", base, ns/1_000_000)
	case ns%1_000 == 0:
		return fmt.Sprintf("%s.%06dZ", base, ns/1_000)
	default:
		return fmt.Sprintf("%s.%09dZ", base, ns)
	}
}
