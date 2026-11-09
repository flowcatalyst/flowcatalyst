package outbox

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// TypePush is the row type of a push to clients: the platform's outbox processor sends the
// payload, with the other PUSH rows', to the push gateway's POST /api/push/batch. The platform
// (and any other drainer) must know the type before an application writes one: an older drainer
// posts an unknown type to the bare platform URL and fails the row.
const TypePush = "PUSH"

// PushNotifyChannel is the Postgres notification channel a PUSH row's writer NOTIFYs in the same
// statement as the insert (PushInsertStatement), so a processor that listens on it delivers the
// push at once instead of at its next poll. Writers that cannot notify (MySQL, SQLite) leave the
// processor's poll interval as the latency.
const PushNotifyChannel = "fc_outbox_push"

// The limits of a push (push contract section 1).
const (
	// MaxPushTopicLength is the longest topic, in characters.
	MaxPushTopicLength = 255
	// MaxPushTypeLength is the longest type, in characters.
	MaxPushTypeLength = 100
	// DefaultPushMaxDataBytes is the default data limit: push.max-data-bytes.
	DefaultPushMaxDataBytes = 16384
	// MaxPushDataBytes is the most the data limit may be set to.
	MaxPushDataBytes = 65536
)

// The codes of a refused push: the codes of the framework's conformance/push/topics.json, which
// the platform's ingest answers with as well.
const (
	PushTopicInvalid  = "TOPIC_INVALID"
	PushTypeInvalid   = "TYPE_INVALID"
	PushTypeReserved  = "TYPE_RESERVED"
	PushDataInvalid   = "DATA_INVALID"
	PushDataNotObject = "DATA_NOT_OBJECT"
	PushDataTooLarge  = "DATA_TOO_LARGE"
	PushIDInvalid     = "PUSH_ID_INVALID"
	PushClientInvalid = "CLIENT_INVALID"
)

// PushError is a push the writer refuses; Code is one of the Push* codes above.
type PushError struct{ Code, Message string }

func (e *PushError) Error() string { return "invalid push: " + e.Code + ": " + e.Message }

func refusePush(code, format string, args ...any) error {
	return &PushError{Code: code, Message: fmt.Sprintf(format, args...)}
}

var (
	pushApplicationPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	pushSegmentPattern     = regexp.MustCompile(`^[0-9A-Za-z_.-]+$`)
	pushRowIDPattern       = regexp.MustCompile(`^[0-9A-Za-z_-]{1,26}$`)
)

// Push is a push to the clients of an application, written to the outbox in the same transaction
// as the change it announces. Build it with NewPush and the With* methods, then PushRow (the
// writers validate before writing, so a push the platform would refuse terminally is never
// stored). Pushes are signals: the clients read the change again; durable work is events and
// dispatch jobs.
type Push struct {
	// ID is the row id and the payload's pushId (the platform writes a pushId once); a 13-character
	// TSID is generated when empty. At most 26 characters [0-9A-Za-z_-].
	ID string
	// Topic is {application}:{segment}(:{segment})*, the first segment [a-z][a-z0-9-]* the
	// application (which the writer's credential must be allowed to push for), the others
	// [0-9A-Za-z_.-]+, at most 255 characters.
	Topic string
	// Type is [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*, at most 100 characters; reset, auth-expired,
	// subscribed and fw.* are reserved.
	Type string
	// Data is the JSON object the listeners receive, as compact JSON.
	Data json.RawMessage
	// ClientCode is the client the push is for, by identifier. Empty: the writer's
	// Config.ClientCode (none for PlatformClientCode, which leaves the credential to decide: an
	// anchor pushes for the platform client).
	ClientCode string
	// MessageGroup is the outbox message group. Empty: the topic, so the pushes of one topic
	// reach the gateway in order (the processor sends a group's rows one at a time, in order).
	// With Ungrouped it is ignored.
	MessageGroup string
	// Ungrouped writes the row with no message group: the processor batches such rows into
	// concurrent calls, which is faster and keeps no order. For signal-only topics.
	Ungrouped bool
	// MaxDataBytes is the data limit (default DefaultPushMaxDataBytes, at most MaxPushDataBytes);
	// set it to the application's push.max-data-bytes.
	MaxDataBytes int

	dataErr error
}

// NewPush starts a push of type to topic carrying data: a struct or map marshalled to JSON, or
// JSON text (json.RawMessage, []byte). A value that is not a JSON object is refused by Validate.
func NewPush(topic, typ string, data any) Push {
	p := Push{Topic: topic, Type: typ}
	switch v := data.(type) {
	case json.RawMessage:
		p.Data = v
	case []byte:
		p.Data = v
	default:
		b, err := marshal(v)
		if err != nil {
			p.dataErr = err
			return p
		}
		p.Data = b
	}
	return p
}

func (p Push) WithID(id string) Push           { p.ID = id; return p }
func (p Push) WithClientCode(code string) Push { p.ClientCode = code; return p }
func (p Push) WithMessageGroup(g string) Push  { p.MessageGroup = g; return p }
func (p Push) WithUngrouped() Push             { p.Ungrouped = true; return p }
func (p Push) WithMaxDataBytes(n int) Push     { p.MaxDataBytes = n; return p }
func (p Push) limit() int {
	if p.MaxDataBytes > 0 {
		return min(p.MaxDataBytes, MaxPushDataBytes)
	}
	return DefaultPushMaxDataBytes
}

// Validate applies framework section 1 (the topic, type and data rules and their codes) and the
// row's own: the id and the client code. The error is a *PushError.
func (p Push) Validate() error {
	if len(p.Topic) > MaxPushTopicLength {
		return refusePush(PushTopicInvalid, "a topic is at most %d characters", MaxPushTopicLength)
	}
	segs := strings.Split(p.Topic, ":")
	if len(segs) < 2 || !pushApplicationPattern.MatchString(segs[0]) {
		return refusePush(PushTopicInvalid, "a topic is {application}:{segment}(:{segment})*: %q", p.Topic)
	}
	for _, s := range segs[1:] {
		if !pushSegmentPattern.MatchString(s) {
			return refusePush(PushTopicInvalid, "a topic segment is [0-9A-Za-z_.-]+: %q", p.Topic)
		}
	}
	if len(p.Type) > MaxPushTypeLength {
		return refusePush(PushTypeInvalid, "a type is at most %d characters", MaxPushTypeLength)
	}
	for part := range strings.SplitSeq(p.Type, ".") {
		if !pushApplicationPattern.MatchString(part) {
			return refusePush(PushTypeInvalid, "a type is [a-z][a-z0-9-]*(.[a-z][a-z0-9-]*)*: %q", p.Type)
		}
	}
	if p.Type == "reset" || p.Type == "auth-expired" || p.Type == "subscribed" || strings.HasPrefix(p.Type, "fw.") {
		return refusePush(PushTypeReserved, "the type %q is reserved", p.Type)
	}
	if p.dataErr != nil {
		return refusePush(PushDataInvalid, "the data does not marshal to JSON: %v", p.dataErr)
	}
	if _, err := p.compactData(); err != nil {
		return err
	}
	if p.ID != "" && !pushRowIDPattern.MatchString(p.ID) {
		return refusePush(PushIDInvalid, "id must match [0-9A-Za-z_-]{1,26}")
	}
	if len(p.ClientCode) > 100 || (p.ClientCode != "" && strings.TrimSpace(p.ClientCode) == "") {
		return refusePush(PushClientInvalid, "clientCode must be 1 to 100 characters")
	}
	if len(p.MessageGroup) > 255 {
		return refusePush(PushTopicInvalid, "messageGroup is at most 255 characters")
	}
	return nil
}

// compactData returns the data as compact JSON after checking that it is an object within the
// limit.
func (p Push) compactData() (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(p.Data)
	if len(trimmed) == 0 {
		return nil, refusePush(PushDataInvalid, "the data is not valid JSON")
	}
	if !json.Valid(trimmed) {
		return nil, refusePush(PushDataInvalid, "the data is not valid JSON")
	}
	if trimmed[0] != '{' {
		return nil, refusePush(PushDataNotObject, "the data is not a JSON object")
	}
	c, err := compact(trimmed)
	if err != nil {
		return nil, refusePush(PushDataInvalid, "the data is not valid JSON")
	}
	if len(c) > p.limit() {
		return nil, refusePush(PushDataTooLarge, "the data is %d bytes, more than %d", len(c), p.limit())
	}
	return c, nil
}

// pushPayload is the PUSH row's payload and POST /api/push/batch's item, members in the
// framework's order: topic, type, data, clientCode, pushId.
type pushPayload struct {
	Topic      string          `json:"topic"`
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data"`
	ClientCode string          `json:"clientCode,omitempty"`
	PushID     string          `json:"pushId"`
}

// PushPayload validates the push, applies its defaults (a generated id) and serialises it as
// POST /api/push/batch's item. It returns the resolved push (its id is the row id and the
// pushId).
func PushPayload(p Push, cfg Config) (Push, []byte, error) {
	if err := p.Validate(); err != nil {
		return p, nil, err
	}
	data, err := p.compactData()
	if err != nil {
		return p, nil, err
	}
	if p.ID == "" {
		p.ID = newRowID()
	}
	code := strings.TrimSpace(p.ClientCode)
	if code == "" {
		if c := strings.TrimSpace(cfg.ClientCode); c != PlatformClientCode {
			code = c
		}
	}
	p.ClientCode = code
	b, err := marshal(pushPayload{Topic: p.Topic, Type: p.Type, Data: data, ClientCode: code, PushID: p.ID})
	if err != nil {
		return p, nil, err
	}
	return p, b, nil
}

// PushRow builds the PUSH row of a push: the row id is the push id, message_group the topic unless
// the push names another group or is Ungrouped. The client rule is the dispatch job's: the push's
// own clientCode, else the writer's configured client except PlatformClientCode.
func PushRow(cfg Config, p Push) (Row, error) {
	resolved, payload, err := PushPayload(p, cfg)
	if err != nil {
		return Row{}, err
	}
	group := ""
	if !resolved.Ungrouped {
		group = resolved.MessageGroup
		if group == "" {
			group = resolved.Topic
		}
	}
	return Row{
		ID: resolved.ID, Type: TypePush, MessageGroup: group, Payload: payload,
		ClientID: cfg.ClientID, CreatedAt: cfg.now(),
	}, nil
}

// PushInsertStatement returns the Postgres INSERT of a PUSH row with the notification: the shared
// InsertSQL's columns, written in one statement with pg_notify(PushNotifyChannel), so the
// processor's LISTEN wakes when the writer's transaction commits and not before. Arguments are
// Row.Args, as for InsertStatement with PlaceholderDollar. table must be a plain or
// schema-qualified identifier.
func PushInsertStatement(table string) (string, error) {
	if table == "" {
		table = DefaultTable
	}
	if !tableNamePattern.MatchString(table) {
		return "", fmt.Errorf("outbox: invalid table name %q", table)
	}
	return "WITH woken AS (SELECT pg_notify('" + PushNotifyChannel + "', ''))\n" +
		"INSERT INTO " + table + " (\n" +
		"  id, type, message_group, payload, status, created_at, updated_at,\n" +
		"  client_id, payload_size, headers\n" +
		")\n" +
		"SELECT $1::text, $2::text, $3::text, $4::text, $5::smallint, $6::timestamptz, $7::timestamptz,\n" +
		"  $8::text, $9::integer, $10::jsonb\n" +
		"FROM woken", nil
}
