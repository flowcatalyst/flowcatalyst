package dispatchjob

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"log/slog"
)

var (
	// ErrAppKeyNotConfigured indicates the app key is not set
	ErrAppKeyNotConfigured = errors.New("app key is not configured")

	// ErrInvalidToken indicates the token validation failed
	ErrInvalidToken = errors.New("invalid auth token")
)

// DispatchAuthService generates and validates HMAC-SHA256 auth tokens for dispatch job processing.
//
// This implements the authentication flow between the platform and message router:
//  1. Platform creates a dispatch job and generates an HMAC token using the app key
//  2. Platform sends the job to SQS with the token in the MessagePointer
//  3. Message router receives the message and calls back to platform with the same token
//  4. Platform validates the token by re-computing the HMAC and comparing
//
// The token is computed as: HMAC-SHA256(dispatchJobId, appKey)
//
// This matches Java's tech.flowcatalyst.dispatchjob.security.DispatchAuthService
type DispatchAuthService struct {
	appKey string
	logger *slog.Logger
}

// NewDispatchAuthService creates a new dispatch auth service
func NewDispatchAuthService(appKey string, logger *slog.Logger) *DispatchAuthService {
	if logger == nil {
		logger = slog.Default()
	}
	return &DispatchAuthService{
		appKey: appKey,
		logger: logger,
	}
}

// GenerateAuthToken generates an HMAC-SHA256 auth token for a dispatch job ID.
// Returns the hex-encoded HMAC-SHA256 token.
func (s *DispatchAuthService) GenerateAuthToken(dispatchJobID string) (string, error) {
	if s.appKey == "" {
		return "", ErrAppKeyNotConfigured
	}

	return s.hmacSHA256Hex(dispatchJobID, s.appKey), nil
}

// ValidateAuthToken validates an auth token from the message router.
// Returns nil if valid, ErrInvalidToken if invalid.
func (s *DispatchAuthService) ValidateAuthToken(dispatchJobID, token string) error {
	if token == "" || dispatchJobID == "" {
		return ErrInvalidToken
	}

	if s.appKey == "" {
		s.logger.Error("app key is not configured, cannot validate auth token")
		return ErrAppKeyNotConfigured
	}

	expected, err := s.GenerateAuthToken(dispatchJobID)
	if err != nil {
		return err
	}

	// Use constant-time comparison to prevent timing attacks
	if subtle.ConstantTimeCompare([]byte(expected), []byte(token)) != 1 {
		return ErrInvalidToken
	}

	return nil
}

// IsConfigured returns true if the app key is configured
func (s *DispatchAuthService) IsConfigured() bool {
	return s.appKey != ""
}

// hmacSHA256Hex computes HMAC-SHA256 and returns hex-encoded result (lowercase)
func (s *DispatchAuthService) hmacSHA256Hex(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	hash := mac.Sum(nil)
	return hex.EncodeToString(hash)
}
