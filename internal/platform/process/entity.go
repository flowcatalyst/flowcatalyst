// Package process is the port of fc-platform/src/process. Stores
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

// ParseStatus is the lenient parser. Unknown → CURRENT.
func ParseStatus(s string) Status {
	if s == string(StatusArchived) {
		return StatusArchived
	}
	return StatusCurrent
}

// Source identifies where the process was authored.
type Source string

const (
	SourceCode Source = "CODE"
	SourceAPI  Source = "API"
	SourceUI   Source = "UI"
)

// ParseSource is the lenient parser. Unknown → UI.
func ParseSource(s string) Source {
	switch s {
	case string(SourceCode):
		return SourceCode
	case string(SourceAPI):
		return SourceAPI
	default:
		return SourceUI
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
