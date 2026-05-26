package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AuthenticateCommand is the input DTO for the FINISH-authentication step.
// Like RegisterCommand, the verified Credential is assembled from the
// *http.Request before Execute() runs.
type AuthenticateCommand struct {
	StateID            string                `json:"stateId"`
	UpdatedCredential  gowebauthn.Credential `json:"-"` // returned by FinishLogin
	PersistedCredentialID string             `json:"-"` // looked up by the handler from the assertion's credential ID
}

// AuthenticateUseCase implements UseCase. It records the successful
// authentication (updates counter + last_used_at) and emits
// PasskeyAuthenticated.
type AuthenticateUseCase struct {
	creds *webauthn.Repository
	uow   *usecasepgx.UnitOfWork
}

// NewAuthenticateUseCase wires the use case.
func NewAuthenticateUseCase(creds *webauthn.Repository, uow *usecasepgx.UnitOfWork) *AuthenticateUseCase {
	return &AuthenticateUseCase{creds: creds, uow: uow}
}

func (uc *AuthenticateUseCase) Validate(_ context.Context, cmd AuthenticateCommand) error {
	if strings.TrimSpace(cmd.StateID) == "" {
		return usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
	}
	if strings.TrimSpace(cmd.PersistedCredentialID) == "" {
		return usecase.Validation("CREDENTIAL_ID_REQUIRED", "persisted credentialId is required")
	}
	return nil
}

func (uc *AuthenticateUseCase) Authorize(_ context.Context, _ AuthenticateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AuthenticateUseCase) Execute(ctx context.Context, cmd AuthenticateCommand, ec usecase.ExecutionContext) usecase.Result[PasskeyAuthenticated] {
	credential, err := uc.creds.FindByID(ctx, cmd.PersistedCredentialID)
	if err != nil {
		return usecase.Failure[PasskeyAuthenticated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if credential == nil {
		return usecase.Failure[PasskeyAuthenticated](httperror.NotFound("WebauthnCredential", cmd.PersistedCredentialID))
	}
	credential.RecordAuthentication(cmd.UpdatedCredential)

	event := PasskeyAuthenticated{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyAuthenticatedType, Source, subjectFor(credential.ID)),
		CredentialID: credential.ID,
		UserID:       credential.PrincipalID,
	}
	return usecasepgx.Commit[webauthn.Credential, PasskeyAuthenticated, AuthenticateCommand](
		ctx, uc.uow, credential, uc.creds, event, cmd,
	)
}

var _ usecase.UseCase[AuthenticateCommand, PasskeyAuthenticated] = (*AuthenticateUseCase)(nil)
