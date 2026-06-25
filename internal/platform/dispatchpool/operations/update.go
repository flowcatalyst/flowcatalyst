package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand applies optional updates.
type UpdateCommand struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty"`
	Concurrency *int32  `json:"concurrency,omitempty"`
}

// UpdateDispatchPool mutates an existing dispatch pool and atomically emits
// [DispatchPoolUpdated].
func UpdateDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[UpdateCommand, DispatchPoolUpdated] {
	return usecaseop.Operation[UpdateCommand, DispatchPoolUpdated]{
		Name: "UpdateDispatchPool",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
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
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may write dispatch pools" permission is on the
		// controller.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolUpdated], error) {
			p, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("DispatchPool", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), p.ClientID); err != nil {
				return nil, err
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
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
