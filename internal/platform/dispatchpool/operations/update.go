package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand applies optional updates.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty"`
	Concurrency *int32  `json:"concurrency,omitempty"`
}

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *dispatchpool.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *dispatchpool.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	if cmd.Concurrency != nil && *cmd.Concurrency < 1 {
		return usecase.Validation("INVALID_CONCURRENCY", "concurrency must be >= 1")
	}
	if cmd.RateLimit != nil && *cmd.RateLimit < 0 {
		return usecase.Validation("INVALID_RATE_LIMIT", "rateLimit cannot be negative")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[DispatchPoolUpdated] {
	p, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[DispatchPoolUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[DispatchPoolUpdated](httperror.NotFound("DispatchPool", cmd.ID))
	}
	if cmd.Name != nil {
		p.Name = strings.TrimSpace(*cmd.Name)
	}
	if cmd.Description != nil {
		p.Description = cmd.Description
	}
	if cmd.RateLimit != nil {
		p.RateLimit = cmd.RateLimit
	}
	if cmd.Concurrency != nil {
		p.Concurrency = *cmd.Concurrency
	}

	event := DispatchPoolUpdated{
		Metadata: usecase.NewEventMetadata(ec, DispatchPoolUpdatedType, Source, subjectFor(p.ID)),
		PoolID:   p.ID,
		Name:     p.Name,
	}
	return usecasepgx.Commit[dispatchpool.DispatchPool, DispatchPoolUpdated, UpdateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, DispatchPoolUpdated] = (*UpdateUseCase)(nil)
