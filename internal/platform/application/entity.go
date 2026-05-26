// Package application is the port of fc-platform/src/application.
// Represents a registered application (or integration) in the platform.
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

// ParseType is the lenient parser. Unknown → APPLICATION.
func ParseType(s string) Type {
	if s == string(TypeIntegration) {
		return TypeIntegration
	}
	return TypeApplication
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
