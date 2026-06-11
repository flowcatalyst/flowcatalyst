package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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

// CreateDispatchPool creates a new dispatch pool and emits DispatchPoolCreated.
func CreateDispatchPool(
	ctx context.Context,
	repo *dispatchpool.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[DispatchPoolCreated], error) {
	var zero commit.Committed[DispatchPoolCreated]

	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "code is required")
	}
	// Deliberately the underscore-tolerant pattern: matches sync and the
	// Rust pool_code_pattern (owner-approved widening from hyphen-only).
	if !validate.CodeUnderscorePattern.MatchString(code) {
		return zero, usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumeric, hyphens, underscores")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name is required")
	}
	if cmd.Concurrency != nil && *cmd.Concurrency < 1 {
		return zero, usecase.Validation("INVALID_CONCURRENCY", "concurrency must be >= 1")
	}
	if cmd.RateLimit != nil && *cmd.RateLimit < 0 {
		return zero, usecase.Validation("INVALID_RATE_LIMIT", "rateLimit cannot be negative")
	}

	existing, err := repo.FindByCode(ctx, code, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("CODE_EXISTS", "Dispatch pool with code '"+code+"' already exists")
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
	return commit.Save(ctx, uow, p, repo, event, cmd)
}
