package client

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
)

// SecretMask is the literal the platform returns as the value of every
// property whose valueType is SECRET, from every ordinary read (Get, and the
// response to Set) and for every caller. The plaintext is available only
// through PlatformConfigResource.RevealSecret.
const SecretMask = "***"

// ─── Request / response DTOs ─────────────────────────────────────────

// SetConfigPropertyRequest — PUT /api/config/{app}/{section}/{property}.
type SetConfigPropertyRequest struct {
	// Value is required. For a SECRET property it is the plaintext to store
	// (the platform encrypts it at rest).
	Value string `json:"value"`
	// ValueType is PLAIN or SECRET. Empty leaves the platform default.
	ValueType   string `json:"valueType,omitempty"`
	Description string `json:"description,omitempty"`
	// ClientID scopes the property to one client. Empty sets the global value.
	ClientID string `json:"clientId,omitempty"`
}

// ConfigResponse is one platform-config property. When ValueType is SECRET,
// Value is always SecretMask ("***"): use RevealSecret for the plaintext.
type ConfigResponse struct {
	ID              string `json:"id"`
	ApplicationCode string `json:"applicationCode"`
	Section         string `json:"section"`
	Property        string `json:"property"`
	Scope           string `json:"scope"`
	ClientID        string `json:"clientId,omitempty"`
	ValueType       string `json:"valueType"`
	// Value is "***" for SECRET properties; see RevealSecret.
	Value       string `json:"value"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

// SecretResponse is the plaintext of one SECRET property, as returned by
// RevealSecret. Value is sensitive: String, GoString and LogValue (slog)
// omit it, so printing or logging the struct with %v, %+v, %#v or slog does
// not leak it. Reading the Value field and encoding/json are not redacted;
// do not marshal this struct into logs.
type SecretResponse struct {
	ApplicationCode string `json:"applicationCode"`
	Section         string `json:"section"`
	Property        string `json:"property"`
	ClientID        string `json:"clientId,omitempty"`
	// Value is the plaintext secret. Never log it.
	Value string `json:"value"`
	// UpdatedAt is the value's version: it changes whenever the secret is
	// set. Compare it to detect rotation.
	UpdatedAt string `json:"updatedAt"`
}

// String renders the response with the value redacted.
func (s SecretResponse) String() string {
	return fmt.Sprintf("SecretResponse{applicationCode=%q section=%q property=%q clientId=%q value=[REDACTED] updatedAt=%q}",
		s.ApplicationCode, s.Section, s.Property, s.ClientID, s.UpdatedAt)
}

// GoString implements fmt.GoStringer (%#v) with the value redacted.
func (s SecretResponse) GoString() string { return s.String() }

// LogValue implements slog.LogValuer with the value redacted.
func (s SecretResponse) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("applicationCode", s.ApplicationCode),
		slog.String("section", s.Section),
		slog.String("property", s.Property),
		slog.String("clientId", s.ClientID),
		slog.String("value", "[REDACTED]"),
		slog.String("updatedAt", s.UpdatedAt),
	)
}

// ─── Resource ────────────────────────────────────────────────────────

// PlatformConfigResource — /api/config/*.
type PlatformConfigResource struct {
	c *FlowCatalystClient
}

func configPath(app, section, property string) string {
	return "/api/config/" + url.PathEscape(app) + "/" + url.PathEscape(section) + "/" + url.PathEscape(property)
}

// Get — GET /api/config/{app}/{section}/{property}?clientId=. Empty clientID
// is omitted. A SECRET property comes back with Value "***" (SecretMask);
// call RevealSecret for its plaintext.
func (r *PlatformConfigResource) Get(ctx context.Context, app, section, property, clientID string) (*ConfigResponse, error) {
	var out ConfigResponse
	q := NewQuery().String("clientId", clientID).Encode()
	if err := r.c.Get(ctx, configPath(app, section, property)+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Set — PUT /api/config/{app}/{section}/{property}. Returns the stored
// property; for a SECRET the returned Value is "***".
func (r *PlatformConfigResource) Set(ctx context.Context, app, section, property string, req *SetConfigPropertyRequest) (*ConfigResponse, error) {
	var out ConfigResponse
	// The platform declares clientId both as a query parameter and a body
	// field; send it in both so either reading of the contract holds.
	q := NewQuery().String("clientId", req.ClientID).Encode()
	if err := r.c.Put(ctx, configPath(app, section, property)+q, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Delete — DELETE /api/config/{app}/{section}/{property}?clientId=. Empty
// clientID deletes the global value.
func (r *PlatformConfigResource) Delete(ctx context.Context, app, section, property, clientID string) error {
	q := NewQuery().String("clientId", clientID).Encode()
	return r.c.Delete(ctx, configPath(app, section, property)+q, nil)
}

// RevealSecret — GET /api/config/{app}/{section}/{property}/secret?clientId=.
// The only way to read a SECRET property's plaintext; every other read
// returns "***". Empty clientID is omitted.
//
// Errors are *APIError: 403 when the caller lacks permission
// platform:admin:config:reveal and is not the application's own service
// account; 404 when the property does not exist; 400 with Code()
// "NOT_A_SECRET" when the property is PLAIN (use Get).
//
// Every reveal is audited by the platform. The result is deliberately not
// cached by the SDK (wrap the call in your own TTL cache if you need one),
// and the value is never logged or placed in an error by the SDK.
func (r *PlatformConfigResource) RevealSecret(ctx context.Context, app, section, property, clientID string) (*SecretResponse, error) {
	var out SecretResponse
	q := NewQuery().String("clientId", clientID).Encode()
	if err := r.c.Get(ctx, configPath(app, section, property)+"/secret"+q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
