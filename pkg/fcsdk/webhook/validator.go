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

// Header names accepted by the Validator. HTTP header lookups are
// case-insensitive, so these match the router's uppercase X-FLOWCATALYST-*
// headers in any compliant framework.
//
// Both this Validator and [Verifier] expect the timestamp format the
// FlowCatalyst router actually emits: ISO8601 with millisecond precision
// (e.g. 2026-05-24T08:30:00.123Z). A bare Unix-seconds integer is also
// accepted for backward compatibility. The two types differ only in API
// style; prefer [Verifier] for new code.
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
	ErrValidatorInvalidTimestamp = errors.New("webhook: invalid timestamp")
	ErrValidatorInvalidSignature = errors.New("webhook: invalid signature")
	ErrTimestampExpired          = errors.New("webhook: timestamp expired")
	ErrTimestampInFuture         = errors.New("webhook: timestamp is in the future")
	ErrMissingSecret             = errors.New("webhook: signing secret not configured")
)

// Validator validates HMAC-SHA256 webhook signatures. The signed message is
// timestamp || payload (hex-encoded). The timestamp is the ISO8601
// millisecond value emitted by the FlowCatalyst router (a bare Unix-seconds
// integer is also accepted for backward compatibility).
//
// Validator and [Verifier] are functionally equivalent; they differ only in
// API style. Prefer [Verifier] for new code.
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
//   - timestamp: value of X-FlowCatalyst-Timestamp header (ISO8601 ms, e.g.
//     2026-05-24T08:30:00.123Z; a bare Unix-seconds integer is also accepted)
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
	tsSecs, err := parseTimestamp(timestamp)
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

// parseTimestamp accepts the X-FlowCatalyst-Timestamp value. The FlowCatalyst
// router emits ISO8601 with millisecond precision (e.g.
// 2026-05-24T08:30:00.123Z); we also accept any RFC3339 fractional precision
// and, for backward compatibility, a bare Unix-seconds integer. Returns Unix
// seconds.
func parseTimestamp(s string) (int64, error) {
	if t, err := time.Parse("2006-01-02T15:04:05.000Z", s); err == nil {
		return t.UTC().Unix(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Unix(), nil
	}
	if secs, err := strconv.ParseInt(s, 10, 64); err == nil {
		return secs, nil
	}
	return 0, errors.New("webhook: unparseable timestamp")
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
