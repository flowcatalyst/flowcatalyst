package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	ExternalID  *string `json:"externalId,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *connection.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *connection.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "Connection name is required")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[ConnectionUpdated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[ConnectionUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[ConnectionUpdated](httperror.NotFound("Connection", cmd.ID))
	}
	c.Name = strings.TrimSpace(cmd.Name)
	c.Description = cmd.Description
	c.ExternalID = cmd.ExternalID

	event := ConnectionUpdated{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionUpdatedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Name:         c.Name,
	}
	return usecasepgx.Commit[connection.Connection, ConnectionUpdated, UpdateCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, ConnectionUpdated] = (*UpdateUseCase)(nil)
