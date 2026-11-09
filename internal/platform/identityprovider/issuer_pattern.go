package identityprovider

import (
	"errors"
	"regexp/syntax"
	"strings"
)

// The issuer pattern of a multi-tenant OIDC provider (docs/oidc-security-audit.md
// §1). A multi-tenant IdP's shared signing keys sign tokens for EVERY tenant,
// so the issuer check is what pins the tenant: the pattern must name the
// issuer(s) this platform accepts exactly — e.g.
//
//	^https://login\.microsoftonline\.com/<tenant-id>/v2\.0$
//
// A pattern with a wildcard where the tenant goes ([^/]+, .*, a character
// class, a repeat) accepts a token from any tenant at the issuer layer, and an
// unanchored pattern accepts any issuer that merely CONTAINS a match. Both are
// refused at save; a stored one keeps working (with a warning) until the
// operator pins it, unless FC_OIDC_ISSUER_PATTERN_REQUIRE_PINNED is on.

// ErrIssuerPatternNotPinned is CheckIssuerPattern's refusal.
var ErrIssuerPatternNotPinned = errors.New("issuer pattern does not pin the issuer")

// IssuerPatternExample is the pinned form, quoted in refusals and on the
// console.
const IssuerPatternExample = `^https://login\.microsoftonline\.com/<tenant-id>/v2\.0$`

// CheckIssuerPattern reports why p does not pin the token issuer, or nil when
// it does. A pinned pattern:
//
//   - is a valid regular expression;
//   - is anchored at both ends (^…$) on every alternative, so it matches a
//     whole issuer and never a substring of one;
//   - names exact issuers only: literal text (escaped dots), optionally an
//     alternation of literals (several tenants, or the v1 and v2 issuer of one)
//     or an optional literal, at most maxPinnedIssuers of them — no ".", no
//     character range, no "*", "+" or repeat, nothing that matches a tenant
//     it does not spell out.
func CheckIssuerPattern(p string) error {
	if strings.TrimSpace(p) == "" {
		return errors.Join(ErrIssuerPatternNotPinned, errors.New("the pattern is empty"))
	}
	re, err := syntax.Parse(p, syntax.Perl)
	if err != nil {
		return errors.Join(ErrIssuerPatternNotPinned, err)
	}
	re = re.Simplify()
	if !beginAnchored(re) || !endAnchored(re) {
		return errors.Join(ErrIssuerPatternNotPinned, errors.New("it must be anchored with ^ at the start and $ at the end of every alternative"))
	}
	if issuerCount(re) > maxPinnedIssuers {
		return errors.Join(ErrIssuerPatternNotPinned, errors.New("it may only spell out exact issuers: no wildcard, character range or repeat (pin the tenant id)"))
	}
	return nil
}

// IssuerPatternPinned is CheckIssuerPattern as a predicate.
func IssuerPatternPinned(p string) bool { return CheckIssuerPattern(p) == nil }

// HasBroadIssuerPattern reports whether the provider's stored issuer pattern
// is one that would be refused at save today: a multi-tenant OIDC provider
// whose non-empty pattern does not pin the issuer. (An empty pattern means
// "the issuer URL exactly", which is pinned.)
func (i *IdentityProvider) HasBroadIssuerPattern() bool {
	if i == nil || i.Type != TypeOIDC || !i.OIDCMultiTenant || i.OIDCIssuerPattern == nil {
		return false
	}
	p := *i.OIDCIssuerPattern
	return strings.TrimSpace(p) != "" && !IssuerPatternPinned(p)
}

func beginAnchored(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpBeginText:
		return true
	case syntax.OpConcat:
		return len(re.Sub) > 0 && beginAnchored(re.Sub[0])
	case syntax.OpCapture:
		return beginAnchored(re.Sub[0])
	case syntax.OpAlternate:
		for _, s := range re.Sub {
			if !beginAnchored(s) {
				return false
			}
		}
		return true
	case syntax.OpNoMatch, syntax.OpEmptyMatch, syntax.OpLiteral, syntax.OpCharClass,
		syntax.OpAnyCharNotNL, syntax.OpAnyChar, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary,
		syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return false
	}
	return false
}

func endAnchored(re *syntax.Regexp) bool {
	switch re.Op {
	case syntax.OpEndText:
		return true
	case syntax.OpConcat:
		return len(re.Sub) > 0 && endAnchored(re.Sub[len(re.Sub)-1])
	case syntax.OpCapture:
		return endAnchored(re.Sub[0])
	case syntax.OpAlternate:
		for _, s := range re.Sub {
			if !endAnchored(s) {
				return false
			}
		}
		return true
	case syntax.OpNoMatch, syntax.OpEmptyMatch, syntax.OpLiteral, syntax.OpCharClass,
		syntax.OpAnyCharNotNL, syntax.OpAnyChar, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpBeginText, syntax.OpWordBoundary, syntax.OpNoWordBoundary,
		syntax.OpStar, syntax.OpPlus, syntax.OpQuest, syntax.OpRepeat:
		return false
	}
	return false
}

// maxPinnedIssuers bounds how many distinct issuers a pinned pattern may
// accept: a handful of tenants spelled out, never a family of them.
const maxPinnedIssuers = 32

// issuerCount is how many distinct strings re matches, capped at
// maxPinnedIssuers+1 (anything at or past the cap — including every
// unbounded pattern — is "too many"). Literals count once (a case-insensitive
// literal is still the one issuer); a character class counts each rune it
// holds, so the parser's factoring of "tenant1|tenant2" into "tenant[12]"
// stays countable while "[0-9a-f]{32}" does not.
func issuerCount(re *syntax.Regexp) int {
	const over = maxPinnedIssuers + 1
	switch re.Op {
	case syntax.OpLiteral, syntax.OpEmptyMatch, syntax.OpBeginText, syntax.OpEndText:
		return 1
	case syntax.OpCapture:
		return issuerCount(re.Sub[0])
	case syntax.OpQuest:
		return min(issuerCount(re.Sub[0])+1, over)
	case syntax.OpConcat:
		n := 1
		for _, s := range re.Sub {
			n = min(n*issuerCount(s), over)
		}
		return n
	case syntax.OpAlternate:
		n := 0
		for _, s := range re.Sub {
			n = min(n+issuerCount(s), over)
		}
		return n
	case syntax.OpCharClass:
		n := 0
		for i := 0; i+1 < len(re.Rune); i += 2 {
			n = min(n+int(re.Rune[i+1]-re.Rune[i])+1, over)
		}
		return n
	case syntax.OpNoMatch, syntax.OpAnyCharNotNL, syntax.OpAnyChar, syntax.OpBeginLine, syntax.OpEndLine,
		syntax.OpWordBoundary, syntax.OpNoWordBoundary, syntax.OpStar, syntax.OpPlus, syntax.OpRepeat:
		// Each matches text the pattern does not spell out (a repeat left by
		// Simplify is unbounded; the line anchors match inside a value).
		return over
	}
	return over
}
