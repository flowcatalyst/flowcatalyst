package operations

import (
	"context"

	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// UpdateApplicationCommand contains the data needed to update an application
type UpdateApplicationCommand struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
}

// UpdateApplicationUseCase handles updating an application
type UpdateApplicationUseCase struct {
	repo       *application.Repository
	unitOfWork common.UnitOfWork
}

// NewUpdateApplicationUseCase creates a new UpdateApplicationUseCase
func NewUpdateApplicationUseCase(repo *application.Repository, uow common.UnitOfWork) *UpdateApplicationUseCase {
	return &UpdateApplicationUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute updates an application
func (uc *UpdateApplicationUseCase) Execute(
	ctx context.Context,
	cmd UpdateApplicationCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.ID == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_ID", "Application ID is required", nil),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Application name is required", nil),
		)
	}

	// Fetch existing application
	existing, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to find application", map[string]any{"error": err.Error()}),
		)
	}
	if existing == nil {
		return common.Failure[common.DomainEvent](
			common.NotFoundError("APPLICATION_NOT_FOUND", "Application not found", map[string]any{"id": cmd.ID}),
		)
	}

	// Update fields (code and type are immutable)
	existing.Name = cmd.Name
	existing.Description = cmd.Description
	existing.DefaultBaseURL = cmd.DefaultBaseURL
	existing.IconURL = cmd.IconURL

	// Create domain event
	event := events.NewApplicationUpdated(execCtx, existing)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, existing, event, cmd)
}
