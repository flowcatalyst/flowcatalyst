package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// RegisterCommand is the input DTO for the FINISH-registration step.
// The BEGIN step is a read-only handler (no UoW commit) that issues the
// challenge — see api/api.go.
type RegisterCommand struct {
	StateID  string                `json:"stateId"`
	Response gowebauthn.Credential `json:"-"` // assembled from the *http.Request before being called
	Name     *string               `json:"name,omitempty"`
}

// Register persists the verified credential and emits [PasskeyRegistered].
//
// The caller (the HTTP handler in api/api.go) has already:
//  1. Consumed the ceremony state from CeremonyRepository
//  2. Invoked webauthn.Service.WebAuthn().FinishRegistration to verify
//     the attestation and produce the gowebauthn.Credential
//  3. Populated cmd.Response and cmd.Name
//
// Register's only job is to wrap the verified credential in our Credential
// entity and commit via the unit of work. This split keeps the use case body
// free of HTTP / library state — single-responsibility.
func Register(creds *webauthn.Repository) usecaseop.Operation[RegisterCommand, PasskeyRegistered] {
	return usecaseop.Operation[RegisterCommand, PasskeyRegistered]{
		Name: "Register",
		Validate: func(_ context.Context, cmd RegisterCommand) error {
			if strings.TrimSpace(cmd.StateID) == "" {
				return usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
			}
			return nil
		},
		// Intentionally open: this is a self-service passkey-registration flow.
		// The transport layer already requires an authenticated session (the
		// handler rejects callers without a principal), and the operation only
		// ever creates a credential owned by that caller — Execute binds the new
		// credential to ec.PrincipalID, so a caller can only register a passkey
		// for themselves. There is no separate permission gate to enforce here.
		Authorize: usecaseop.Public[RegisterCommand],
		Execute: func(_ context.Context, cmd RegisterCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PasskeyRegistered], error) {
			credential := webauthn.New(ec.PrincipalID, cmd.Response, cmd.Name)

			event := PasskeyRegistered{
				Metadata:     usecase.NewEventMetadata(ec, PasskeyRegisteredType, Source, subjectFor(credential.ID)),
				CredentialID: credential.ID,
				UserID:       ec.PrincipalID,
				Name:         cmd.Name,
			}
			return usecaseop.Save(credential, creds, event), nil
		},
	}
}
