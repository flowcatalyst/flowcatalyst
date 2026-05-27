package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// UpdateDispatchPool mutates an existing dispatch pool and emits
// DispatchPoolUpdated.
func UpdateDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolUpdated], error) {
	var zero commit.Committed[DispatchPoolUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	if cmd.Concurrency != nil && *cmd.Concurrency < 1 {
		return zero, usecase.Validation("INVALID_CONCURRENCY", "concurrency must be >= 1")
	}
	if cmd.RateLimit != nil && *cmd.RateLimit < 0 {
		return zero, usecase.Validation("INVALID_RATE_LIMIT", "rateLimit cannot be negative")
	}

	p, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("DispatchPool", cmd.ID)
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
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
