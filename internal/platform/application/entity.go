// Package application represents a registered application (or
// integration) in the platform.
//
// TODO(wave-3b-follow-up): port client_config sub-aggregate and
// tenant-relation ops (attach_service_account, enable_for_client,
// disable_for_client, update_client_applications, update_client_config).
package application

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Type is the application kind.
type Type string

const (
	TypeApplication Type = "APPLICATION"
	TypeIntegration Type = "INTEGRATION"
)

// ParseType parses a stored/wire type value. Returns ok=false for anything
// other than APPLICATION or INTEGRATION — callers MUST reject on ok=false
// rather than coerce an unrecognised value to APPLICATION (X-06: a loud
// read error, never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseType(s string) (Type, bool) {
	switch Type(s) {
	case TypeApplication, TypeIntegration:
		return Type(s), true
	default:
		return "", false
	}
}

// Application is the aggregate root.
type Application struct {
	ID               string    `json:"id"`
	Type             Type      `json:"type"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Description      *string   `json:"description,omitempty"`
	IconURL          *string   `json:"iconUrl,omitempty"`
	Website          *string   `json:"website,omitempty"`
	Logo             *string   `json:"logo,omitempty"`
	LogoMimeType     *string   `json:"logoMimeType,omitempty"`
	DefaultBaseURL   *string   `json:"defaultBaseUrl,omitempty"`
	ServiceAccountID *string   `json:"serviceAccountId,omitempty"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (a Application) IDStr() string { return a.ID }

// New constructs an Application (default Type=APPLICATION, Active=true).
func New(code, name string) *Application {
	now := time.Now().UTC()
	return &Application{
		ID:        tsid.Generate(tsid.Application),
		Type:      TypeApplication,
		Code:      code,
		Name:      name,
		Active:    true,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// NewIntegration constructs an Application with Type=INTEGRATION.
func NewIntegration(code, name string) *Application {
	a := New(code, name)
	a.Type = TypeIntegration
	return a
}

// IsIntegration reports whether the app is an integration.
func (a *Application) IsIntegration() bool { return a.Type == TypeIntegration }

// Activate flips Active=true.
func (a *Application) Activate() {
	a.Active = true
	a.UpdatedAt = time.Now().UTC()
}

// Deactivate flips Active=false.
func (a *Application) Deactivate() {
	a.Active = false
	a.UpdatedAt = time.Now().UTC()
}
