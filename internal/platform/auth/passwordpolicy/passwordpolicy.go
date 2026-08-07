// Package passwordpolicy is the single password-acceptance policy for
// internal (password-auth) users, applied everywhere the platform sets a
// password on a USER principal: admin user create, the self-service
// change-password flow, and the password-reset confirm flow.
//
// It deliberately follows NIST SP 800-63B's shape — length + a
// common-password blocklist + "not your own identity" — and imposes NO
// composition rules (mandatory symbols/digits mostly produce "Password1!").
//
// The SDK-facing relaxed reset path (EnforcePasswordComplexity=false) does
// NOT run this policy — external applications own their own password rules by
// contract (owner decision).
package passwordpolicy

import (
	"bufio"
	"bytes"
	_ "embed"
	"fmt"
	"strings"
	"sync"
)

// MinLength / MaxLength bound accepted passwords. The floor matches the
// pre-existing rule (Rust PasswordPolicy::default min_length = 8); the cap
// bounds the argon2 hashing cost.
const (
	MinLength = 8
	MaxLength = 128
)

// commonPasswordsRaw is the SecLists 10k-most-common list (lower-cased,
// deduped at build time). ~70KB embedded; membership is checked against the
// lower-cased candidate.
//
//go:embed common_passwords.txt
var commonPasswordsRaw []byte

var commonPasswords = sync.OnceValue(func() map[string]struct{} {
	set := make(map[string]struct{}, 10_000)
	sc := bufio.NewScanner(bytes.NewReader(commonPasswordsRaw))
	for sc.Scan() {
		if w := strings.TrimSpace(sc.Text()); w != "" {
			set[w] = struct{}{}
		}
	}
	return set
})

// Violation describes why a password was rejected. Code is a stable
// machine-readable identifier; Message is safe to show to the end user.
type Violation struct {
	Code    string
	Message string
}

func (v *Violation) Error() string { return v.Code + ": " + v.Message }

// Validate checks pw against the policy. email and name identify the account
// the password is for (either may be empty when unknown); the password must
// not be derived from them. Returns nil when the password is acceptable.
func Validate(pw, email, name string) *Violation {
	if len(pw) < MinLength {
		return &Violation{"PASSWORD_TOO_SHORT",
			fmt.Sprintf("Password must be at least %d characters", MinLength)}
	}
	if len(pw) > MaxLength {
		return &Violation{"PASSWORD_TOO_LONG",
			fmt.Sprintf("Password must be at most %d characters", MaxLength)}
	}

	norm := strings.ToLower(strings.TrimSpace(pw))

	if allSameRune(norm) {
		return &Violation{"PASSWORD_TOO_WEAK",
			"Password cannot be a single repeated character"}
	}

	// Identity material: the full email, its local part, and each word of the
	// display name must not appear in the password (forwards or reversed —
	// "moc.caleb@werdna" is not a fix). Substrings shorter than 4 characters
	// are only rejected on exact equality; containment would be too eager
	// ("jo" appears in "majority").
	if c := identityViolation(norm, email, name); c != nil {
		return c
	}

	if _, common := commonPasswords()[norm]; common {
		return &Violation{"PASSWORD_TOO_COMMON",
			"That password is on the list of most commonly used passwords — choose something less guessable"}
	}
	// The product name is a house blocklist entry the public list can't know.
	if strings.Contains(norm, "flowcatalyst") {
		return &Violation{"PASSWORD_TOO_COMMON",
			"Password cannot be based on the product name"}
	}
	return nil
}

// identityViolation reports whether normPw is derived from the account's
// email or name. normPw must already be lower-cased.
func identityViolation(normPw, email, name string) *Violation {
	reversed := reverse(normPw)
	candidates := make([]string, 0, 6)
	if e := strings.ToLower(strings.TrimSpace(email)); e != "" {
		candidates = append(candidates, e)
		if at := strings.IndexByte(e, '@'); at > 0 {
			candidates = append(candidates, e[:at])
		}
	}
	for _, tok := range strings.Fields(strings.ToLower(name)) {
		candidates = append(candidates, tok)
	}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		var hit bool
		if len(c) >= 4 {
			hit = strings.Contains(normPw, c) || strings.Contains(reversed, c)
		} else {
			hit = normPw == c || reversed == c
		}
		if hit {
			return &Violation{"PASSWORD_CONTAINS_IDENTITY",
				"Password cannot contain your email address or name"}
		}
	}
	return nil
}

func allSameRune(s string) bool {
	var first rune
	for i, r := range s {
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			return false
		}
	}
	return true
}

func reverse(s string) string {
	r := []rune(s)
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return string(r)
}
