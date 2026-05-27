package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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
func Authenticate(
	ctx context.Context,
	creds *webauthn.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AuthenticateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[PasskeyAuthenticated], error) {
	var zero commit.Committed[PasskeyAuthenticated]

	if strings.TrimSpace(cmd.StateID) == "" {
		return zero, usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
	}
	if strings.TrimSpace(cmd.PersistedCredentialID) == "" {
		return zero, usecase.Validation("CREDENTIAL_ID_REQUIRED", "persisted credentialId is required")
	}

	credential, err := creds.FindByID(ctx, cmd.PersistedCredentialID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if credential == nil {
		return zero, httperror.NotFound("WebauthnCredential", cmd.PersistedCredentialID)
	}
	credential.RecordAuthentication(cmd.UpdatedCredential)

	event := PasskeyAuthenticated{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyAuthenticatedType, Source, subjectFor(credential.ID)),
		CredentialID: credential.ID,
		UserID:       credential.PrincipalID,
	}
	return commit.Save(ctx, uow, credential, creds, event, cmd)
}
