package dispatchjob

import (
	"strings"
	"testing"
	"time"
)

func TestWebhookSigner_Sign(t *testing.T) {
	signer := NewWebhookSigner()

	payload := `{"event":"test","data":{"id":"123"}}`
	authToken := "test-bearer-token"
	signingSecret := "my-secret-key"

	result := signer.Sign(payload, authToken, signingSecret)

	// Verify all fields are set
	if result.Payload != payload {
		t.Errorf("expected payload %q, got %q", payload, result.Payload)
	}
	if result.BearerToken != authToken {
		t.Errorf("expected bearer token %q, got %q", authToken, result.BearerToken)
	}
	if result.Timestamp == "" {
		t.Error("expected timestamp to be set")
	}
	if result.Signature == "" {
		t.Error("expected signature to be set")
	}

	// Verify timestamp is valid ISO8601
	_, err := time.Parse(time.RFC3339Nano, result.Timestamp)
	if err != nil {
		t.Errorf("expected valid ISO8601 timestamp, got %q: %v", result.Timestamp, err)
	}

	// Verify signature is hex-encoded (lowercase)
	if strings.ToLower(result.Signature) != result.Signature {
		t.Error("expected signature to be lowercase hex")
	}
	if len(result.Signature) != 64 { // SHA256 produces 32 bytes = 64 hex chars
		t.Errorf("expected 64-char hex signature, got %d chars", len(result.Signature))
	}
}

func TestWebhookSigner_Verify(t *testing.T) {
	signer := NewWebhookSigner()

	payload := `{"event":"test"}`
	signingSecret := "my-secret-key"

	// Sign the payload
	signed := signer.Sign(payload, "token", signingSecret)

	// Verify should return true for valid signature
	if !signer.Verify(payload, signed.Timestamp, signed.Signature, signingSecret) {
		t.Error("expected valid signature to verify")
	}

	// Verify should return false for wrong secret
	if signer.Verify(payload, signed.Timestamp, signed.Signature, "wrong-secret") {
		t.Error("expected verification to fail with wrong secret")
	}

	// Verify should return false for tampered payload
	if signer.Verify("tampered", signed.Timestamp, signed.Signature, signingSecret) {
		t.Error("expected verification to fail with tampered payload")
	}

	// Verify should return false for tampered timestamp
	if signer.Verify(payload, "2024-01-01T00:00:00.000Z", signed.Signature, signingSecret) {
		t.Error("expected verification to fail with tampered timestamp")
	}

	// Verify should return false for tampered signature
	if signer.Verify(payload, signed.Timestamp, "invalidsignature", signingSecret) {
		t.Error("expected verification to fail with tampered signature")
	}
}

func TestWebhookSigner_DeterministicSignature(t *testing.T) {
	signer := NewWebhookSigner()

	payload := `{"test":"data"}`
	timestamp := "2024-01-15T10:30:00.123Z"
	signingSecret := "test-secret"

	// Manually compute expected signature for timestamp + payload
	signaturePayload := timestamp + payload
	expected := signer.hmacSHA256Hex(signaturePayload, signingSecret)

	// Verify should work with the computed signature
	if !signer.Verify(payload, timestamp, expected, signingSecret) {
		t.Error("expected deterministic signature to verify")
	}
}

func TestSignatureHeader_Constants(t *testing.T) {
	// Verify header constants match Java implementation
	if SignatureHeader != "X-FLOWCATALYST-SIGNATURE" {
		t.Errorf("expected SignatureHeader %q, got %q", "X-FLOWCATALYST-SIGNATURE", SignatureHeader)
	}
	if TimestampHeader != "X-FLOWCATALYST-TIMESTAMP" {
		t.Errorf("expected TimestampHeader %q, got %q", "X-FLOWCATALYST-TIMESTAMP", TimestampHeader)
	}
}
