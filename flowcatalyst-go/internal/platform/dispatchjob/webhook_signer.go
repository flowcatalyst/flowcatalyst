package dispatchjob

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"time"
)

const (
	// SignatureHeader is the HTTP header name for the webhook signature
	SignatureHeader = "X-FLOWCATALYST-SIGNATURE"

	// TimestampHeader is the HTTP header name for the webhook timestamp
	TimestampHeader = "X-FLOWCATALYST-TIMESTAMP"
)

// SignedWebhookRequest contains all the data needed to send a signed webhook request
type SignedWebhookRequest struct {
	Payload     string
	Signature   string
	Timestamp   string
	BearerToken string
}

// WebhookSigner generates HMAC-SHA256 signatures for outbound webhook requests.
//
// The signature is generated using the timestamp concatenated with the payload,
// then signed with the signing secret. The receiver can verify by reproducing this signature.
//
// This matches Java's tech.flowcatalyst.dispatchjob.security.WebhookSigner
type WebhookSigner struct{}

// NewWebhookSigner creates a new webhook signer
func NewWebhookSigner() *WebhookSigner {
	return &WebhookSigner{}
}

// Sign signs a webhook payload with the provided credentials.
//
// The signature is computed as: HMAC-SHA256(timestamp + payload, signingSecret)
//
// Parameters:
//   - payload: The request body to sign
//   - authToken: The bearer token for Authorization header
//   - signingSecret: The secret key for HMAC-SHA256 signing
//
// Returns a SignedWebhookRequest with signature, timestamp, and bearer token
func (s *WebhookSigner) Sign(payload, authToken, signingSecret string) *SignedWebhookRequest {
	// Generate ISO8601 timestamp with millisecond precision
	timestamp := time.Now().UTC().Truncate(time.Millisecond).Format(time.RFC3339Nano)

	// Create signature payload: timestamp + body
	signaturePayload := timestamp + payload

	// Generate HMAC SHA-256 signature
	signature := s.hmacSHA256Hex(signaturePayload, signingSecret)

	return &SignedWebhookRequest{
		Payload:     payload,
		Signature:   signature,
		Timestamp:   timestamp,
		BearerToken: authToken,
	}
}

// Verify verifies a webhook signature.
//
// Parameters:
//   - payload: The request body that was signed
//   - timestamp: The timestamp from the TimestampHeader
//   - signature: The signature from the SignatureHeader
//   - signingSecret: The secret key used for signing
//
// Returns true if the signature is valid
func (s *WebhookSigner) Verify(payload, timestamp, signature, signingSecret string) bool {
	// Recreate the signature payload
	signaturePayload := timestamp + payload

	// Compute expected signature
	expected := s.hmacSHA256Hex(signaturePayload, signingSecret)

	// Use constant-time comparison to prevent timing attacks
	return hmac.Equal([]byte(expected), []byte(signature))
}

// hmacSHA256Hex computes HMAC-SHA256 and returns hex-encoded result (lowercase)
func (s *WebhookSigner) hmacSHA256Hex(data, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(data))
	hash := mac.Sum(nil)
	return hex.EncodeToString(hash)
}
