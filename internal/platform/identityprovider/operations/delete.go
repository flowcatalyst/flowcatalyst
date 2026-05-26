package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteUseCase implements UseCase.
type DeleteUseCase struct {
	repo *identityprovider.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *identityprovider.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
	return &DeleteUseCase{repo: repo, uow: uow}
}

func (uc *DeleteUseCase) Validate(_ context.Context, cmd DeleteCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeleteUseCase) Authorize(_ context.Context, _ DeleteCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[IdentityProviderDeleted] {
	ip, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[IdentityProviderDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if ip == nil {
		return usecase.Failure[IdentityProviderDeleted](httperror.NotFound("IdentityProvider", cmd.ID))
	}
	event := IdentityProviderDeleted{
		Metadata:           usecase.NewEventMetadata(ec, IdentityProviderDeletedType, Source, subjectFor(ip.ID)),
		IdentityProviderID: ip.ID,
		Code:               ip.Code,
	}
	return usecasepgx.CommitDelete[identityprovider.IdentityProvider, IdentityProviderDeleted, DeleteCommand](
		ctx, uc.uow, ip, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, IdentityProviderDeleted] = (*DeleteUseCase)(nil)
