// Package connection is the port of fc-platform/src/connection. Stores
// outbound webhook delivery targets (subscriber endpoints).
package connection

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the connection lifecycle state.
type Status string

const (
	StatusActive Status = "ACTIVE"
	StatusPaused Status = "PAUSED"
)

// ParseStatus is the lenient parser. Unknown → ACTIVE.
func ParseStatus(s string) Status {
	if s == string(StatusPaused) {
		return StatusPaused
	}
	return StatusActive
}

// Connection is the aggregate root.
type Connection struct {
	ID               string    `json:"id"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	ExternalID       *string   `json:"externalId,omitempty"`
	Status           Status    `json:"status"`
	ServiceAccountID string    `json:"serviceAccountId"`
	ClientID         *string   `json:"clientId,omitempty"`
	ClientIdentifier *string   `json:"clientIdentifier,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (c Connection) IDStr() string { return c.ID }

// New constructs a Connection.
func New(code, name, serviceAccountID string) *Connection {
	now := time.Now().UTC()
	return &Connection{
		ID:               tsid.Generate(tsid.Connection),
		Code:             code,
		Name:             name,
		Status:           StatusActive,
		ServiceAccountID: serviceAccountID,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

// Pause flips the status to PAUSED.
func (c *Connection) Pause() {
	c.Status = StatusPaused
	c.UpdatedAt = time.Now().UTC()
}

// Activate flips the status back to ACTIVE.
func (c *Connection) Activate() {
	c.Status = StatusActive
	c.UpdatedAt = time.Now().UTC()
}
