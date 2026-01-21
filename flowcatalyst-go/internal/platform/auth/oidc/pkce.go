package oidc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"io"
)

var (
	ErrInvalidCodeVerifier  = errors.New("invalid code verifier")
	ErrInvalidCodeChallenge = errors.New("invalid code challenge")
	ErrPKCEMismatch         = errors.New("PKCE verification failed")
)

const (
	// CodeChallengeMethodS256 is the SHA-256 challenge method
	CodeChallengeMethodS256 = "S256"

	// CodeChallengeMethodPlain is the plain challenge method (not recommended)
	CodeChallengeMethodPlain = "plain"

	// CodeVerifierLength is the length of the code verifier in bytes (before encoding)
	CodeVerifierLength = 48 // Results in 64 base64url characters
)

// PKCEService handles PKCE operations
type PKCEService struct {
	required bool
}

// NewPKCEService creates a new PKCE service
func NewPKCEService(required bool) *PKCEService {
	return &PKCEService{
		required: required,
	}
}

// IsRequired returns true if PKCE is required
func (s *PKCEService) IsRequired() bool {
	return s.required
}

// GenerateCodeVerifier generates a random code verifier
func (s *PKCEService) GenerateCodeVerifier() (string, error) {
	bytes := make([]byte, CodeVerifierLength)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// GenerateCodeChallenge generates a code challenge from a verifier using S256
func (s *PKCEService) GenerateCodeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

// VerifyCodeChallenge verifies a code verifier against a code challenge
func (s *PKCEService) VerifyCodeChallenge(verifier, challenge, method string) error {
	if verifier == "" {
		return ErrInvalidCodeVerifier
	}
	if challenge == "" {
		return ErrInvalidCodeChallenge
	}

	var expectedChallenge string

	switch method {
	case CodeChallengeMethodS256, "": // Default to S256
		expectedChallenge = s.GenerateCodeChallenge(verifier)
	case CodeChallengeMethodPlain:
		expectedChallenge = verifier
	default:
		return ErrInvalidCodeChallenge
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expectedChallenge), []byte(challenge)) != 1 {
		return ErrPKCEMismatch
	}

	return nil
}

// ValidateCodeVerifier validates the format of a code verifier
func (s *PKCEService) ValidateCodeVerifier(verifier string) error {
	// Code verifier must be between 43 and 128 characters
	if len(verifier) < 43 || len(verifier) > 128 {
		return ErrInvalidCodeVerifier
	}

	// Code verifier must only contain unreserved URI characters
	for _, c := range verifier {
		if !isUnreservedChar(byte(c)) {
			return ErrInvalidCodeVerifier
		}
	}

	return nil
}

// ValidateCodeChallenge validates the format of a code challenge
func (s *PKCEService) ValidateCodeChallenge(challenge string) error {
	// Code challenge must be 43 characters for S256
	if len(challenge) < 43 || len(challenge) > 128 {
		return ErrInvalidCodeChallenge
	}

	// Validate base64url encoding
	for _, c := range challenge {
		if !isBase64URLChar(byte(c)) {
			return ErrInvalidCodeChallenge
		}
	}

	return nil
}

// isUnreservedChar checks if a character is an unreserved URI character
func isUnreservedChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '.' || c == '_' || c == '~'
}

// isBase64URLChar checks if a character is valid in base64url encoding
func isBase64URLChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') ||
		(c >= 'a' && c <= 'z') ||
		(c >= '0' && c <= '9') ||
		c == '-' || c == '_'
}

// GeneratePKCEPair generates a new verifier and challenge pair
func (s *PKCEService) GeneratePKCEPair() (verifier, challenge string, err error) {
	verifier, err = s.GenerateCodeVerifier()
	if err != nil {
		return "", "", err
	}
	challenge = s.GenerateCodeChallenge(verifier)
	return verifier, challenge, nil
}
