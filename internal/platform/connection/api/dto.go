// dto.go contains the wire-format types for the connection API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateConnectionRequest is the wire body for POST /api/connections.
type CreateConnectionRequest struct {
	Code             string  `json:"code" doc:"Connection code (lowercase, alphanumeric, hyphens)"`
	ApplicationCode  *string `json:"applicationCode,omitempty" doc:"Optional application this connection belongs to. Omitted means shared (usable from any application)."`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	ServiceAccountID string  `json:"serviceAccountId"`
	ExternalID       *string `json:"externalId,omitempty"`
	ClientID         *string `json:"clientId,omitempty"`
}

func (r CreateConnectionRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:             r.Code,
		ApplicationCode:  r.ApplicationCode,
		Name:             r.Name,
		Description:      r.Description,
		ServiceAccountID: r.ServiceAccountID,
		ExternalID:       r.ExternalID,
		ClientID:         r.ClientID,
	}
}

// UpdateConnectionRequest is the wire body for PUT /api/connections/{id}.
type UpdateConnectionRequest struct {
	Name string `json:"name"`
	// ApplicationCode is set-if-provided: omitted leaves the existing link
	// alone (see operations.UpdateCommand.ApplicationCode).
	ApplicationCode *string `json:"applicationCode,omitempty"`
	Description     *string `json:"description,omitempty"`
	ExternalID      *string `json:"externalId,omitempty"`
	Status          *string `json:"status,omitempty"`
}

func (r UpdateConnectionRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:              id,
		Name:            r.Name,
		ApplicationCode: r.ApplicationCode,
		Description:     r.Description,
		ExternalID:      r.ExternalID,
		Status:          r.Status,
	}
}

// ConnectionResponse mirrors connection.Connection.
type ConnectionResponse struct {
	ID               string          `json:"id"`
	Code             string          `json:"code"`
	ApplicationCode  *string         `json:"applicationCode,omitempty"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitempty"`
	ExternalID       *string         `json:"externalId,omitempty"`
	Status           string          `json:"status"`
	ServiceAccountID string          `json:"serviceAccountId"`
	ClientID         *string         `json:"clientId,omitempty"`
	ClientIdentifier *string         `json:"clientIdentifier,omitempty"`
	Source           string          `json:"source"`
	CreatedAt        httpcompat.Time `json:"createdAt"`
	UpdatedAt        httpcompat.Time `json:"updatedAt"`
}

func fromEntity(c *connection.Connection) ConnectionResponse {
	return ConnectionResponse{
		ID:               c.ID,
		Code:             c.Code,
		ApplicationCode:  c.ApplicationCode,
		Name:             c.Name,
		Description:      c.Description,
		ExternalID:       c.ExternalID,
		Status:           string(c.Status),
		ServiceAccountID: c.ServiceAccountID,
		ClientID:         c.ClientID,
		ClientIdentifier: c.ClientIdentifier,
		Source:           string(c.Source),
		CreatedAt:        jsontime.New(c.CreatedAt),
		UpdatedAt:        jsontime.New(c.UpdatedAt),
	}
}

// ConnectionListResponse is the wire shape for GET /api/connections.
// SPA's ConnectionListPage reads `response.connections`.
type ConnectionListResponse struct {
	Connections []ConnectionResponse `json:"connections"`
	Total       int                  `json:"total"`
}
