package common

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ParseRetryAfter reads an HTTP Retry-After value in either form RFC 9110
// allows: delay-seconds ("120") or an HTTP-date ("Wed, 21 Oct 2026 07:28:00
// GMT", measured from now). A date already past, or a negative number, is a
// zero wait. ok=false when the value is empty or neither form parses — the
// caller applies its own default.
func ParseRetryAfter(v string, now time.Time) (time.Duration, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, false
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		if n <= 0 {
			return 0, true
		}
		// Bound the conversion: a year is far beyond any broker's delay.
		return time.Duration(min(n, int64(365*24*3600))) * time.Second, true
	}
	if t, err := http.ParseTime(v); err == nil {
		return max(t.Sub(now), 0), true
	}
	return 0, false
}
