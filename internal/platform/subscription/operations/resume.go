package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ResumeCommand is the input DTO.
type ResumeCommand struct {
	ID string `json:"id"`
}

// ResumeSubscription resumes a paused subscription and emits [SubscriptionResumed].
func ResumeSubscription(
	ctx context.Context,
	repo *subscription.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd ResumeCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[SubscriptionResumed], error) {
	var zero commit.Committed[SubscriptionResumed]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	s, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if s == nil {
		return zero, httperror.NotFound("Subscription", cmd.ID)
	}
	s.Resume()
	event := SubscriptionResumed{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionResumedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
	}
	return commit.Save(ctx, uow, s, repo, event, cmd)
}
