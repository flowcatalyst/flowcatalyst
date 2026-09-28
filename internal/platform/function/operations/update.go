package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand applies optional settings updates. Address, ApplicationID,
// ClientID, and Name are immutable — the address is the function's identity
// (plan §4) — so this command carries none of them.
type UpdateCommand struct {
	ID          string       `json:"id"`
	Description *string      `json:"description,omitempty"`
	Pool        *string      `json:"pool,omitempty"`
	ClearPool   bool         `json:"clearPool,omitempty"`
	Warm        *bool        `json:"warm,omitempty"`
	Limits      *LimitsInput `json:"limits,omitempty"`
}

// UpdateFunction mutates an existing function's settings (description,
// pool, warm, limits — validated against the platform ceilings) and
// atomically emits [FunctionUpdated].
func UpdateFunction(repo *function.Repository) usecaseop.Operation[UpdateCommand, FunctionUpdated] {
	return usecaseop.Operation[UpdateCommand, FunctionUpdated]{
		Name: "UpdateFunction",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Per-resource authz needs the loaded row, so it runs post-load in
		// Execute; the coarse "may manage functions" permission is on the
		// controller.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[FunctionUpdated], error) {
			f, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if f == nil {
				return nil, httperror.NotFound("Function", cmd.ID)
			}
			if err := auth.CheckScopeAccess(auth.FromContext(ctx), f.ClientID); err != nil {
				return nil, err
			}

			if cmd.Description != nil {
				f.Description = cmd.Description
			}
			if cmd.ClearPool {
				f.Pool = nil
			} else if cmd.Pool != nil {
				f.Pool = cmd.Pool
			}
			if cmd.Warm != nil {
				f.Warm = *cmd.Warm
			}
			newLimits := cmd.Limits.applyTo(f.Limits)
			if err := validateLimits(newLimits); err != nil {
				return nil, err
			}
			f.Limits = newLimits

			event := FunctionUpdated{
				Metadata:   usecase.NewEventMetadata(ec, FunctionUpdatedType, Source, subjectFor(f.ID)),
				FunctionID: f.ID,
				Address:    f.Address,
			}
			return usecaseop.Save(f, repo, event), nil
		},
	}
}
