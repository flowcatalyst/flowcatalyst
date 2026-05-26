// Package cors is the port of fc-platform/src/cors. Stores allowed
// CORS origins for the platform's HTTP layer.
package cors

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// AllowedOrigin is the aggregate root.
type AllowedOrigin struct {
	ID          string    `json:"id"`
	Origin      string    `json:"origin"`
	Description *string   `json:"description,omitempty"`
	CreatedBy   *string   `json:"createdBy,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (a AllowedOrigin) IDStr() string { return a.ID }

// New constructs an AllowedOrigin with a fresh TSID.
func New(origin string, description, createdBy *string) *AllowedOrigin {
	now := time.Now().UTC()
	return &AllowedOrigin{
		ID:          tsid.Generate(tsid.CorsOrigin),
		Origin:      origin,
		Description: description,
		CreatedBy:   createdBy,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}
