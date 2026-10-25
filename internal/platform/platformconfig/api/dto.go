// dto.go contains the wire-format types for the platform_config API.
package api

import (
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// SetPropertyRequest is the wire body for
// PUT /api/platform-config/{app}/{section}/{property}.
type SetPropertyRequest struct {
	Value       string  `json:"value"`
	ValueType   *string `json:"valueType,omitempty" doc:"PLAIN or SECRET"`
	Description *string `json:"description,omitempty"`
	ClientID    *string `json:"clientId,omitempty"`
}

func (r SetPropertyRequest) toCommand(app, section, property string) operations.SetPropertyCommand {
	return operations.SetPropertyCommand{
		ApplicationCode: app,
		Section:         section,
		Property:        property,
		Value:           r.Value,
		ValueType:       r.ValueType,
		Description:     r.Description,
		ClientID:        r.ClientID,
	}
}

// GrantAccessRequest is the wire body for POST /api/platform-config/{app}/access.
type GrantAccessRequest struct {
	RoleCode string `json:"roleCode"`
	CanWrite bool   `json:"canWrite"`
}

func (r GrantAccessRequest) toCommand(app string) operations.GrantAccessCommand {
	return operations.GrantAccessCommand{
		ApplicationCode: app,
		RoleCode:        r.RoleCode,
		CanWrite:        r.CanWrite,
	}
}

// ConfigResponse mirrors platformconfig.Config.
type ConfigResponse struct {
	ID              string  `json:"id"`
	ApplicationCode string  `json:"applicationCode"`
	Section         string  `json:"section"`
	Property        string  `json:"property"`
	Scope           string  `json:"scope"`
	ClientID        *string `json:"clientId,omitempty"`
	// ClientIdentifier is the client's identifier (e.g. acme) beside its
	// stored id, on list answers, so a caller holding either can match rows.
	ClientIdentifier *string         `json:"clientIdentifier,omitempty"`
	ValueType        string          `json:"valueType"`
	Value            string          `json:"value"`
	Description      *string         `json:"description,omitempty"`
	CreatedAt        httpcompat.Time `json:"createdAt"`
	UpdatedAt        httpcompat.Time `json:"updatedAt"`
}

func configFromEntity(c *platformconfig.Config) ConfigResponse {
	return ConfigResponse{
		ID:              c.ID,
		ApplicationCode: c.ApplicationCode,
		Section:         c.Section,
		Property:        c.Property,
		Scope:           string(c.Scope),
		ClientID:        ids.StringPtr(c.ClientID),
		ValueType:       string(c.ValueType),
		// A SECRET is never serialised: reveal is its own, audited endpoint.
		Value:       c.MaskedValue(),
		Description: c.Description,
		CreatedAt:   jsontime.New(c.CreatedAt),
		UpdatedAt:   jsontime.New(c.UpdatedAt),
	}
}

// SecretResponse is the body of the reveal endpoint: the one place a SECRET's
// plaintext leaves the platform.
type SecretResponse struct {
	ApplicationCode string          `json:"applicationCode"`
	Section         string          `json:"section"`
	Property        string          `json:"property"`
	ClientID        *string         `json:"clientId,omitempty"`
	Value           string          `json:"value"`
	UpdatedAt       httpcompat.Time `json:"updatedAt" doc:"The value's version: it changes whenever the secret is set. Use it to detect rotation."`
}

// AccessResponse mirrors platformconfig.Access.
type AccessResponse struct {
	ID              string          `json:"id"`
	ApplicationCode string          `json:"applicationCode"`
	RoleCode        string          `json:"roleCode"`
	CanRead         bool            `json:"canRead"`
	CanWrite        bool            `json:"canWrite"`
	CreatedAt       httpcompat.Time `json:"createdAt"`
}

func accessFromEntity(a *platformconfig.Access) AccessResponse {
	return AccessResponse{
		ID:              a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
		CanRead:         a.CanRead,
		CanWrite:        a.CanWrite,
		CreatedAt:       jsontime.New(a.CreatedAt),
	}
}

// ConfigListResponse is the wire shape for GET /api/platform-config/{app}.
type ConfigListResponse struct {
	Items []ConfigResponse `json:"items"`
}

// AccessListResponse is the wire shape for GET /api/platform-config/{app}/access.
type AccessListResponse struct {
	Items []AccessResponse `json:"items"`
}
