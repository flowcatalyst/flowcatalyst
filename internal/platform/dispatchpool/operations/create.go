package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	RateLimit   *int32  `json:"rateLimit,omitempty"`
	Concurrency *int32  `json:"concurrency,omitempty"`
	ClientID    *string `json:"clientId,omitempty"`
}

// CreateDispatchPool validates cmd, enforces per-resource client scope,
// enforces uniqueness on (code, clientID), persists the pool, and atomically
// emits [DispatchPoolCreated].
func CreateDispatchPool(repo *dispatchpool.Repository) usecaseop.Operation[CreateCommand, DispatchPoolCreated] {
	return usecaseop.Operation[CreateCommand, DispatchPoolCreated]{
		Name: "CreateDispatchPool",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			// Deliberately the underscore-tolerant pattern: matches sync and the
			// Rust pool_code_pattern (owner-approved widening from hyphen-only).
			if !validate.CodeUnderscorePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"code must start with a lowercase letter and contain only lowercase alphanumeric, hyphens, underscores")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if cmd.Concurrency != nil && *cmd.Concurrency < 1 {
				return usecase.Validation("INVALID_CONCURRENCY", "concurrency must be >= 1")
			}
			if cmd.RateLimit != nil && *cmd.RateLimit < 0 {
				return usecase.Validation("INVALID_RATE_LIMIT", "rateLimit cannot be negative")
			}
			return nil
		},
		// Resource-level authorization (the coarse "may write dispatch pools"
		// permission is enforced at the controller). A pool bound to a client
		// may only be created by a principal with access to that client; a
		// platform-wide pool (nil ClientID) requires anchor. This is exactly
		// auth.CheckScopeAccess on the target client.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[DispatchPoolCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			existing, err := repo.FindByCode(ctx, code, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS", "Dispatch pool with code '"+code+"' already exists")
			}

			p := dispatchpool.New(code, strings.TrimSpace(cmd.Name))
			p.Description = cmd.Description
			p.RateLimit = cmd.RateLimit
			if cmd.Concurrency != nil {
				p.Concurrency = *cmd.Concurrency
			}
			p.ClientID = cmd.ClientID

			event := DispatchPoolCreated{
				Metadata: usecase.NewEventMetadata(ec, DispatchPoolCreatedType, Source, subjectFor(p.ID)),
				PoolID:   p.ID,
				Code:     p.Code,
				Name:     p.Name,
			}
			return usecaseop.Save(p, repo, event), nil
		},
	}
}
