// Package client is the port of fc-platform/src/client. Tenant /
// organization root in the multi-tenant model. Subscriptions, dispatch
// pools, applications, principals, etc. all hang off a Client via
// client_id.
package client

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the tenant lifecycle state.
type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusInactive  Status = "INACTIVE"
	StatusSuspended Status = "SUSPENDED"
)

// ParseStatus is the lenient parser. Unknown → ACTIVE.
func ParseStatus(s string) Status {
	switch s {
	case string(StatusInactive):
		return StatusInactive
	case string(StatusSuspended):
		return StatusSuspended
	default:
		return StatusActive
	}
}

// Note is an audit-trail entry. Stored as JSONB in PostgreSQL.
type Note struct {
	Category string    `json:"category"`
	Text     string    `json:"text"`
	AddedBy  *string   `json:"addedBy,omitempty"`
	AddedAt  time.Time `json:"addedAt"`
}

// NewNote builds a fresh note.
func NewNote(category, text string, addedBy *string) Note {
	return Note{
		Category: category,
		Text:     text,
		AddedBy:  addedBy,
		AddedAt:  time.Now().UTC(),
	}
}

// Client is the aggregate root.
type Client struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	Identifier      string     `json:"identifier"`
	Status          Status     `json:"status"`
	StatusReason    *string    `json:"statusReason,omitempty"`
	StatusChangedAt *time.Time `json:"statusChangedAt,omitempty"`
	Notes           []Note     `json:"notes"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (c Client) IDStr() string { return c.ID }

// New constructs a Client (default Status=ACTIVE).
func New(name, identifier string) *Client {
	now := time.Now().UTC()
	return &Client{
		ID:         tsid.Generate(tsid.Client),
		Name:       name,
		Identifier: identifier,
		Status:     StatusActive,
		Notes:      []Note{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// AddNote appends a note and bumps UpdatedAt.
func (c *Client) AddNote(n Note) {
	c.Notes = append(c.Notes, n)
	c.UpdatedAt = time.Now().UTC()
}

// SetStatus transitions and records the change time and reason.
func (c *Client) SetStatus(s Status, reason *string) {
	now := time.Now().UTC()
	c.Status = s
	c.StatusReason = reason
	c.StatusChangedAt = &now
	c.UpdatedAt = now
}

// Suspend is a convenience for SetStatus(StatusSuspended, &reason).
func (c *Client) Suspend(reason string) { c.SetStatus(StatusSuspended, &reason) }

// Activate is a convenience for SetStatus(StatusActive, nil).
func (c *Client) Activate() { c.SetStatus(StatusActive, nil) }
