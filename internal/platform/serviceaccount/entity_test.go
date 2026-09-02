package serviceaccount

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseAuthType_KnownValuesRoundTrip pins the happy path: every declared
// WebhookAuthType constant parses back to itself with ok=true.
func TestParseAuthType_KnownValuesRoundTrip(t *testing.T) {
	for _, want := range []WebhookAuthType{AuthNone, AuthBearer, AuthBasic, AuthAPIKey, AuthHMAC} {
		got, ok := ParseAuthType(string(want))
		assert.True(t, ok, "%s should parse", want)
		assert.Equal(t, want, got)
	}
}

// TestParseAuthType_EmptyStringMeansNone pins the pre-existing "no authType
// given" behaviour: it is NOT an unknown value, it means NONE — matching
// NoCredentials(). Only genuinely unrecognised strings must reject.
func TestParseAuthType_EmptyStringMeansNone(t *testing.T) {
	got, ok := ParseAuthType("")
	assert.True(t, ok)
	assert.Equal(t, AuthNone, got)
}

// TestParseAuthType_UnknownValueRejectsLoudly is the X-06 fix: a typo or
// corrupted value must never silently become NONE (which would ship an
// unauthenticated webhook). It must report ok=false so callers can reject.
func TestParseAuthType_UnknownValueRejectsLoudly(t *testing.T) {
	for _, bad := range []string{"NON", "bearer_token", "BEARERTOKEN", "garbage", " NONE", "none"} {
		got, ok := ParseAuthType(bad)
		assert.False(t, ok, "%q must not parse", bad)
		assert.Equal(t, WebhookAuthType(""), got)
	}
}
