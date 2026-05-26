package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
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
	repo *emaildomainmapping.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewDeleteUseCase wires the use case.
func NewDeleteUseCase(repo *emaildomainmapping.Repository, uow *usecasepgx.UnitOfWork) *DeleteUseCase {
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

func (uc *DeleteUseCase) Execute(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) usecase.Result[EmailDomainMappingDeleted] {
	e, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[EmailDomainMappingDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if e == nil {
		return usecase.Failure[EmailDomainMappingDeleted](httperror.NotFound("EmailDomainMapping", cmd.ID))
	}
	event := EmailDomainMappingDeleted{
		Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingDeletedType, Source, subjectFor(e.ID)),
		MappingID:   e.ID,
		EmailDomain: e.EmailDomain,
	}
	return usecasepgx.CommitDelete[emaildomainmapping.EmailDomainMapping, EmailDomainMappingDeleted, DeleteCommand](
		ctx, uc.uow, e, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteCommand, EmailDomainMappingDeleted] = (*DeleteUseCase)(nil)
