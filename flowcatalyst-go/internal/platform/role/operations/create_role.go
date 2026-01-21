package operations

import (
	"context"
	"time"

	"go.flowcatalyst.tech/internal/common/tsid"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
	"go.flowcatalyst.tech/internal/platform/role"
)

// CreateRoleCommand contains the data needed to create a role
type CreateRoleCommand struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Scope       string   `json:"scope"`
	Permissions []string `json:"permissions,omitempty"`
}

// CreateRoleUseCase handles creating a new role
type CreateRoleUseCase struct {
	repo       role.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateRoleUseCase creates a new CreateRoleUseCase
func NewCreateRoleUseCase(repo role.Repository, uow common.UnitOfWork) *CreateRoleUseCase {
	return &CreateRoleUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new role
func (uc *CreateRoleUseCase) Execute(
	ctx context.Context,
	cmd CreateRoleCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Role code is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Role name is required", nil),
		)
	}

	// Check for duplicate code
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing role", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"A role with this code already exists",
				map[string]any{"code": cmd.Code}),
		)
	}

	// Create role
	now := time.Now()
	r := &role.Role{
		ID:          tsid.Generate(),
		Code:        cmd.Code,
		Name:        cmd.Name,
		Description: cmd.Description,
		Scope:       cmd.Scope,
		Permissions: cmd.Permissions,
		BuiltIn:     false,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Create domain event
	event := events.NewRoleCreated(execCtx, r)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, r, event, cmd)
}
