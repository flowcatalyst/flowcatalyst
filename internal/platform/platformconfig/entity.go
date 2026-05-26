// Package platformconfig is the port of fc-platform/src/platform_config.
// Stores system-wide settings (one row per application:section:property)
// and a per-role access grant table.
package platformconfig

import (
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Scope is the visibility scope.
type Scope string

const (
	ScopeGlobal Scope = "GLOBAL"
	ScopeClient Scope = "CLIENT"
)

// ParseScope is the lenient parser. Unknown → GLOBAL.
func ParseScope(s string) Scope {
	if s == string(ScopeClient) {
		return ScopeClient
	}
	return ScopeGlobal
}

// ValueType identifies whether a value is plaintext or a secret reference.
type ValueType string

const (
	ValuePlain  ValueType = "PLAIN"
	ValueSecret ValueType = "SECRET"
)

// ParseValueType is the lenient parser. Unknown → PLAIN.
func ParseValueType(s string) ValueType {
	if s == string(ValueSecret) {
		return ValueSecret
	}
	return ValuePlain
}

// Config is a single config row.
type Config struct {
	ID              string    `json:"id"`
	ApplicationCode string    `json:"applicationCode"`
	Section         string    `json:"section"`
	Property        string    `json:"property"`
	Scope           Scope     `json:"scope"`
	ClientID        *string   `json:"clientId,omitempty"`
	ValueType       ValueType `json:"valueType"`
	Value           string    `json:"value"`
	Description     *string   `json:"description,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// IDStr satisfies usecase.HasID.
func (c Config) IDStr() string { return c.ID }

// NewConfig constructs a Config.
func NewConfig(app, section, property, value string) *Config {
	now := time.Now().UTC()
	return &Config{
		ID:              tsid.Generate(tsid.PlatformConfig),
		ApplicationCode: app,
		Section:         section,
		Property:        property,
		Scope:           ScopeGlobal,
		ValueType:       ValuePlain,
		Value:           value,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// MaskedValue returns "***" for secret values, the literal value otherwise.
// Use in list/get responses when the caller doesn't have read-secrets permission.
func (c *Config) MaskedValue() string {
	if c.ValueType == ValueSecret {
		return "***"
	}
	return c.Value
}

// Access is an access grant — role X may read/write configs for application Y.
type Access struct {
	ID              string    `json:"id"`
	ApplicationCode string    `json:"applicationCode"`
	RoleCode        string    `json:"roleCode"`
	CanRead         bool      `json:"canRead"`
	CanWrite        bool      `json:"canWrite"`
	CreatedAt       time.Time `json:"createdAt"`
}

// IDStr satisfies usecase.HasID.
func (a Access) IDStr() string { return a.ID }

// NewAccess constructs an Access grant (default canRead=true, canWrite=false).
func NewAccess(app, role string) *Access {
	return &Access{
		ID:              tsid.Generate(tsid.ConfigAccess),
		ApplicationCode: app,
		RoleCode:        role,
		CanRead:         true,
		CanWrite:        false,
		CreatedAt:       time.Now().UTC(),
	}
}
