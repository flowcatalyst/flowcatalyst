package webhook_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/webhook"
)

// TestVerifyMatchesRouterSigner pins the canonical signing formula:
//
//	HMAC-SHA256(secret, timestamp_bytes || body_bytes) → hex
//
// If this fails, the SDK and the router are out of sync — and existing
// webhook subscribers will stop verifying signatures correctly after
// cutover. See docs/api-parity.md §HMAC for the test vector.
func TestVerifyMatchesRouterSigner(t *testing.T) {
	secret := "test-secret-do-not-use-in-prod"
	body := []byte(`{"messageId":"msg_TEST123456"}`)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	v := webhook.NewVerifier(secret)
	require.NoError(t, v.Verify(body, sig, timestamp))
}

func TestVerifyRejectsTamperedBody(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"messageId":"msg_TEST123456"}`)
	timestamp := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(timestamp))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	tampered := []byte(`{"messageId":"msg_DIFFERENT"}`)
	err := webhook.NewVerifier(secret).Verify(tampered, sig, timestamp)
	require.Error(t, err)
	assert.ErrorIs(t, err, webhook.ErrBadSignature)
}

func TestVerifyRejectsStaleTimestamp(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{}`)
	old := time.Now().UTC().Add(-30 * time.Minute).Format("2006-01-02T15:04:05.000Z")

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(old))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	v := webhook.NewVerifier(secret)
	v.MaxClockSkew = 5 * time.Minute
	err := v.Verify(body, sig, old)
	require.Error(t, err)
	assert.ErrorIs(t, err, webhook.ErrStaleTimestamp)
}

func TestVerifyMissingHeaders(t *testing.T) {
	v := webhook.NewVerifier("s")
	assert.ErrorIs(t, v.Verify(nil, "", "2026-01-01T00:00:00.000Z"), webhook.ErrMissingSignature)
	assert.ErrorIs(t, v.Verify(nil, "abc", ""), webhook.ErrMissingTimestamp)
}
