package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID                 string                             `json:"id"`
	Name               *string                            `json:"name,omitempty"`
	Description        *string                            `json:"description,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
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

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountUpdated] {
	sa, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ServiceAccountUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountUpdated](httperror.NotFound("ServiceAccount", cmd.ID))
	}
	if cmd.Name != nil {
		sa.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		sa.Description = cmd.Description
	}
	if cmd.WebhookCredentials != nil {
		sa.WebhookCredentials = *cmd.WebhookCredentials
	}

	event := ServiceAccountUpdated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountUpdatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Name:             sa.Name,
	}
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountUpdated, UpdateCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, ServiceAccountUpdated] = (*UpdateUseCase)(nil)
