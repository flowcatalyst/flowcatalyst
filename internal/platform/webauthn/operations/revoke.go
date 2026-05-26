package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RevokeCommand is the input DTO.
type RevokeCommand struct {
	ID string `json:"id"`
}

// RevokeUseCase implements UseCase.
type RevokeUseCase struct {
	creds *webauthn.Repository
	uow   *usecasepgx.UnitOfWork
}

// NewRevokeUseCase wires the use case.
func NewRevokeUseCase(creds *webauthn.Repository, uow *usecasepgx.UnitOfWork) *RevokeUseCase {
	return &RevokeUseCase{creds: creds, uow: uow}
}

func (uc *RevokeUseCase) Validate(_ context.Context, cmd RevokeCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *RevokeUseCase) Authorize(_ context.Context, _ RevokeCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *RevokeUseCase) Execute(ctx context.Context, cmd RevokeCommand, ec usecase.ExecutionContext) usecase.Result[PasskeyRevoked] {
	c, err := uc.creds.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[PasskeyRevoked](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[PasskeyRevoked](httperror.NotFound("WebauthnCredential", cmd.ID))
	}
	event := PasskeyRevoked{
		Metadata:     usecase.NewEventMetadata(ec, PasskeyRevokedType, Source, subjectFor(c.ID)),
		CredentialID: c.ID,
		UserID:       c.PrincipalID,
	}
	return usecasepgx.CommitDelete[webauthn.Credential, PasskeyRevoked, RevokeCommand](
		ctx, uc.uow, c, uc.creds, event, cmd,
	)
}

var _ usecase.UseCase[RevokeCommand, PasskeyRevoked] = (*RevokeUseCase)(nil)
