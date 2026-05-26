package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID          string   `json:"id"`
	Name        *string  `json:"name,omitempty"`
	Description *string  `json:"description,omitempty"`
	Body        *string  `json:"body,omitempty"`
	DiagramType *string  `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *process.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *process.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[ProcessUpdated] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ProcessUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[ProcessUpdated](httperror.NotFound("Process", cmd.ID))
	}
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		p.Description = cmd.Description
	}
	if cmd.Body != nil {
		p.Body = *cmd.Body
	}
	if cmd.DiagramType != nil {
		p.DiagramType = *cmd.DiagramType
	}
	if cmd.Tags != nil {
		p.Tags = cmd.Tags
	}

	event := ProcessUpdated{
		Metadata:  usecase.NewEventMetadata(ec, ProcessUpdatedType, Source, subjectFor(p.ID)),
		ProcessID: p.ID,
		Name:      p.Name,
	}
	return usecasepgx.Commit[process.Process, ProcessUpdated, UpdateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, ProcessUpdated] = (*UpdateUseCase)(nil)
