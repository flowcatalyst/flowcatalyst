package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

type AddOriginRequest struct {
	Origin      string  `json:"origin" doc:"CORS-allowed origin (e.g. https://example.com)"`
	Description *string `json:"description,omitempty"`
}

func (r AddOriginRequest) toCommand() operations.AddCommand {
	return operations.AddCommand{Origin: r.Origin, Description: r.Description}
}

type AllowedOriginResponse struct {
	ID          string          `json:"id"`
	Origin      string          `json:"origin"`
	Description *string         `json:"description,omitempty"`
	CreatedBy   *string         `json:"createdBy,omitempty"`
	CreatedAt   httpcompat.Time `json:"createdAt"`
	UpdatedAt   httpcompat.Time `json:"updatedAt"`
}

func fromEntity(o *cors.AllowedOrigin) AllowedOriginResponse {
	return AllowedOriginResponse{
		ID:          o.ID,
		Origin:      o.Origin,
		Description: o.Description,
		CreatedBy:   o.CreatedBy,
		CreatedAt:   jsontime.New(o.CreatedAt),
		UpdatedAt:   jsontime.New(o.UpdatedAt),
	}
}

type CorsOriginListResponse struct {
	CorsOrigins []AllowedOriginResponse `json:"corsOrigins"`
	Total       int                     `json:"total"`
}

type PublicAllowedResponse struct {
	AllowedOrigins []string `json:"allowedOrigins"`
}
