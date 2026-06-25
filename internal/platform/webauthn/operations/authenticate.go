package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// AuthenticateCommand is the input DTO for the FINISH-authentication step.
// Like RegisterCommand, the verified Credential is assembled from the
// *http.Request before Authenticate runs.
type AuthenticateCommand struct {
	StateID               string                `json:"stateId"`
	UpdatedCredential     gowebauthn.Credential `json:"-"` // returned by FinishLogin
	PersistedCredentialID string                `json:"-"` // looked up by the handler from the assertion's credential ID
}

// Authenticate records the successful authentication (updates counter +
// last_used_at) and emits [PasskeyAuthenticated].
func Authenticate(creds *webauthn.Repository) usecaseop.Operation[AuthenticateCommand, PasskeyAuthenticated] {
	return usecaseop.Operation[AuthenticateCommand, PasskeyAuthenticated]{
		Name: "Authenticate",
		Validate: func(_ context.Context, cmd AuthenticateCommand) error {
			if strings.TrimSpace(cmd.StateID) == "" {
				return usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
			}
			if strings.TrimSpace(cmd.PersistedCredentialID) == "" {
				return usecase.Validation("CREDENTIAL_ID_REQUIRED", "persisted credentialId is required")
			}
			return nil
		},
		// Intentionally open: this is the self-service passkey sign-in flow. The
		// caller already proved control of the credential by completing the
		// WebAuthn assertion (verified by the handler before this op runs), and
		// the operation only updates that same credential's counter/last_used_at
		// on the credential's own principal. The completed ceremony is the gate;
		// there is no separate permission to check.
		Authorize: usecaseop.Public[AuthenticateCommand],
		Execute: func(ctx context.Context, cmd AuthenticateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[PasskeyAuthenticated], error) {
			credential, err := creds.FindByID(ctx, cmd.PersistedCredentialID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if credential == nil {
				return nil, httperror.NotFound("WebauthnCredential", cmd.PersistedCredentialID)
			}
			credential.RecordAuthentication(cmd.UpdatedCredential)

			event := PasskeyAuthenticated{
				Metadata:     usecase.NewEventMetadata(ec, PasskeyAuthenticatedType, Source, subjectFor(credential.ID)),
				CredentialID: credential.ID,
				UserID:       credential.PrincipalID,
			}
			return usecaseop.Save(credential, creds, event), nil
		},
	}
}
