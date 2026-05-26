package auth

import (
	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// HmacTokenValidator validates HS256-signed JWTs using a shared secret.
// Use for development or when your app shares a signing secret with
// FlowCatalyst. For production OIDC integration, prefer TokenValidator
// (RS256 via JWKS).
type HmacTokenValidator struct {
	secret   []byte
	issuer   string
	audience string
}

// NewHmacTokenValidator builds an HS256 validator.
func NewHmacTokenValidator(secret, issuer, audience string) *HmacTokenValidator {
	return &HmacTokenValidator{
		secret:   []byte(secret),
		issuer:   issuer,
		audience: audience,
	}
}

// Validate verifies the HS256 signature, issuer, audience, and exp/nbf.
func (v *HmacTokenValidator) Validate(token string) (*AuthContext, error) {
	_, err := jwt.ParseString(
		token,
		jwt.WithKey(jwa.HS256, v.secret),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
	)
	if err != nil {
		return nil, mapJWXError(err)
	}
	claims, derr := decodeAccessTokenClaims(token)
	if derr != nil {
		return nil, derr
	}
	return NewAuthContext(claims, token), nil
}

// ValidateBearer strips the "Bearer " prefix and calls Validate.
func (v *HmacTokenValidator) ValidateBearer(authHeader string) (*AuthContext, error) {
	token, err := stripBearer(authHeader)
	if err != nil {
		return nil, err
	}
	return v.Validate(token)
}
