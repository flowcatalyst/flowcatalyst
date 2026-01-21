package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/principal"
	"go.flowcatalyst.tech/internal/platform/serviceaccount"
)

// CreateServiceAccountCommand contains the data needed to create a service account
type CreateServiceAccountCommand struct {
	Code          string                        `json:"code"`
	Name          string                        `json:"name"`
	Description   string                        `json:"description,omitempty"`
	ClientIDs     []string                      `json:"clientIds,omitempty"`
	ApplicationID string                        `json:"applicationId,omitempty"`
	Roles         []principal.RoleAssignment    `json:"roles,omitempty"`
}

// CreateServiceAccountUseCase handles creating a new service account
type CreateServiceAccountUseCase struct {
	repo       *serviceaccount.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateServiceAccountUseCase creates a new CreateServiceAccountUseCase
func NewCreateServiceAccountUseCase(repo *serviceaccount.Repository, uow common.UnitOfWork) *CreateServiceAccountUseCase {
	return &CreateServiceAccountUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new service account
func (uc *CreateServiceAccountUseCase) Execute(
	ctx context.Context,
	cmd CreateServiceAccountCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Service account code is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Service account name is required", nil),
		)
	}

	// Check for duplicate code
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing service account", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"A service account with this code already exists",
				map[string]any{"code": cmd.Code}),
		)
	}

	// Create service account
	now := time.Now()
	sa := &serviceaccount.ServiceAccount{
		ID:            tsid.Generate(),
		Code:          cmd.Code,
		Name:          cmd.Name,
		Description:   cmd.Description,
		ClientIDs:     cmd.ClientIDs,
		ApplicationID: cmd.ApplicationID,
		Active:        true,
		Roles:         cmd.Roles,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Create domain event
	event := events.NewServiceAccountCreated(execCtx, sa)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, sa, event, cmd)
}
