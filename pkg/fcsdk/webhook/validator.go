package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

// Headers and timestamp format used by the FlowCatalyst (Rust) router and
// other peer SDKs. Note: this *parallel* shape differs from the
// [Verifier] form above — the legacy Verifier matches this Go platform's
// own router (uppercase headers, ISO8601 timestamps). Validator matches
// the Rust SDK shape (mixed-case headers, Unix-second timestamps).
const (
	ValidatorSignatureHeader = "X-FlowCatalyst-Signature"
	ValidatorTimestampHeader = "X-FlowCatalyst-Timestamp"
)

// DefaultToleranceSecs is how stale a timestamp may be before rejection.
const DefaultToleranceSecs = 300

// FutureGraceSecs caps how far into the future a timestamp may be (clock
// skew between platform and consumer).
const FutureGraceSecs = 60

// Sentinel validation errors used by [Validator]. Use errors.Is to branch.
var (
	ErrValidatorMissingSignature = errors.New("webhook: missing signature header (" + ValidatorSignatureHeader + ")")
	ErrValidatorMissingTimestamp = errors.New("webhook: missing timestamp header (" + ValidatorTimestampHeader + ")")
	ErrValidatorInvalidTimestamp = errors.New("webhook: invalid timestamp: not an integer")
	ErrValidatorInvalidSignature = errors.New("webhook: invalid signature")
	ErrTimestampExpired          = errors.New("webhook: timestamp expired")
	ErrTimestampInFuture         = errors.New("webhook: timestamp is in the future")
	ErrMissingSecret             = errors.New("webhook: signing secret not configured")
)

// Validator validates HMAC-SHA256 webhook signatures using the Rust-SDK
// wire format: timestamp as Unix seconds, signed message = timestamp ||
// payload, signature hex-encoded.
//
// Use this when the sender is the Rust platform or any peer SDK. For
// webhooks signed by THIS Go platform's router (uppercase headers,
// ISO8601 timestamps), use [Verifier] instead.
type Validator struct {
	secret    []byte
	tolerance time.Duration
	now       func() time.Time
}

// ValidatorOption configures a Validator.
type ValidatorOption func(*Validator)

// WithTolerance sets the timestamp freshness window. Default 300s.
func WithTolerance(d time.Duration) ValidatorOption {
	return func(v *Validator) { v.tolerance = d }
}

// WithClock overrides the time source. Tests use this to inject a fixed
// clock.
func WithClock(now func() time.Time) ValidatorOption {
	return func(v *Validator) { v.now = now }
}

// NewValidator builds a Validator with the given signing secret.
func NewValidator(secret string, opts ...ValidatorOption) *Validator {
	v := &Validator{
		secret:    []byte(secret),
		tolerance: DefaultToleranceSecs * time.Second,
		now:       time.Now,
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// ValidatorFromEnv builds a Validator from FLOWCATALYST_SIGNING_SECRET.
// Returns ErrMissingSecret if the env var is unset or empty.
func ValidatorFromEnv(opts ...ValidatorOption) (*Validator, error) {
	s := os.Getenv("FLOWCATALYST_SIGNING_SECRET")
	if s == "" {
		return nil, ErrMissingSecret
	}
	return NewValidator(s, opts...), nil
}

// Validate checks the signature against the body and timestamp.
//
//   - signature: value of X-FlowCatalyst-Signature header (hex HMAC-SHA256)
//   - timestamp: value of X-FlowCatalyst-Timestamp header (Unix seconds)
//   - payload: raw request body
//
// Returns nil on success, or one of the sentinel errors. The signature
// comparison is constant-time.
func (v *Validator) Validate(signature, timestamp string, payload []byte) error {
	if signature == "" {
		return ErrValidatorMissingSignature
	}
	if timestamp == "" {
		return ErrValidatorMissingTimestamp
	}
	tsSecs, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return ErrValidatorInvalidTimestamp
	}
	if err := v.validateTimestamp(tsSecs); err != nil {
		return err
	}

	expected := v.ComputeSignature(timestamp, payload)
	if !hmac.Equal([]byte(signature), []byte(expected)) {
		return ErrValidatorInvalidSignature
	}
	return nil
}

// ComputeSignature renders the expected hex-encoded HMAC-SHA256 for a
// (timestamp, payload) pair. Exposed so consumers can sign outbound
// callbacks with the same algorithm.
func (v *Validator) ComputeSignature(timestamp string, payload []byte) string {
	mac := hmac.New(sha256.New, v.secret)
	mac.Write([]byte(timestamp))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

func (v *Validator) validateTimestamp(tsSecs int64) error {
	now := v.now().UTC().Unix()
	lower := now - int64(v.tolerance.Seconds())
	if tsSecs < lower {
		return fmt.Errorf("%w (tolerance: %s)", ErrTimestampExpired, v.tolerance)
	}
	if tsSecs > now+FutureGraceSecs {
		return ErrTimestampInFuture
	}
	return nil
}
