package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
)

// PkceChallenge is a one-shot verifier/challenge pair for the OAuth2
// authorization-code flow with PKCE (RFC 7636).
type PkceChallenge struct {
	// CodeVerifier is the secret. Store in session; send on token exchange.
	CodeVerifier string
	// CodeChallenge is sent in the authorize URL.
	CodeChallenge string
	// CodeChallengeMethod is always "S256".
	CodeChallengeMethod string
}

// NewPkceChallenge generates a fresh PKCE pair using S256.
func NewPkceChallenge() PkceChallenge {
	verifier := randomURLSafe(64)
	sum := sha256.Sum256([]byte(verifier))
	return PkceChallenge{
		CodeVerifier:        verifier,
		CodeChallenge:       base64.RawURLEncoding.EncodeToString(sum[:]),
		CodeChallengeMethod: "S256",
	}
}

// randomURLSafe returns a URL-safe base64 string from n random bytes.
func randomURLSafe(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
