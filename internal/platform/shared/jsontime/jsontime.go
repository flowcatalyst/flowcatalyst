// Package jsontime provides a time wrapper with fixed-precision
// microsecond ISO-8601 JSON serialisation.
//
// API response DTOs use jsontime.Time wherever they would otherwise use
// time.Time so the wire format is stable: exactly 6 fractional digits
// every time, regardless of the underlying instant's precision. This
// matches Postgres's native TIMESTAMPTZ precision and gives parsers a
// predictable shape (vs Go's time.RFC3339Nano which trims trailing
// zeros, producing variable-length strings).
//
// Does NOT cover the HMAC signing path in internal/router/mediator.go,
// which deliberately uses millisecond precision to match the Rust
// signer + consumer SDKs. That format is wire-protocol crypto and lives
// outside this helper.
package jsontime

import (
	"fmt"
	"time"
)

// Time wraps time.Time. Marshal/Unmarshal preserve microsecond
// precision; the value is truncated to microseconds on every entry
// point so the wrapper is canonical.
type Time struct {
	t time.Time
}

// Layout is the fixed format used for marshalling: ISO-8601 with
// exactly six fractional digits and a timezone offset.
const Layout = "2006-01-02T15:04:05.000000Z07:00"

// New wraps t, truncating to microsecond precision.
func New(t time.Time) Time {
	return Time{t: t.Truncate(time.Microsecond)}
}

// Now returns the current UTC instant truncated to microseconds.
func Now() Time {
	return New(time.Now().UTC())
}

// Underlying returns the wrapped time.Time (already truncated).
func (jt Time) Underlying() time.Time { return jt.t }

// IsZero matches time.Time.IsZero.
func (jt Time) IsZero() bool { return jt.t.IsZero() }

// String returns the wire-format representation. Useful for logging.
func (jt Time) String() string { return jt.t.Format(Layout) }

// MarshalJSON emits the fixed-precision microsecond ISO-8601 string.
func (jt Time) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(Layout)+2)
	b = append(b, '"')
	b = jt.t.AppendFormat(b, Layout)
	b = append(b, '"')
	return b, nil
}

// UnmarshalJSON accepts any RFC-3339 string (millisecond, microsecond,
// nanosecond — Go's parser handles all three) and truncates to
// microseconds. `null` is treated as the zero time.
func (jt *Time) UnmarshalJSON(b []byte) error {
	s := string(b)
	if s == "null" {
		jt.t = time.Time{}
		return nil
	}
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return fmt.Errorf("jsontime: expected quoted string, got %q", s)
	}
	parsed, err := time.Parse(time.RFC3339Nano, s[1:len(s)-1])
	if err != nil {
		return fmt.Errorf("jsontime: parse %q: %w", s[1:len(s)-1], err)
	}
	jt.t = parsed.Truncate(time.Microsecond)
	return nil
}
