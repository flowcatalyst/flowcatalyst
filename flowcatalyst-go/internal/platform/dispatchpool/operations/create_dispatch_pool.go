package operations

import (
	"context"
	"regexp"

	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/dispatchpool"
	"go.flowcatalyst.tech/internal/platform/events"
)

// Code format: lowercase alphanumeric with hyphens
var dispatchPoolCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// CreateDispatchPoolCommand contains the data needed to create a dispatch pool
type CreateDispatchPoolCommand struct {
	Code            string `json:"code"`
	Name            string `json:"name"`
	Description     string `json:"description,omitempty"`
	ClientID        string `json:"clientId,omitempty"`
	MediatorType    string `json:"mediatorType"`
	Concurrency     int    `json:"concurrency"`
	QueueCapacity   int    `json:"queueCapacity"`
	RateLimitPerMin *int   `json:"rateLimitPerMin,omitempty"`
}

// CreateDispatchPoolUseCase handles creating a new dispatch pool
type CreateDispatchPoolUseCase struct {
	repo       dispatchpool.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateDispatchPoolUseCase creates a new CreateDispatchPoolUseCase
func NewCreateDispatchPoolUseCase(repo dispatchpool.Repository, uow common.UnitOfWork) *CreateDispatchPoolUseCase {
	return &CreateDispatchPoolUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new dispatch pool
func (uc *CreateDispatchPoolUseCase) Execute(
	ctx context.Context,
	cmd CreateDispatchPoolCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Dispatch pool code is required", nil),
		)
	}

	if !dispatchPoolCodePattern.MatchString(cmd.Code) {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CODE_FORMAT",
				"Dispatch pool code must be lowercase alphanumeric with hyphens",
				map[string]any{"code": cmd.Code}),
		)
	}

	if cmd.Concurrency <= 0 {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CONCURRENCY", "Concurrency must be greater than 0", nil),
		)
	}

	// Set defaults
	mediatorType := dispatchpool.MediatorType(cmd.MediatorType)
	if mediatorType == "" {
		mediatorType = dispatchpool.MediatorTypeHTTPWebhook
	}

	queueCapacity := cmd.QueueCapacity
	if queueCapacity <= 0 {
		queueCapacity = 1000 // Default
	}

	// Check for duplicate code
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil && err != dispatchpool.ErrNotFound {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing dispatch pool", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"A dispatch pool with this code already exists",
				map[string]any{"code": cmd.Code}),
		)
	}

	// Create dispatch pool
	dp := &dispatchpool.DispatchPool{
		Code:            cmd.Code,
		Name:            cmd.Name,
		Description:     cmd.Description,
		ClientID:        cmd.ClientID,
		MediatorType:    mediatorType,
		Concurrency:     cmd.Concurrency,
		QueueCapacity:   queueCapacity,
		RateLimitPerMin: cmd.RateLimitPerMin,
		Status:          dispatchpool.DispatchPoolStatusActive,
	}

	// Create domain event
	event := events.NewDispatchPoolCreated(execCtx, dp)

	// Atomic commit
	if cmd.ClientID != "" {
		return uc.unitOfWork.CommitWithClientID(ctx, dp, event, cmd, cmd.ClientID)
	}
	return uc.unitOfWork.Commit(ctx, dp, event, cmd)
}
