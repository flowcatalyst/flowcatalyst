// Package eventtype is the worked-example subdomain.
// Every other subdomain follows this exact shape (entity.go, repository.go,
// operations/, api.go).
package eventtype

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the event-type lifecycle state (CURRENT | ARCHIVED).
type Status string

const (
	StatusCurrent  Status = "CURRENT"
	StatusArchived Status = "ARCHIVED"
)

// ParseStatus parses a stored status value. Returns ok=false for anything
// other than CURRENT or ARCHIVED — callers MUST reject on ok=false rather
// than coerce an unrecognised value to CURRENT (X-06: a loud read error,
// never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseStatus(s string) (Status, bool) {
	switch Status(s) {
	case StatusCurrent, StatusArchived:
		return Status(s), true
	default:
		return "", false
	}
}

// Source identifies where the event type was authored.
type Source string

const (
	SourceCode Source = "CODE"
	SourceAPI  Source = "API"
	SourceUI   Source = "UI"
)

// ParseSource parses a stored source value. Returns ok=false for anything
// other than CODE, API, or UI — callers MUST reject on ok=false rather than
// coerce an unrecognised value to UI (X-06: a loud read error, never a
// silent default). Follows the (T, bool) shape of common.ParseOutboxItemType.
func ParseSource(s string) (Source, bool) {
	switch Source(s) {
	case SourceCode, SourceAPI, SourceUI:
		return Source(s), true
	default:
		return "", false
	}
}

// SpecVersionStatus is the schema-version state.
type SpecVersionStatus string

const (
	SpecFinalising SpecVersionStatus = "FINALISING"
	SpecCurrent    SpecVersionStatus = "CURRENT"
	SpecDeprecated SpecVersionStatus = "DEPRECATED"
)

// ParseSpecVersionStatus parses a stored spec-version status value. Returns
// ok=false for anything other than FINALISING, CURRENT, or DEPRECATED —
// callers MUST reject on ok=false rather than coerce an unrecognised value
// to FINALISING (X-06: a loud read error, never a silent default). Follows
// the (T, bool) shape of common.ParseOutboxItemType.
func ParseSpecVersionStatus(s string) (SpecVersionStatus, bool) {
	switch SpecVersionStatus(s) {
	case SpecFinalising, SpecCurrent, SpecDeprecated:
		return SpecVersionStatus(s), true
	default:
		return "", false
	}
}

// SchemaType identifies the payload language of a schema.
type SchemaType string

const (
	SchemaJSON  SchemaType = "JSON_SCHEMA"
	SchemaXSD   SchemaType = "XSD"
	SchemaProto SchemaType = "PROTO"
)

// ParseSchemaType parses a stored/wire schema-type value, accepting the
// legacy aliases XML_SCHEMA→XSD and PROTOBUF→PROTO. Returns ok=false for
// anything else — callers MUST reject on ok=false rather than coerce an
// unrecognised value to JSON_SCHEMA (X-06: a loud read error, never a
// silent default). Follows the (T, bool) shape of common.ParseOutboxItemType.
func ParseSchemaType(s string) (SchemaType, bool) {
	switch s {
	case string(SchemaJSON):
		return SchemaJSON, true
	case "XSD", "XML_SCHEMA":
		return SchemaXSD, true
	case "PROTO", "PROTOBUF":
		return SchemaProto, true
	default:
		return "", false
	}
}

// DefaultMimeType is the MIME type a schema version of type t gets when none
// is given.
func DefaultMimeType(t SchemaType) string {
	switch t {
	case SchemaXSD:
		return "application/xml"
	case SchemaProto:
		return "application/x-protobuf"
	case SchemaJSON:
		return "application/schema+json"
	default:
		return "application/schema+json"
	}
}

// NormaliseSchemaContent returns the stored form of a schema of type t. A JSON
// Schema is the JSON document itself: a JSON string holding a document (an
// object or an array) is unwrapped, so a schema typed into a text field is
// not stored double-encoded; any other string is refused. XSD and PROTO
// schemas are text and must arrive as a JSON string.
func NormaliseSchemaContent(t SchemaType, raw json.RawMessage) (json.RawMessage, error) {
	trimmed := bytes.TrimSpace(raw)
	if !json.Valid(trimmed) {
		return nil, errors.New("schema is not valid JSON")
	}
	isString := len(trimmed) > 0 && trimmed[0] == '"'
	if t != SchemaJSON {
		if !isString {
			return nil, errors.New("an XSD or PROTO schema is text: send it as a JSON string")
		}
		return trimmed, nil
	}
	if !isString {
		return trimmed, nil
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err != nil {
		return nil, errors.New("schema is not valid JSON")
	}
	inner := bytes.TrimSpace([]byte(text))
	if len(inner) == 0 || (inner[0] != '{' && inner[0] != '[') || !json.Valid(inner) {
		return nil, errors.New("a JSON Schema must be a JSON document")
	}
	return json.RawMessage(inner), nil
}

// SpecVersion is a schema version row.
type SpecVersion struct {
	ID            string            `json:"id"`
	EventTypeID   string            `json:"eventTypeId"`
	Version       string            `json:"version"`
	MimeType      string            `json:"mimeType"`
	SchemaContent json.RawMessage   `json:"schemaContent,omitempty"`
	SchemaType    SchemaType        `json:"schemaType"`
	Status        SpecVersionStatus `json:"status"`
	CreatedAt     time.Time         `json:"createdAt"`
	UpdatedAt     time.Time         `json:"updatedAt"`
}

// NewSpecVersion mints a new schema version.
func NewSpecVersion(eventTypeID, version string, schemaContent json.RawMessage) SpecVersion {
	now := time.Now().UTC()
	return SpecVersion{
		ID:            tsid.Generate(tsid.Schema),
		EventTypeID:   eventTypeID,
		Version:       version,
		MimeType:      "application/schema+json",
		SchemaContent: schemaContent,
		SchemaType:    SchemaJSON,
		Status:        SpecFinalising,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

// IsCurrent / IsDeprecated report the spec version's status.
func (s *SpecVersion) IsCurrent() bool    { return s.Status == SpecCurrent }
func (s *SpecVersion) IsDeprecated() bool { return s.Status == SpecDeprecated }

// majorVersion returns the leading numeric segment of a semver-style
// version string (e.g. "1.0" → "1", "2.3.4-alpha" → "2"). Returns
// "" when no major segment can be parsed; callers treat that as "no
// auto-deprecate sibling lookup possible".
func majorVersion(v string) string {
	for i, c := range v {
		if c == '.' || c == '-' || c == '+' {
			return v[:i]
		}
	}
	return v
}

// Major returns the leading numeric major segment of the version
// string. Used by the finalise logic to find sibling CURRENT versions
// that should be auto-deprecated. Exposed so use cases can match
// without re-implementing the parse.
func (s *SpecVersion) Major() string { return majorVersion(s.Version) }

// EventType is the aggregate root.
type EventType struct {
	ID           string        `json:"id"`
	Code         string        `json:"code"`
	Name         string        `json:"name"`
	Description  *string       `json:"description,omitempty"`
	SpecVersions []SpecVersion `json:"specVersions"`
	Status       Status        `json:"status"`
	Source       Source        `json:"source"`
	ClientScoped bool          `json:"clientScoped"`
	Application  string        `json:"application"`
	Subdomain    string        `json:"subdomain"`
	Aggregate    string        `json:"aggregate"`
	EventName    string        `json:"eventName"`
	ClientID     *ids.ClientID `json:"clientId,omitempty"`
	CreatedBy    *string       `json:"createdBy,omitempty"`
	CreatedAt    time.Time     `json:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt"`
}

// IDStr returns the aggregate ID. Method exists because usecase.HasID
// is an interface constraint, and the `ID` field can't satisfy it.
// Value receiver so both EventType and *EventType satisfy HasID — the
// generic call site in operations/*.go passes EventType as the type
// parameter and *EventType as the value.
func (e EventType) IDStr() string { return e.ID }

// New constructs an EventType from a colon-separated code. Returns an
// error if the code is invalid (must be application:subdomain:aggregate:event).
func New(code, name string) (*EventType, error) {
	parts := strings.Split(code, ":")
	if len(parts) != 4 {
		return nil, errors.New("event type code must follow format: application:subdomain:aggregate:event")
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return nil, errors.New("event type code segments cannot be empty")
		}
	}
	now := time.Now().UTC()
	return &EventType{
		ID:           tsid.Generate(tsid.EventType),
		Code:         code,
		Name:         name,
		SpecVersions: []SpecVersion{},
		Status:       StatusCurrent,
		Source:       SourceUI,
		ClientScoped: false,
		Application:  parts[0],
		Subdomain:    parts[1],
		Aggregate:    parts[2],
		EventName:    parts[3],
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// Lifecycle rules (the SPA drawer's guards, enforced by the archive and
// delete use cases). The messages are the ones the UIs show beside the
// disabled actions.
const (
	CannotArchiveReason = "Deprecate every schema version first."
	CannotDeleteReason  = "Only an archived event type, or one whose schema versions are all still finalising, can be deleted."
)

// CanArchive reports whether the event type may be archived: it is CURRENT
// and every schema version (if any) is DEPRECATED.
func (e *EventType) CanArchive() bool {
	if e.Status != StatusCurrent {
		return false
	}
	for _, sv := range e.SpecVersions {
		if sv.Status != SpecDeprecated {
			return false
		}
	}
	return true
}

// CanDelete reports whether the event type may be deleted: it is ARCHIVED,
// or CURRENT with every schema version (if any) still FINALISING (never
// published).
func (e *EventType) CanDelete() bool {
	if e.Status == StatusArchived {
		return true
	}
	if e.Status != StatusCurrent {
		return false
	}
	for _, sv := range e.SpecVersions {
		if sv.Status != SpecFinalising {
			return false
		}
	}
	return true
}

// Archive flips status to ARCHIVED and bumps UpdatedAt.
func (e *EventType) Archive() {
	e.Status = StatusArchived
	e.UpdatedAt = time.Now().UTC()
}

// AddSchemaVersion appends a schema version and bumps UpdatedAt.
func (e *EventType) AddSchemaVersion(sv SpecVersion) {
	e.SpecVersions = append(e.SpecVersions, sv)
	e.UpdatedAt = time.Now().UTC()
}
