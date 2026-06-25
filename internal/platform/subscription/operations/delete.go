package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteSubscription removes a subscription and emits [SubscriptionDeleted].
func DeleteSubscription(repo *subscription.Repository) usecaseop.Operation[DeleteCommand, SubscriptionDeleted] {
	return usecaseop.Operation[DeleteCommand, SubscriptionDeleted]{
		Name: "DeleteSubscription",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may delete
		// subscriptions" permission is on the controller.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[SubscriptionDeleted], error) {
			s, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if s == nil {
				return nil, httperror.NotFound("Subscription", cmd.ID)
			}
			// Per-resource scope.
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), s.ClientID); err != nil {
				return nil, err
			}
			event := SubscriptionDeleted{
				Metadata:       usecase.NewEventMetadata(ec, SubscriptionDeletedType, Source, subjectFor(s.ID)),
				SubscriptionID: s.ID,
				Code:           s.Code,
			}
			return usecaseop.Delete(s, repo, event), nil
		},
	}
}
