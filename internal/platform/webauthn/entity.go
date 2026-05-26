// Package webauthn is the port of fc-platform/src/webauthn. Passkey
// (WebAuthn) credentials and the registration/authentication ceremonies.
//
// Library: github.com/go-webauthn/webauthn. The library handles all the
// RFC mechanics (attestation, assertion, signature verification, counter
// monotonicity); we provide:
//   - persistence of registered credentials
//   - persistence of in-flight ceremony state (challenge + session data)
//   - routing the begin/finish HTTP endpoints
//   - mapping the library's User + Credential shapes to our Principal
//
// The go-webauthn library's *webauthn.WebAuthn instance is constructed
// once at startup with RP ID, RP origin, and other config. We pass it
// into the service struct.
package webauthn

import (
	"encoding/json"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

// Credential is the persisted WebAuthn credential. We wrap the
// library's webauthn.Credential to add FlowCatalyst metadata (id,
// principal, display name, timestamps).
type Credential struct {
	ID          string              `json:"id"`
	PrincipalID string              `json:"principalId"`
	Credential  webauthn.Credential `json:"credential"` // serialized to JSONB
	Name        *string             `json:"name,omitempty"`
	CreatedAt   time.Time           `json:"createdAt"`
	LastUsedAt  *time.Time          `json:"lastUsedAt,omitempty"`
}

// IDStr satisfies usecase.HasID.
func (c Credential) IDStr() string { return c.ID }

// New constructs a Credential.
func New(principalID string, cred webauthn.Credential, name *string) *Credential {
	now := time.Now().UTC()
	return &Credential{
		ID:          tsid.Generate(tsid.WebauthnCredential),
		PrincipalID: principalID,
		Credential:  cred,
		Name:        name,
		CreatedAt:   now,
	}
}

// CredentialIDBytes returns the opaque authenticator-issued ID. This is
// what the webauthn_credentials.credential_id BYTEA column indexes on.
func (c *Credential) CredentialIDBytes() []byte { return c.Credential.ID }

// RecordAuthentication updates the in-memory counter / backup-state on
// the wrapped credential after a successful assertion. The library's
// counter monotonicity check is already enforced before we get here.
func (c *Credential) RecordAuthentication(updated webauthn.Credential) {
	c.Credential = updated
	now := time.Now().UTC()
	c.LastUsedAt = &now
}

// MarshalJSON serializes the credential with a stable JSON shape. The
// library's webauthn.Credential is already JSON-friendly; nothing custom
// needed beyond the default.
func (c *Credential) MarshalJSON() ([]byte, error) {
	type alias Credential
	return json.Marshal((*alias)(c))
}
