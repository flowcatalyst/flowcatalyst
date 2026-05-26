package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID             string  `json:"id"`
	Name           *string `json:"name,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *application.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *application.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
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

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationUpdated] {
	a, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ApplicationUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[ApplicationUpdated](httperror.NotFound("Application", cmd.ID))
	}
	if cmd.Name != nil {
		a.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		a.Description = cmd.Description
	}
	if cmd.IconURL != nil {
		a.IconURL = cmd.IconURL
	}
	if cmd.Website != nil {
		a.Website = cmd.Website
	}
	if cmd.DefaultBaseURL != nil {
		a.DefaultBaseURL = cmd.DefaultBaseURL
	}

	event := ApplicationUpdated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationUpdatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
		Name:          a.Name,
	}
	return usecasepgx.Commit[application.Application, ApplicationUpdated, UpdateCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, ApplicationUpdated] = (*UpdateUseCase)(nil)
