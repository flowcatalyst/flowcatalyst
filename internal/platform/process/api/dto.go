// dto.go contains the wire-format types for the process API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateProcessRequest is the wire body for POST /api/processes.
type CreateProcessRequest struct {
	Code        string   `json:"code" doc:"Process code in application:subdomain:name format"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Body        string   `json:"body,omitempty" doc:"Process documentation body"`
	DiagramType string   `json:"diagramType,omitempty" doc:"Diagram syntax (e.g. mermaid)"`
	Tags        []string `json:"tags,omitempty"`
}

func (r CreateProcessRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		Body:        r.Body,
		DiagramType: r.DiagramType,
		Tags:        r.Tags,
	}
}

// UpdateProcessRequest is the wire body for PUT /api/processes/{id}.
type UpdateProcessRequest struct {
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Body        *string  `json:"body,omitempty"`
	DiagramType *string  `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

func (r UpdateProcessRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:          id,
		Name:        r.Name,
		Description: r.Description,
		Body:        r.Body,
		DiagramType: r.DiagramType,
		Tags:        r.Tags,
	}
}

// ProcessResponse mirrors process.Process.
type ProcessResponse struct {
	ID          string          `json:"id"`
	Code        string          `json:"code"`
	Name        string          `json:"name"`
	Description *string         `json:"description,omitempty"`
	Status      string          `json:"status"`
	Source      string          `json:"source"`
	Application string          `json:"application"`
	Subdomain   string          `json:"subdomain"`
	ProcessName string          `json:"processName"`
	Body        string          `json:"body"`
	DiagramType string          `json:"diagramType"`
	Tags        []string        `json:"tags"`
	CreatedBy   *string         `json:"createdBy,omitempty"`
	CreatedAt   httpcompat.Time `json:"createdAt"`
	UpdatedAt   httpcompat.Time `json:"updatedAt"`
}

func fromEntity(p *process.Process) ProcessResponse {
	tags := p.Tags
	if tags == nil {
		tags = []string{}
	}
	return ProcessResponse{
		ID:          p.ID,
		Code:        p.Code,
		Name:        p.Name,
		Description: p.Description,
		Status:      string(p.Status),
		Source:      string(p.Source),
		Application: p.Application,
		Subdomain:   p.Subdomain,
		ProcessName: p.ProcessName,
		Body:        p.Body,
		DiagramType: p.DiagramType,
		Tags:        tags,
		CreatedBy:   p.CreatedBy,
		CreatedAt:   jsontime.New(p.CreatedAt),
		UpdatedAt:   jsontime.New(p.UpdatedAt),
	}
}

// ProcessListResponse is the wire shape for GET /api/processes.
type ProcessListResponse struct {
	Items []ProcessResponse `json:"items"`
}
