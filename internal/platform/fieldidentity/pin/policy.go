// Package pin is the field-worker PIN policy and hashing (docs/field-worker-
// identity-p1-spec.md section 3.2). It is pure: no database, no clock.
//
// The policy decides which PINs are acceptable; the hasher stores them as
// argon2id over an HMAC of (client, identity, pin) keyed by a pepper derived
// from FLOWCATALYST_APP_KEY, so a database leak alone cannot be brute-forced
// offline (a 6-digit PIN has only a million candidates; the pepper, not the
// hash cost, is what protects it).
package pin

import (
	"crypto/rand"
	"math/big"
	"strings"
)

const (
	// DefaultMinLength is the minimum PIN length unless a client's policy row
	// lowers it to MinAllowedLength.
	DefaultMinLength = 6
	// MinAllowedLength is the shortest PIN a client policy may permit.
	MinAllowedLength = 4
	// MaxLength is the longest PIN accepted.
	MaxLength = 12
)

// Violation codes returned by [Check].
const (
	TooShort     = "PIN_TOO_SHORT"
	TooLong      = "PIN_TOO_LONG"
	NotDigits    = "PIN_NOT_DIGITS"
	AllSame      = "PIN_ALL_SAME"
	Sequential   = "PIN_SEQUENTIAL"
	Common       = "PIN_COMMON"
	IsWorkerCode = "PIN_IS_WORKER_CODE"
)

// common is the short top-N list of guessable PINs that are neither all-same
// nor a straight run.
var common = map[string]bool{
	"121212": true, "112233": true, "696969": true, "123123": true,
	"159753": true, "147258": true, "123321": true, "111222": true,
	"654654": true, "101010": true, "131313": true, "223344": true,
	"1212": true, "1122": true, "6969": true, "2580": true,
	"1004": true, "2000": true, "1313": true, "1010": true,
}

// Check returns the policy violations of pin (empty = acceptable). minLength
// is the client's configured minimum (values outside
// [MinAllowedLength, MaxLength] fall back to the default). workerCode is the
// worker's own code, which may not be used as the PIN.
func Check(pin, workerCode string, minLength int) []string {
	if minLength < MinAllowedLength || minLength > MaxLength {
		minLength = DefaultMinLength
	}
	var v []string
	if pin == "" || !allDigits(pin) {
		return append(v, NotDigits)
	}
	if len(pin) < minLength {
		v = append(v, TooShort)
	}
	if len(pin) > MaxLength {
		v = append(v, TooLong)
	}
	if allSame(pin) {
		v = append(v, AllSame)
	}
	if isRun(pin) {
		v = append(v, Sequential)
	}
	if common[pin] {
		v = append(v, Common)
	}
	if workerCode != "" && strings.EqualFold(pin, strings.TrimSpace(workerCode)) {
		v = append(v, IsWorkerCode)
	}
	return v
}

// ValidMinLength reports whether n is an acceptable pin_min_length setting.
func ValidMinLength(n int) bool { return n >= MinAllowedLength && n <= MaxLength }

// Generate returns a random PIN of length n that passes [Check]. n outside the
// allowed range uses the default.
func Generate(n int, workerCode string) (string, error) {
	if !ValidMinLength(n) {
		n = DefaultMinLength
	}
	ten := big.NewInt(10)
	for {
		b := make([]byte, n)
		for i := range b {
			d, err := rand.Int(rand.Reader, ten)
			if err != nil {
				return "", err
			}
			b[i] = byte('0' + d.Int64())
		}
		if s := string(b); len(Check(s, workerCode, n)) == 0 {
			return s, nil
		}
	}
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func allSame(s string) bool {
	for i := 1; i < len(s); i++ {
		if s[i] != s[0] {
			return false
		}
	}
	return len(s) > 0
}

// isRun reports an ascending or descending run of digits, wrapping 9 to 0
// (123456, 654321, 890123, 109876).
func isRun(s string) bool {
	if len(s) < 2 {
		return false
	}
	asc, desc := true, true
	for i := 1; i < len(s); i++ {
		d := int(s[i]) - int(s[i-1])
		if d != 1 && d != -9 {
			asc = false
		}
		if d != -1 && d != 9 {
			desc = false
		}
	}
	return asc || desc
}
