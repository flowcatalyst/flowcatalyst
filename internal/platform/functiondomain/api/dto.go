// dto.go contains the wire-format types for the function-domain API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/functiondomain/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateDomainRequest is the wire body for POST /api/function-domains.
type CreateDomainRequest struct {
	Zone     string  `json:"zone" doc:"The claimed hostname zone (e.g. \"acme.example.com\")"`
	ClientID *string `json:"clientId,omitempty" doc:"Omitted claims a platform-owned zone (anchor only)"`
}

func (r CreateDomainRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{Zone: r.Zone, ClientID: r.ClientID}
}

// DomainResponse mirrors functiondomain.FunctionDomain.
type DomainResponse struct {
	ID        string          `json:"id"`
	Zone      string          `json:"zone"`
	ClientID  *string         `json:"clientId,omitempty"`
	CreatedBy *string         `json:"createdBy,omitempty"`
	CreatedAt httpcompat.Time `json:"createdAt"`
	UpdatedAt httpcompat.Time `json:"updatedAt"`
}

func fromEntity(d *functiondomain.FunctionDomain) DomainResponse {
	return DomainResponse{
		ID:        d.ID,
		Zone:      d.Zone,
		ClientID:  d.ClientID,
		CreatedBy: d.CreatedBy,
		CreatedAt: jsontime.New(d.CreatedAt),
		UpdatedAt: jsontime.New(d.UpdatedAt),
	}
}
