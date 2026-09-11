package encryption

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrNotConfigured is returned by EncryptSecretRef when a plaintext secret
// needs encrypting but no key is configured (FLOWCATALYST_APP_KEY unset).
// Callers map it to a user-facing validation error.
var ErrNotConfigured = errors.New("encryption not configured: FLOWCATALYST_APP_KEY is unset")

// externalSecretSchemes are secret-manager reference prefixes that are stored
// verbatim and resolved at read time — never encrypted inline.
var externalSecretSchemes = []string{"aws-sm://", "aws-ps://", "gcp-sm://", "vault://", "env://", "literal:"}

// secretManagerSchemeNames lists the secret-manager schemes for messages, in
// their "aws-sm://" form (owner ruling 2026-09-08: a typo like "aws-smm://"
// must see the correct spelling) and without "literal:" — that is the dev
// bypass, not a secret manager, even though it is accepted as a reference.
func secretManagerSchemeNames() []string {
	out := make([]string, 0, len(externalSecretSchemes))
	for _, s := range externalSecretSchemes {
		if strings.HasSuffix(s, "://") {
			out = append(out, s)
		}
	}
	return out
}

// ErrUnsupportedScheme is returned by EncryptSecretRef for a "<scheme>://…"
// value whose scheme is not in externalSecretSchemes. Callers map it to a
// user-facing validation error (400, not 500 — it is the caller's input).
var ErrUnsupportedScheme = errors.New("unsupported secret-manager scheme")

// unsupportedScheme returns the scheme of a "<scheme>://…" value that claims a
// secret-manager reference we do not support, or "" when there is nothing to
// reject (owner ruling 2026-09-08: the list is closed and an unknown scheme is
// *rejected*, never encrypted — silently sealing a mistyped "aws-smm://…"
// stores a reference as though it were the secret itself).
//
// Write-side only: Decrypt and every read path still treat such a value
// exactly as before, so rows already stored under an unknown scheme keep
// working. The "encrypt:" directive is the override for a genuine secret
// shaped like a URL — it carries a ':', so it can never form a scheme token
// and never reaches the rejection.
func unsupportedScheme(v string) string {
	i := strings.Index(v, "://")
	if i <= 0 {
		return ""
	}
	scheme := v[:i]
	if !isSchemeToken(scheme) {
		return "" // not a scheme: a secret that merely contains "://"
	}
	if slices.Contains(externalSecretSchemes, scheme+"://") {
		return ""
	}
	return scheme
}

// isSchemeToken reports whether s is an RFC 3986 scheme:
// ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ).
func isSchemeToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
		case i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.'):
		default:
			return false
		}
	}
	return true
}

// EncryptSecretRef converts an incoming secret reference into its at-rest form.
//
// A plaintext value — optionally carrying the SecretRefInput "encrypt:"
// directive — is encrypted inline as "encrypted:<blob>", the format Decrypt
// reads (and the one other producers emit, e.g. the TS SDK). Values that are already an
// inline ciphertext ("encrypted:…") or an external provider reference
// ("aws-sm://", "vault://", …) pass through unchanged. A nil pointer (the
// field was omitted) is preserved so an update leaves the stored secret
// untouched, and an empty string is preserved so it can clear the secret.
//
// Returns ErrNotConfigured when a plaintext value needs encrypting but enc is
// nil — never silently store a plaintext secret.
func EncryptSecretRef(enc *Service, ref *string) (*string, error) {
	if ref == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*ref)
	if v == "" {
		return ref, nil
	}
	if strings.HasPrefix(v, "encrypted:") {
		return &v, nil // already an inline ciphertext — idempotent
	}
	for _, scheme := range externalSecretSchemes {
		if strings.HasPrefix(v, scheme) {
			return &v, nil // external reference, resolved at read time
		}
	}
	if scheme := unsupportedScheme(v); scheme != "" {
		return nil, fmt.Errorf("%w %q; supported: %s (prefix the value with \"encrypt:\" to store it as an encrypted plaintext secret instead)",
			ErrUnsupportedScheme, scheme+"://", strings.Join(secretManagerSchemeNames(), ", "))
	}
	plaintext := strings.TrimPrefix(v, "encrypt:")
	if enc == nil {
		return nil, ErrNotConfigured
	}
	blob, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	out := "encrypted:" + blob
	return &out, nil
}
