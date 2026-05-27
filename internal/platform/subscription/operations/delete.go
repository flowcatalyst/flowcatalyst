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

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteSubscription removes a subscription and emits [SubscriptionDeleted].
func DeleteSubscription(
	ctx context.Context,
	repo *subscription.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[SubscriptionDeleted], error) {
	var zero commit.Committed[SubscriptionDeleted]

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
	event := SubscriptionDeleted{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionDeletedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
		Code:           s.Code,
	}
	return commit.Delete(ctx, uow, s, repo, event, cmd)
}
