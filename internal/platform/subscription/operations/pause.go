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

// PauseCommand is the input DTO.
type PauseCommand struct {
	ID string `json:"id"`
}

// PauseSubscription pauses a subscription and emits [SubscriptionPaused].
// Idempotent — pausing a paused subscription still emits the event (matches Rust).
func PauseSubscription(repo *subscription.Repository) usecaseop.Operation[PauseCommand, SubscriptionPaused] {
	return usecaseop.Operation[PauseCommand, SubscriptionPaused]{
		Name: "PauseSubscription",
		Validate: func(_ context.Context, cmd PauseCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz runs post-load in Execute; the coarse "may write
		// subscriptions" permission is on the controller.
		Authorize: usecaseop.Public[PauseCommand],
		Execute: func(ctx context.Context, cmd PauseCommand, ec usecase.ExecutionContext) (usecaseop.Plan[SubscriptionPaused], error) {
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
			s.Pause()
			event := SubscriptionPaused{
				Metadata:       usecase.NewEventMetadata(ec, SubscriptionPausedType, Source, subjectFor(s.ID)),
				SubscriptionID: s.ID,
			}
			return usecaseop.Save(s, repo, event), nil
		},
	}
}
