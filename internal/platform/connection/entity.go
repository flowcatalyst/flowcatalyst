// Package connection stores
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

// ParseStatus parses a stored/wire status value. Returns ok=false for
// anything other than ACTIVE or PAUSED — callers MUST reject on ok=false
// rather than coerce an unrecognised value to ACTIVE (X-06: a loud read
// error, never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseStatus(s string) (Status, bool) {
	switch Status(s) {
	case StatusActive, StatusPaused:
		return Status(s), true
	default:
		return "", false
	}
}

// Source identifies where the connection was authored. Mirrors
// subscription.Source exactly: a later code-first sync (not this package's
// job) needs to tell a code/API-authored row apart from a UI-authored one so
// it can refuse to touch the latter.
type Source string

const (
	SourceCode Source = "CODE"
	SourceAPI  Source = "API"
	SourceUI   Source = "UI"
)

// ParseSource parses a stored source value. Returns ok=false for anything
// other than CODE, API, or UI — callers MUST reject on ok=false rather than
// coerce an unrecognised value to UI (X-06: a loud read error, never a
// silent default). Follows the (T, bool) shape of common.ParseOutboxItemType.
func ParseSource(s string) (Source, bool) {
	switch Source(s) {
	case SourceCode, SourceAPI, SourceUI:
		return Source(s), true
	default:
		return "", false
	}
}

// Connection is the aggregate root.
type Connection struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	// ApplicationCode optionally links this connection to a registered
	// application, exactly like subscription.Subscription.ApplicationCode. A
	// connection with no application is "shared" (not "global" — that word is
	// reserved in this codebase for "no client").
	ApplicationCode  *string   `json:"applicationCode,omitempty"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	ExternalID       *string   `json:"externalId,omitempty"`
	Status           Status    `json:"status"`
	ServiceAccountID string    `json:"serviceAccountId"`
	ClientID         *string   `json:"clientId,omitempty"`
	ClientIdentifier *string   `json:"clientIdentifier,omitempty"`
	Source           Source    `json:"source"`
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
		Source:           SourceUI,
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
