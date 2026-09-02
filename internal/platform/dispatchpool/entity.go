// Package dispatchpool encapsulates rate-limit + concurrency settings
// used by the message router.
package dispatchpool

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Status is the lifecycle state of a pool.
type Status string

const (
	StatusActive    Status = "ACTIVE"
	StatusSuspended Status = "SUSPENDED"
	StatusArchived  Status = "ARCHIVED"
)

// ParseStatus parses a stored status value. Returns ok=false for anything
// other than ACTIVE, SUSPENDED, or ARCHIVED — callers MUST reject on
// ok=false rather than coerce an unrecognised value to ACTIVE (X-06: a loud
// read error, never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseStatus(s string) (Status, bool) {
	switch Status(s) {
	case StatusActive, StatusSuspended, StatusArchived:
		return Status(s), true
	default:
		return "", false
	}
}

// DispatchPool is the aggregate root.
type DispatchPool struct {
	ID          string  `json:"id"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	// RateLimit is messages per minute. nil → no rate limit, concurrency-only.
	RateLimit        *int32    `json:"rateLimit,omitempty"`
	Concurrency      int32     `json:"concurrency"`
	ClientID         *string   `json:"clientId,omitempty"`
	ClientIdentifier *string   `json:"clientIdentifier,omitempty"`
	Status           Status    `json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (p DispatchPool) IDStr() string { return p.ID }

// New constructs a DispatchPool with defaults (concurrency=10, status=ACTIVE).
func New(code, name string) *DispatchPool {
	now := time.Now().UTC()
	return &DispatchPool{
		ID:          tsid.Generate(tsid.DispatchPool),
		Code:        code,
		Name:        name,
		Concurrency: 10,
		Status:      StatusActive,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// Suspend flips status to SUSPENDED.
func (p *DispatchPool) Suspend() {
	p.Status = StatusSuspended
	p.UpdatedAt = time.Now().UTC()
}

// Activate flips status to ACTIVE.
func (p *DispatchPool) Activate() {
	p.Status = StatusActive
	p.UpdatedAt = time.Now().UTC()
}

// Archive flips status to ARCHIVED.
func (p *DispatchPool) Archive() {
	p.Status = StatusArchived
	p.UpdatedAt = time.Now().UTC()
}
