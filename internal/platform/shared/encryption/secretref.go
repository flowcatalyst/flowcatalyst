package encryption

import (
	"errors"
	"strings"
)

// ErrNotConfigured is returned by EncryptSecretRef when a plaintext secret
// needs encrypting but no key is configured (FLOWCATALYST_APP_KEY unset).
// Callers map it to a user-facing validation error.
var ErrNotConfigured = errors.New("encryption not configured: FLOWCATALYST_APP_KEY is unset")

// externalSecretSchemes are secret-manager reference prefixes that are stored
// verbatim and resolved at read time — never encrypted inline.
var externalSecretSchemes = []string{"aws-sm://", "aws-ps://", "gcp-sm://", "vault://", "env://", "literal:"}

// EncryptSecretRef converts an incoming secret reference into its at-rest form.
//
// A plaintext value — optionally carrying the SecretRefInput "encrypt:"
// directive — is encrypted inline as "encrypted:<blob>", the format Decrypt
// reads (and the one the Rust/TS producers emit). Values that are already an
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
