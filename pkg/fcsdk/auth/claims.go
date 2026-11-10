package auth

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// Audience is a JWT aud claim. RFC 7519 allows aud to be either a
// string or an array of strings. FlowCatalyst-minted tokens emit a
// single string, but tokens minted with other
// tooling (e.g. jwx itself) emit an array. We accept both forms on
// unmarshal and always marshal as a single string for wire parity.
type Audience string

// UnmarshalJSON accepts either a string or a one-element string array.
// Multi-element arrays take the first entry.
func (a *Audience) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*a = Audience(s)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		if len(arr) == 0 {
			*a = ""
		} else {
			*a = Audience(arr[0])
		}
		return nil
	}
	return fmt.Errorf("aud claim must be a string or array of strings, got %s", b)
}

func (a Audience) String() string { return string(a) }

// AccessTokenClaims is the JWT payload shape issued by FlowCatalyst's
// /oauth/token endpoint. JSON tags match the wire contract byte-for-byte
// so the same token deserialises identically across SDKs.
type AccessTokenClaims struct {
	// Sub is the principal id, e.g. "prn_0HZXEQ5Y8JY5Z".
	Sub string `json:"sub"`
	// Iss is the issuer URL.
	Iss string `json:"iss"`
	// Aud is the audience. Accepts string or []string on the wire.
	Aud Audience `json:"aud"`
	// Exp is the expiration time (Unix seconds).
	Exp int64 `json:"exp"`
	// Iat is the issued-at time (Unix seconds).
	Iat int64 `json:"iat"`
	// Nbf is the not-before time (Unix seconds).
	Nbf int64 `json:"nbf"`
	// Jti is the JWT id.
	Jti string `json:"jti"`
	// PrincipalType is "USER" or "SERVICE". The wire field name is
	// "type".
	PrincipalType string `json:"type"`
	// Tier is the tenancy tier: "ANCHOR", "PARTNER" or "CLIENT". Absent on
	// tokens minted by older platform versions, which carried the tier in
	// Scope; use [AccessTokenClaims.TierName] / IsAnchor rather than reading
	// it raw.
	Tier string `json:"tier,omitempty"`
	// Scope is the granted OAuth scope: a space-delimited list of permission
	// codes (see [AccessTokenClaims.Permissions]). It is NOT the tenancy
	// tier on current tokens (that is Tier). Older platform versions put the
	// tier name here; [AccessTokenClaims.TierName] still honours that when
	// Tier is absent. May be absent entirely.
	Scope string `json:"scope,omitempty"`
	// Email is present for USER principals, absent for SERVICE.
	Email string `json:"email,omitempty"`
	// Name is the display name.
	Name string `json:"name"`
	// Clients are the client entries this principal can access: "id:code"
	// pairs (or bare ids on older tokens), or ["*"] for anchor users. Use
	// [AccessTokenClaims.ClientIDs] / ClientCodes for the parsed halves.
	Clients []string `json:"clients"`
	// Roles are the role codes assigned to this principal.
	Roles []string `json:"roles,omitempty"`
	// Applications are the application codes derived from roles, e.g.
	// "operant" from "operant:admin". Always present on tokens issued
	// by FC but absent on older tokens.
	Applications []string `json:"applications,omitempty"`
	// TokenUse is the token class: "api" for a credential an application
	// backend may accept, absent on older tokens. [TokenValidator.Validate]
	// refuses every other value ("identity", "field", ...).
	TokenUse string `json:"token_use,omitempty"`
}

// splitClient splits a clients-claim entry into id and code. The first
// colon separates them, so a code containing a colon stays intact. A bare
// id (older tokens) yields an empty code.
func splitClient(entry string) (id, code string) {
	if i := strings.IndexByte(entry, ':'); i > 0 {
		return entry[:i], entry[i+1:]
	}
	return entry, ""
}

// ClientIDs returns the client ids from the clients claim, with any
// ":code" suffix removed. A "*" (all clients) entry is kept as "*".
func (c *AccessTokenClaims) ClientIDs() []string {
	out := make([]string, 0, len(c.Clients))
	for _, e := range c.Clients {
		if e == "" {
			continue
		}
		id, _ := splitClient(e)
		out = append(out, id)
	}
	return out
}

// ClientCodes returns the client codes from "id:code" entries of the
// clients claim. Bare ids and "*" contribute nothing.
func (c *AccessTokenClaims) ClientCodes() []string {
	var out []string
	for _, e := range c.Clients {
		if _, code := splitClient(e); code != "" {
			out = append(out, code)
		}
	}
	return out
}

// HasClientAccess reports whether the principal can access the given
// client id. It matches on the id half of "id:code" entries (bare ids on
// older tokens match too); a "*" entry grants every client.
func (c *AccessTokenClaims) HasClientAccess(clientID string) bool {
	for _, e := range c.Clients {
		if e == "*" {
			return true
		}
		if id, _ := splitClient(e); id != "" && id == clientID {
			return true
		}
	}
	return false
}

// Permissions returns the permission codes granted by the scope claim
// (space-delimited). Empty when the token carries no scope, or when scope
// is only a legacy tier name.
func (c *AccessTokenClaims) Permissions() []string {
	if isTierName(c.Scope) {
		return nil
	}
	return strings.Fields(c.Scope)
}

// HasPermission reports whether the scope claim grants the permission.
func (c *AccessTokenClaims) HasPermission(perm string) bool {
	return slices.Contains(c.Permissions(), perm)
}

func isTierName(s string) bool { return s == "ANCHOR" || s == "PARTNER" || s == "CLIENT" }

// TierName returns the tenancy tier ("ANCHOR", "PARTNER" or "CLIENT"). It
// reads Tier, and only when that is absent falls back to Scope if Scope is
// exactly a tier name (tokens from older platform versions). Empty when
// neither identifies a tier.
func (c *AccessTokenClaims) TierName() string {
	if c.Tier != "" {
		return c.Tier
	}
	if isTierName(c.Scope) {
		return c.Scope
	}
	return ""
}

// HasRole reports whether the principal has the given role code.
func (c *AccessTokenClaims) HasRole(role string) bool {
	return slices.Contains(c.Roles, role)
}

// IsAnchor reports whether this is an anchor (platform-wide) principal.
func (c *AccessTokenClaims) IsAnchor() bool { return c.TierName() == "ANCHOR" }

// IsPartner reports whether this is a partner-tier principal.
func (c *AccessTokenClaims) IsPartner() bool { return c.TierName() == "PARTNER" }

// IsClient reports whether this is a client-tier principal.
func (c *AccessTokenClaims) IsClient() bool { return c.TierName() == "CLIENT" }

// IsService reports whether this is a service account.
func (c *AccessTokenClaims) IsService() bool { return c.PrincipalType == "SERVICE" }

// PrincipalID returns the Sub claim.
func (c *AccessTokenClaims) PrincipalID() string { return c.Sub }

// AuthContext is a validated token plus the raw JWT, ready for
// authorization checks and forwarding to downstream services.
type AuthContext struct {
	Claims AccessTokenClaims
	Token  string
}

// NewAuthContext constructs an AuthContext.
func NewAuthContext(claims AccessTokenClaims, token string) *AuthContext {
	return &AuthContext{Claims: claims, Token: token}
}

func (a *AuthContext) PrincipalID() string { return a.Claims.Sub }
func (a *AuthContext) Email() string       { return a.Claims.Email }
func (a *AuthContext) Name() string        { return a.Claims.Name }
func (a *AuthContext) IsAnchor() bool      { return a.Claims.IsAnchor() }
func (a *AuthContext) IsService() bool     { return a.Claims.IsService() }
func (a *AuthContext) HasClientAccess(clientID string) bool {
	return a.Claims.HasClientAccess(clientID)
}
func (a *AuthContext) HasRole(role string) bool    { return a.Claims.HasRole(role) }
func (a *AuthContext) ClientIDs() []string         { return a.Claims.ClientIDs() }
func (a *AuthContext) ClientCodes() []string       { return a.Claims.ClientCodes() }
func (a *AuthContext) Permissions() []string       { return a.Claims.Permissions() }
func (a *AuthContext) HasPermission(p string) bool { return a.Claims.HasPermission(p) }
func (a *AuthContext) Roles() []string             { return a.Claims.Roles }

// BearerToken returns the raw JWT for forwarding to downstream services.
func (a *AuthContext) BearerToken() string { return a.Token }
