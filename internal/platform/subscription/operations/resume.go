package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ResumeCommand is the input DTO.
type ResumeCommand struct {
	ID string `json:"id"`
}

// ResumeUseCase implements UseCase.
type ResumeUseCase struct {
	repo *subscription.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewResumeUseCase wires the use case.
func NewResumeUseCase(repo *subscription.Repository, uow *usecasepgx.UnitOfWork) *ResumeUseCase {
	return &ResumeUseCase{repo: repo, uow: uow}
}

func (uc *ResumeUseCase) Validate(_ context.Context, cmd ResumeCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *ResumeUseCase) Authorize(_ context.Context, _ ResumeCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *ResumeUseCase) Execute(ctx context.Context, cmd ResumeCommand, ec usecase.ExecutionContext) usecase.Result[SubscriptionResumed] {
	s, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[SubscriptionResumed](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if s == nil {
		return usecase.Failure[SubscriptionResumed](httperror.NotFound("Subscription", cmd.ID))
	}
	s.Resume()
	event := SubscriptionResumed{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionResumedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
	}
	return usecasepgx.Commit[subscription.Subscription, SubscriptionResumed, ResumeCommand](
		ctx, uc.uow, s, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[ResumeCommand, SubscriptionResumed] = (*ResumeUseCase)(nil)
