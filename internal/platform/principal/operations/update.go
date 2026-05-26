package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand is the input DTO. Nil pointers mean "don't change".
type UpdateCommand struct {
	ID        string  `json:"id"`
	Name      *string `json:"name,omitempty"`
	FirstName *string `json:"firstName,omitempty"`
	LastName  *string `json:"lastName,omitempty"`
	Phone     *string `json:"phone,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *principal.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *principal.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
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

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[UserUpdated] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[UserUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[UserUpdated](httperror.NotFound("Principal", cmd.ID))
	}
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if p.UserIdentity != nil {
		if cmd.FirstName != nil {
			p.UserIdentity.FirstName = cmd.FirstName
		}
		if cmd.LastName != nil {
			p.UserIdentity.LastName = cmd.LastName
		}
		if cmd.Phone != nil {
			p.UserIdentity.Phone = cmd.Phone
		}
	}

	event := UserUpdated{
		Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Name:     p.Name,
	}
	return usecasepgx.Commit[principal.Principal, UserUpdated, UpdateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, UserUpdated] = (*UpdateUseCase)(nil)
