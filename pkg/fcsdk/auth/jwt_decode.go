package auth

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"

	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/flowcatalyst/go-sdk/internal/compactjws"
)

// decodeAccessTokenClaims pulls the payload out of a JWT string and
// unmarshals it as AccessTokenClaims. The token MUST already be verified
// and validated by jwx — this function does no security work.
func decodeAccessTokenClaims(token string) (AccessTokenClaims, error) {
	var claims AccessTokenClaims
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return claims, newErr(KindInvalidToken, "JWT does not have three segments")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return claims, newErr(KindInvalidToken, "decode payload: "+err.Error())
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return claims, newErr(KindInvalidToken, "parse claims: "+err.Error())
	}
	return claims, nil
}

// checkSpelling refuses a token that is not three segments of canonical
// base64url: the spelling the platform issues, and the only one it accepts
// (docs/wire-contract.md, Authentication, "Token spelling").
func checkSpelling(token string) *Error {
	if err := compactjws.Check(token); err != nil {
		return newErr(KindInvalidToken, "token is not spelled as issued: "+err.Error())
	}
	return nil
}

// mapJWXError translates a jwx parse/validate/verify failure into our
// Error taxonomy. We surface TokenExpired separately so callers can
// distinguish refresh-needed from outright-bad-token.
func mapJWXError(err error) *Error {
	if err == nil {
		return nil
	}
	if errors.Is(err, jwt.TokenExpiredError{}) {
		return ErrTokenExpired
	}
	return newErr(KindInvalidToken, err.Error())
}

// stripBearer removes a leading "Bearer " from an Authorization header
// value, returning the bare token. Returns InvalidToken if the prefix
// is missing.
func stripBearer(authHeader string) (string, *Error) {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return "", newErr(KindInvalidToken, "missing 'Bearer ' prefix")
	}
	return authHeader[len(prefix):], nil
}
