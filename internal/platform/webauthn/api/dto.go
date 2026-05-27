// dto.go contains the wire-format types for the WebAuthn API.
package api

import (
	"encoding/json"

	"github.com/go-webauthn/webauthn/webauthn"

	wa "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
)

// RegisterBeginRequest is the wire body for POST /auth/webauthn/register/begin.
type RegisterBeginRequest struct {
	DisplayName *string `json:"displayName,omitempty"`
}

// RegisterBeginResponse is returned by POST /auth/webauthn/register/begin.
type RegisterBeginResponse struct {
	StateID string `json:"stateId"`
	Options any    `json:"options"`
}

// RegisterCompleteRequest is the wire body for
// POST /auth/webauthn/register/complete.
type RegisterCompleteRequest struct {
	StateID    string          `json:"stateId"`
	Name       *string         `json:"name,omitempty"`
	Credential json.RawMessage `json:"credential"`
}

// RegisterCompleteResponse is returned by register/complete.
type RegisterCompleteResponse struct {
	CredentialID string `json:"credentialId"`
}

// AuthenticateBeginRequest is the wire body for
// POST /auth/webauthn/authenticate/begin.
type AuthenticateBeginRequest struct {
	Email string `json:"email"`
}

// AuthenticateBeginResponse is returned by authenticate/begin.
type AuthenticateBeginResponse struct {
	StateID string `json:"stateId"`
	Options any    `json:"options"`
}

// AuthenticateCompleteRequest is the wire body for
// POST /auth/webauthn/authenticate/complete.
type AuthenticateCompleteRequest struct {
	StateID    string          `json:"stateId"`
	Credential json.RawMessage `json:"credential"`
}

// CredentialResponse mirrors webauthn.Credential.
type CredentialResponse struct {
	ID          string              `json:"id"`
	PrincipalID string              `json:"principalId"`
	Credential  webauthn.Credential `json:"credential"`
	Name        *string             `json:"name,omitempty"`
	CreatedAt   httpcompat.Time     `json:"createdAt"`
	LastUsedAt  *httpcompat.Time    `json:"lastUsedAt,omitempty"`
}

func credentialFromEntity(c *wa.Credential) CredentialResponse {
	var lastUsed *httpcompat.Time
	if c.LastUsedAt != nil {
		v := jsontime.New(*c.LastUsedAt)
		lastUsed = &v
	}
	return CredentialResponse{
		ID:          c.ID,
		PrincipalID: c.PrincipalID,
		Credential:  c.Credential,
		Name:        c.Name,
		CreatedAt:   jsontime.New(c.CreatedAt),
		LastUsedAt:  lastUsed,
	}
}

// CredentialListResponse is the wire shape for GET /auth/webauthn/credentials.
type CredentialListResponse struct {
	Items []CredentialResponse `json:"items"`
}
