// dto.go contains the wire-format types for the client API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateClientRequest is the wire body for POST /api/clients.
type CreateClientRequest struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier" doc:"URL-safe identifier (lowercase alphanumeric, hyphens)"`
}

func (r CreateClientRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Name:       r.Name,
		Identifier: r.Identifier,
	}
}

// UpdateClientRequest is the wire body for PUT /api/clients/{id}.
type UpdateClientRequest struct {
	Name *string `json:"name,omitempty"`
}

func (r UpdateClientRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{ID: id, Name: r.Name}
}

// SuspendClientRequest is the wire body for POST /api/clients/{id}/suspend.
type SuspendClientRequest struct {
	Reason string `json:"reason"`
}

// AddNoteRequest is the wire body for POST /api/clients/{id}/notes.
type AddNoteRequest struct {
	Category string `json:"category"`
	Text     string `json:"text"`
}

// SearchClientRequest is the wire body for POST /api/clients/search.
type SearchClientRequest struct {
	Term string `json:"term"`
}

// NoteResponse mirrors client.Note.
type NoteResponse struct {
	Category string          `json:"category"`
	Text     string          `json:"text"`
	AddedBy  *string         `json:"addedBy,omitempty"`
	AddedAt  httpcompat.Time `json:"addedAt"`
}

// ClientResponse mirrors client.Client.
type ClientResponse struct {
	ID              string           `json:"id"`
	Name            string           `json:"name"`
	Identifier      string           `json:"identifier"`
	Status          string           `json:"status"`
	StatusReason    *string          `json:"statusReason,omitempty"`
	StatusChangedAt *httpcompat.Time `json:"statusChangedAt,omitempty"`
	Notes           []NoteResponse   `json:"notes"`
	CreatedAt       httpcompat.Time  `json:"createdAt"`
	UpdatedAt       httpcompat.Time  `json:"updatedAt"`
}

func fromEntity(c *client.Client) ClientResponse {
	notes := make([]NoteResponse, 0, len(c.Notes))
	for _, n := range c.Notes {
		notes = append(notes, NoteResponse{
			Category: n.Category,
			Text:     n.Text,
			AddedBy:  n.AddedBy,
			AddedAt:  jsontime.New(n.AddedAt),
		})
	}
	var statusChanged *httpcompat.Time
	if c.StatusChangedAt != nil {
		v := jsontime.New(*c.StatusChangedAt)
		statusChanged = &v
	}
	return ClientResponse{
		ID:              c.ID,
		Name:            c.Name,
		Identifier:      c.Identifier,
		Status:          string(c.Status),
		StatusReason:    c.StatusReason,
		StatusChangedAt: statusChanged,
		Notes:           notes,
		CreatedAt:       jsontime.New(c.CreatedAt),
		UpdatedAt:       jsontime.New(c.UpdatedAt),
	}
}

// ClientListResponse is the wire shape for GET /api/clients.
type ClientListResponse struct {
	Items []ClientResponse `json:"items"`
}
