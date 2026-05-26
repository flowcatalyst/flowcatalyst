package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
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
	repo *role.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *role.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[RoleDeleted] {
	r, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[RoleDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if r == nil {
		return usecase.Failure[RoleDeleted](httperror.NotFound("Role", cmd.ID))
	}
	if r.Source == role.SourceCode {
		return usecase.Failure[RoleDeleted](usecase.Conflict(
			"CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be deleted"))
	}
	event := RoleDeleted{
		Metadata: usecase.NewEventMetadata(ec, RoleDeletedType, Source, subjectFor(r.ID)),
		RoleID:   r.ID,
		Name:     r.Name,
	}
	return usecasepgx.CommitDelete[role.Role, RoleDeleted, DeleteCommand](
		ctx, uc.uow, r, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, RoleDeleted] = (*DeleteUseCase)(nil)
