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

// ResumeCommand is the input DTO.
type ResumeCommand struct {
	ID string `json:"id"`
}

// ResumeSubscription resumes a paused subscription and emits [SubscriptionResumed].
func ResumeSubscription(repo *subscription.Repository) usecaseop.Operation[ResumeCommand, SubscriptionResumed] {
	return usecaseop.Operation[ResumeCommand, SubscriptionResumed]{
		Name: "ResumeSubscription",
		Validate: func(_ context.Context, cmd ResumeCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may write
		// subscriptions" permission is on the controller.
		Authorize: usecaseop.Public[ResumeCommand],
		Execute: func(ctx context.Context, cmd ResumeCommand, ec usecase.ExecutionContext) (usecaseop.Plan[SubscriptionResumed], error) {
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
			s.Resume()
			event := SubscriptionResumed{
				Metadata:       usecase.NewEventMetadata(ec, SubscriptionResumedType, Source, subjectFor(s.ID)),
				SubscriptionID: s.ID,
			}
			return usecaseop.Save(s, repo, event), nil
		},
	}
}
