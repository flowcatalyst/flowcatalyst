// Package platformconfig
// stores system-wide settings (one row per application:section:property)
// and a per-role access grant table.
package platformconfig

import (
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Scope is the visibility scope.
type Scope string

const (
	ScopeGlobal Scope = "GLOBAL"
	ScopeClient Scope = "CLIENT"
)

// ParseScope parses a stored/wire scope value. Returns ok=false for
// anything other than GLOBAL or CLIENT — callers MUST reject on ok=false
// rather than coerce an unrecognised value to GLOBAL (X-06: a loud read
// error, never a silent default). Follows the (T, bool) shape of
// common.ParseOutboxItemType.
func ParseScope(s string) (Scope, bool) {
	switch Scope(s) {
	case ScopeGlobal, ScopeClient:
		return Scope(s), true
	default:
		return "", false
	}
}

// ValueType identifies whether a value is plaintext or a secret reference.
type ValueType string

const (
	ValuePlain  ValueType = "PLAIN"
	ValueSecret ValueType = "SECRET"
)

// ParseValueType parses a stored/wire value-type value. Returns ok=false
// for anything other than PLAIN or SECRET — callers MUST reject on
// ok=false rather than coerce an unrecognised value to PLAIN (X-06: a loud
// read error, never a silent default — and PLAIN is the LESS protected
// type, so a coerced default here risks treating a secret reference as
// displayable plaintext, not just a display bug). Follows the (T, bool)
// shape of common.ParseOutboxItemType.
func ParseValueType(s string) (ValueType, bool) {
	switch ValueType(s) {
	case ValuePlain, ValueSecret:
		return ValueType(s), true
	default:
		return "", false
	}
}

// ReservedSection is the section that holds the per-environment settings of
// ingestions (docs/ingestion-design.md section 13). Its entries are written
// only through the ingestion that names them, which validates them against
// every ingestion that does; the generic write surfaces refuse it
// (RequireWritableSection). A read is as for any section except that the value
// of an entry is shown only to a caller who can update ingestions
// (ingestion:update): the settings name accounts and endpoints, and the people
// who merely view ingestions see the entry's status, not its content. For
// everyone else Service.List and Service.Get blank the value and mark the row
// Hidden.
const ReservedSection = "ingest"

// RequireWritableSection refuses a write or a delete in a reserved section:
// 400 CONFIG_SECTION_RESERVED.
func RequireWritableSection(section string) error {
	if strings.EqualFold(strings.TrimSpace(section), ReservedSection) {
		return usecase.Validation("CONFIG_SECTION_RESERVED",
			"section '"+ReservedSection+"' holds ingestion settings, which are changed on the ingestion (PUT /api/ingestions/{id}/config), not here")
	}
	return nil
}

// EncryptedPrefix marks a SECRET value stored encrypted at rest.
const EncryptedPrefix = "encrypted:"

// Config is a single config row. For a SECRET, Value is the stored
// representation (ciphertext) when read from the repository and the plaintext
// when about to be written; it must never reach a response: use MaskedValue.
type Config struct {
	ID              string        `json:"id"`
	ApplicationCode string        `json:"applicationCode"`
	Section         string        `json:"section"`
	Property        string        `json:"property"`
	Scope           Scope         `json:"scope"`
	ClientID        *ids.ClientID `json:"clientId,omitempty"`
	ValueType       ValueType     `json:"valueType"`
	Value           string        `json:"value"`
	Description     *string       `json:"description,omitempty"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	// Hidden is set by the read service (Service.List, Service.Get) on a row
	// whose value the caller may not see, with Value blanked. It is never
	// stored.
	Hidden bool `json:"-"`
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

// MaskedValue returns "***" for secret values and for a value the read service
// hid from the caller (Hidden), the literal value otherwise. Use in list/get
// responses when the caller doesn't have read-secrets permission.
func (c *Config) MaskedValue() string {
	if c.ValueType == ValueSecret || c.Hidden {
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
