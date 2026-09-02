// Package process stores
// free-form workflow documentation (typically Mermaid diagrams)
// scoped to {application, subdomain, process-name}.
package process

import (
	"errors"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the lifecycle state.
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

// Source identifies where the process was authored.
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

// Process is the aggregate root.
type Process struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	Status      Status    `json:"status"`
	Source      Source    `json:"source"`
	Application string    `json:"application"`
	Subdomain   string    `json:"subdomain"`
	ProcessName string    `json:"processName"`
	Body        string    `json:"body"`
	DiagramType string    `json:"diagramType"`
	Tags        []string  `json:"tags"`
	CreatedBy   *string   `json:"createdBy,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (p Process) IDStr() string { return p.ID }

// New constructs a Process from a colon-separated code.
func New(code, name string) (*Process, error) {
	parts := strings.Split(code, ":")
	if len(parts) != 3 {
		return nil, errors.New("Process code must follow format: application:subdomain:process-name")
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return nil, errors.New("Process code segments cannot be empty")
		}
	}
	now := time.Now().UTC()
	return &Process{
		ID:          tsid.Generate(tsid.Process),
		Code:        code,
		Name:        name,
		Status:      StatusCurrent,
		Source:      SourceUI,
		Application: parts[0],
		Subdomain:   parts[1],
		ProcessName: parts[2],
		DiagramType: "mermaid",
		Tags:        []string{},
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// Archive flips status to ARCHIVED.
func (p *Process) Archive() {
	p.Status = StatusArchived
	p.UpdatedAt = time.Now().UTC()
}
