package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO. Permissions: nil means "don't touch";
// empty slice means "clear".
type UpdateCommand struct {
	ID            string   `json:"id"`
	DisplayName   *string  `json:"displayName,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	ClientManaged *bool    `json:"clientManaged,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *role.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *role.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.DisplayName != nil && strings.TrimSpace(*cmd.DisplayName) == "" {
		return usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName cannot be empty")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[RoleUpdated] {
	r, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[RoleUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if r == nil {
		return usecase.Failure[RoleUpdated](httperror.NotFound("Role", cmd.ID))
	}
	if r.Source == role.SourceCode {
		return usecase.Failure[RoleUpdated](usecase.Conflict(
			"CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be modified"))
	}
	if cmd.DisplayName != nil {
		r.DisplayName = strings.TrimSpace(*cmd.DisplayName)
	}
	if cmd.Description != nil {
		r.Description = cmd.Description
	}
	if cmd.Permissions != nil {
		r.Permissions = cmd.Permissions
	}
	if cmd.ClientManaged != nil {
		r.ClientManaged = *cmd.ClientManaged
	}

	event := RoleUpdated{
		Metadata: usecase.NewEventMetadata(ec, RoleUpdatedType, Source, subjectFor(r.ID)),
		RoleID:   r.ID,
		Name:     r.Name,
	}
	return usecasepgx.Commit[role.Role, RoleUpdated, UpdateCommand](
		ctx, uc.uow, r, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, RoleUpdated] = (*UpdateUseCase)(nil)
