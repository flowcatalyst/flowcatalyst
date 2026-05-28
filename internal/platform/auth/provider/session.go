package provider

import (
	"github.com/mohae/deepcopy"
	"github.com/ory/fosite"
	"github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/token/jwt"
)

// FCSession is FlowCatalyst's fosite session. It embeds JWTSession so
// access tokens are minted as JWTs with our extra claims (scope_label,
// clients, roles, applications, email) under JWTClaims.Extra.
//
// Whenever fosite needs to serialize/deserialize a session (refresh
// flow, introspection), it round-trips this struct through gob/json.
// Keep field types JSON-friendly.
type FCSession struct {
	*oauth2.JWTSession
}

// NewSession constructs an empty FCSession ready to be populated by a
// grant handler. AccessRequest does this by reading the request body.
func NewSession() *FCSession {
	return &FCSession{
		JWTSession: &oauth2.JWTSession{
			JWTClaims: &jwt.JWTClaims{Extra: map[string]interface{}{}},
			JWTHeader: &jwt.Headers{Extra: map[string]interface{}{}},
		},
	}
}

// Clone overrides JWTSession.Clone so the returned value is *FCSession.
// fosite calls Clone before passing sessions around; without this
// override the strategy receives an *oauth2.JWTSession and loses our
// wrapper (which today is mostly a documentation hook but will hold the
// owning-principal-id once we plumb refresh-token-family checks).
func (s *FCSession) Clone() fosite.Session {
	if s == nil {
		return nil
	}
	return deepcopy.Copy(s).(fosite.Session)
}

// applyClaims copies the values built by BuildClaims onto the JWT
// session so the signed access token carries them as top-level claims.
//
// Note on key placement: fosite reserves the "scope" / "scp" / "iss" /
// "sub" / "aud" / "iat" / "exp" claim names — values stored in Extra
// under those keys get overwritten by ToMap (it reconstructs them from
// JWTClaims.Issuer / Subject / Scope / etc.). Everything that isn't a
// JWT-standard claim goes into Extra.
func (s *FCSession) applyClaims(c *Claims) {
	if c == nil {
		return
	}
	jc := s.GetJWTClaims().(*jwt.JWTClaims)
	if c.Issuer != "" {
		jc.Issuer = c.Issuer
	}
	if c.Subject != "" {
		jc.Subject = c.Subject
		s.SetSubject(c.Subject)
	}
	if c.Audience != "" {
		jc.Audience = []string{c.Audience}
	}
	if c.Scope != "" {
		jc.Scope = []string{c.Scope}
	}
	if jc.Extra == nil {
		jc.Extra = map[string]interface{}{}
	}
	if len(c.Clients) > 0 {
		jc.Extra["clients"] = c.Clients
	}
	if len(c.Roles) > 0 {
		jc.Extra["roles"] = c.Roles
	}
	if len(c.Applications) > 0 {
		jc.Extra["applications"] = c.Applications
	}
	if len(c.Permissions) > 0 {
		jc.Extra["permissions"] = c.Permissions
	}
	if c.Email != "" {
		jc.Extra["email"] = c.Email
	}
	if c.Name != "" {
		jc.Extra["name"] = c.Name
	}
	// OIDC ID-token claims. Populated only when the caller hydrated the
	// ID-token side of the request (authorize flow with openid scope).
	if c.Nonce != "" {
		jc.Extra["nonce"] = c.Nonce
	}
	if c.AuthorizedParty != "" {
		jc.Extra["azp"] = c.AuthorizedParty
	}
	if c.AuthTime > 0 {
		jc.Extra["auth_time"] = c.AuthTime
	}
	if c.EmailVerified != nil {
		jc.Extra["email_verified"] = *c.EmailVerified
	}
}
