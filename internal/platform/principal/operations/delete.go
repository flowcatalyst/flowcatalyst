package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
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
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[UserDeleted] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[UserDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[UserDeleted](httperror.NotFound("Principal", cmd.ID))
	}
	email := ""
	if p.UserIdentity != nil {
		email = p.UserIdentity.Email
	}
	event := UserDeleted{
		Metadata: usecase.NewEventMetadata(ec, UserDeletedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Email:    email,
	}
	return usecasepgx.CommitDelete[principal.Principal, UserDeleted, DeleteCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, UserDeleted] = (*DeleteUseCase)(nil)
