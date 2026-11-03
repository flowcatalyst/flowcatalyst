package fieldauth

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/fieldidentity"
)

// TokenVerifier verifies field access tokens against the platform's own signing
// keys. It implements fieldidentity.TokenVerifier for the introspection
// endpoint: signature, issuer, expiry, JOSE typ fc-field+jwt and token_use
// "field". Whether the session is still live is the introspection check's
// job (fieldidentity.Ops.TokenActive).
type TokenVerifier struct{ Auth *authservice.AuthService }

var _ fieldidentity.TokenVerifier = TokenVerifier{}

// VerifyFieldAccessToken implements fieldidentity.TokenVerifier.
func (v TokenVerifier) VerifyFieldAccessToken(_ context.Context, token string) (*fieldidentity.TokenClaims, error) {
	claims, err := v.Auth.ValidateFieldToken(token)
	if err != nil {
		return nil, err
	}
	return tokenClaims(claims), nil
}
