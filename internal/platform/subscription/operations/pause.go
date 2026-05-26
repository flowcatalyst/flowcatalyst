package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// PauseCommand is the input DTO.
type PauseCommand struct {
	ID string `json:"id"`
}

// PauseUseCase implements UseCase.
type PauseUseCase struct {
	repo *subscription.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewPauseUseCase wires the use case.
func NewPauseUseCase(repo *subscription.Repository, uow *usecasepgx.UnitOfWork) *PauseUseCase {
	return &PauseUseCase{repo: repo, uow: uow}
}

func (uc *PauseUseCase) Validate(_ context.Context, cmd PauseCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *PauseUseCase) Authorize(_ context.Context, _ PauseCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *PauseUseCase) Execute(ctx context.Context, cmd PauseCommand, ec usecase.ExecutionContext) usecase.Result[SubscriptionPaused] {
	s, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[SubscriptionPaused](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if s == nil {
		return usecase.Failure[SubscriptionPaused](httperror.NotFound("Subscription", cmd.ID))
	}
	if !s.IsActive() {
		// Idempotent: pausing a paused subscription is a no-op success.
		// Match the Rust behaviour by still emitting the event.
	}
	s.Pause()
	event := SubscriptionPaused{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionPausedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
	}
	return usecasepgx.Commit[subscription.Subscription, SubscriptionPaused, PauseCommand](
		ctx, uc.uow, s, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[PauseCommand, SubscriptionPaused] = (*PauseUseCase)(nil)
