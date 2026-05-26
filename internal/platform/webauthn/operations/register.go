package operations

import (
	"context"
	"strings"

	gowebauthn "github.com/go-webauthn/webauthn/webauthn"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RegisterCommand is the input DTO for the FINISH-registration step.
// The BEGIN step is a read-only handler (no UoW commit) that issues the
// challenge — see api/api.go.
type RegisterCommand struct {
	StateID  string                 `json:"stateId"`
	Response gowebauthn.Credential  `json:"-"` // assembled from the *http.Request before Run()
	Name     *string                `json:"name,omitempty"`
}

// RegisterUseCase implements UseCase. It validates the finish-registration
// payload (the authenticator's attestation), persists the new credential,
// and emits PasskeyRegistered.
type RegisterUseCase struct {
	creds *webauthn.Repository
	uow   *usecasepgx.UnitOfWork
}

// NewRegisterUseCase wires the use case.
func NewRegisterUseCase(creds *webauthn.Repository, uow *usecasepgx.UnitOfWork) *RegisterUseCase {
	return &RegisterUseCase{creds: creds, uow: uow}
}

func (uc *RegisterUseCase) Validate(_ context.Context, cmd RegisterCommand) error {
	if strings.TrimSpace(cmd.StateID) == "" {
		return usecase.Validation("STATE_ID_REQUIRED", "stateId is required")
	}
	return nil
}

func (uc *RegisterUseCase) Authorize(_ context.Context, _ RegisterCommand, _ usecase.ExecutionContext) error {
	return nil
}

// Execute persists the credential and emits PasskeyRegistered.
//
// The caller (the HTTP handler in api/api.go) has already:
//  1. Consumed the ceremony state from CeremonyRepository
//  2. Invoked webauthn.Service.WebAuthn().FinishRegistration to verify
//     the attestation and produce the gowebauthn.Credential
//  3. Populated cmd.Response and cmd.Name
//
// Execute's only job is to wrap the verified credential in our Credential
// entity and commit via UoW. This split keeps the use case body free of
// HTTP / library state — single-responsibility.
func (uc *RegisterUseCase) Execute(ctx context.Context, cmd RegisterCommand, ec usecase.ExecutionContext) usecase.Result[PasskeyRegistered] {
	credential := webauthn.New(ec.PrincipalID, cmd.Response, cmd.Name)

	event := PasskeyRegistered{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyRegisteredType, Source, subjectFor(credential.ID)),
		CredentialID: credential.ID,
		UserID:       ec.PrincipalID,
		Name:         cmd.Name,
	}
	return usecasepgx.Commit[webauthn.Credential, PasskeyRegistered, RegisterCommand](
		ctx, uc.uow, credential, uc.creds, event, cmd,
	)
}

var _ usecase.UseCase[RegisterCommand, PasskeyRegistered] = (*RegisterUseCase)(nil)
