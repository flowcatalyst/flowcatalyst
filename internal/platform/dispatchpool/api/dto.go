// dto.go contains the wire-format types for the dispatch_pool API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// CreateDispatchPoolRequest is the wire body for POST /api/dispatch-pools.
type CreateDispatchPoolRequest struct {
	Code        string  `json:"code" doc:"Pool code (lowercase, alphanumeric, hyphens)"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty" doc:"Messages per minute (nil = no rate limit)"`
	Concurrency *int32  `json:"concurrency,omitempty" doc:"Max concurrent dispatches (default 10)"`
	ClientID    *string `json:"clientId,omitempty"`
}

func (r CreateDispatchPoolRequest) toCommand() operations.CreateCommand {
	return operations.CreateCommand{
		Code:        r.Code,
		Name:        r.Name,
		Description: r.Description,
		RateLimit:   r.RateLimit,
		Concurrency: r.Concurrency,
		ClientID:    r.ClientID,
	}
}

// UpdateDispatchPoolRequest is the wire body for PUT /api/dispatch-pools/{id}.
type UpdateDispatchPoolRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty"`
	Concurrency *int32  `json:"concurrency,omitempty"`
}

func (r UpdateDispatchPoolRequest) toCommand(id string) operations.UpdateCommand {
	return operations.UpdateCommand{
		ID:          id,
		Name:        r.Name,
		Description: r.Description,
		RateLimit:   r.RateLimit,
		Concurrency: r.Concurrency,
	}
}

// DispatchPoolResponse mirrors dispatchpool.DispatchPool.
type DispatchPoolResponse struct {
	ID               string          `json:"id"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitempty"`
	RateLimit        *int32          `json:"rateLimit,omitempty"`
	Concurrency      int32           `json:"concurrency"`
	ClientID         *string         `json:"clientId,omitempty"`
	ClientIdentifier *string         `json:"clientIdentifier,omitempty"`
	Status           string          `json:"status"`
	CreatedAt        httpcompat.Time `json:"createdAt"`
	UpdatedAt        httpcompat.Time `json:"updatedAt"`
}

func fromEntity(p *dispatchpool.DispatchPool) DispatchPoolResponse {
	return DispatchPoolResponse{
		ID:               p.ID,
		Code:             p.Code,
		Name:             p.Name,
		Description:      p.Description,
		RateLimit:        p.RateLimit,
		Concurrency:      p.Concurrency,
		ClientID:         p.ClientID,
		ClientIdentifier: p.ClientIdentifier,
		Status:           string(p.Status),
		CreatedAt:        jsontime.New(p.CreatedAt),
		UpdatedAt:        jsontime.New(p.UpdatedAt),
	}
}

// DispatchPoolListResponse is the wire shape for GET /api/dispatch-pools.
type DispatchPoolListResponse struct {
	Items []DispatchPoolResponse `json:"items"`
}
