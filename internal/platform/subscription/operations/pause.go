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

// PauseCommand is the input DTO.
type PauseCommand struct {
	ID string `json:"id"`
}

// PauseSubscription pauses a subscription and emits [SubscriptionPaused].
// Idempotent — pausing a paused subscription still emits the event (matches Rust).
func PauseSubscription(
	ctx context.Context,
	repo *subscription.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd PauseCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[SubscriptionPaused], error) {
	var zero commit.Committed[SubscriptionPaused]

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
	s.Pause()
	event := SubscriptionPaused{
		Metadata:       usecase.NewEventMetadata(ec, SubscriptionPausedType, Source, subjectFor(s.ID)),
		SubscriptionID: s.ID,
	}
	return commit.Save(ctx, uow, s, repo, event, cmd)
}
