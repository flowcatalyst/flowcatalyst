package operations

import (
	"context"
	"regexp"

	"go.flowcatalyst.tech/internal/platform/application"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// Code format: lowercase alphanumeric with hyphens
var applicationCodePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// CreateApplicationCommand contains the data needed to create an application
type CreateApplicationCommand struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Type           string `json:"type"`
	DefaultBaseURL string `json:"defaultBaseUrl,omitempty"`
	IconURL        string `json:"iconUrl,omitempty"`
}

// CreateApplicationUseCase handles creating a new application
type CreateApplicationUseCase struct {
	repo       *application.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateApplicationUseCase creates a new CreateApplicationUseCase
func NewCreateApplicationUseCase(repo *application.Repository, uow common.UnitOfWork) *CreateApplicationUseCase {
	return &CreateApplicationUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new application
func (uc *CreateApplicationUseCase) Execute(
	ctx context.Context,
	cmd CreateApplicationCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Code == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_CODE", "Application code is required", nil),
		)
	}

	if !applicationCodePattern.MatchString(cmd.Code) {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_CODE_FORMAT",
				"Application code must be lowercase alphanumeric with hyphens",
				map[string]any{"code": cmd.Code}),
		)
	}

	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Application name is required", nil),
		)
	}

	// Validate type
	appType := application.ApplicationType(cmd.Type)
	if appType == "" {
		appType = application.ApplicationTypeApplication // Default
	}

	switch appType {
	case application.ApplicationTypeApplication, application.ApplicationTypeIntegration:
		// Valid
	default:
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_TYPE",
				"Application type must be APPLICATION or INTEGRATION",
				map[string]any{"type": cmd.Type}),
		)
	}

	// Check for duplicate code
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing application", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("CODE_EXISTS",
				"An application with this code already exists",
				map[string]any{"code": cmd.Code}),
		)
	}

	// Create application
	app := &application.Application{
		Code:           cmd.Code,
		Name:           cmd.Name,
		Description:    cmd.Description,
		Type:           appType,
		DefaultBaseURL: cmd.DefaultBaseURL,
		IconURL:        cmd.IconURL,
		Active:         true,
	}

	// Create domain event
	event := events.NewApplicationCreated(execCtx, app)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, app, event, cmd)
}
