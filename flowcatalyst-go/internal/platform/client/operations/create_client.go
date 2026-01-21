package operations

import (
	"context"
	"regexp"
	"strings"

	"go.flowcatalyst.tech/internal/platform/client"
	"go.flowcatalyst.tech/internal/platform/common"
	"go.flowcatalyst.tech/internal/platform/events"
)

// Identifier format: lowercase alphanumeric with hyphens
var clientIdentifierPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// CreateClientCommand contains the data needed to create a client
type CreateClientCommand struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// CreateClientUseCase handles creating a new client
type CreateClientUseCase struct {
	repo       client.Repository
	unitOfWork common.UnitOfWork
}

// NewCreateClientUseCase creates a new CreateClientUseCase
func NewCreateClientUseCase(repo client.Repository, uow common.UnitOfWork) *CreateClientUseCase {
	return &CreateClientUseCase{
		repo:       repo,
		unitOfWork: uow,
	}
}

// Execute creates a new client
func (uc *CreateClientUseCase) Execute(
	ctx context.Context,
	cmd CreateClientCommand,
	execCtx *common.ExecutionContext,
) common.Result[common.DomainEvent] {
	// Validation
	if cmd.Name == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_NAME", "Client name is required", nil),
		)
	}

	if cmd.Identifier == "" {
		return common.Failure[common.DomainEvent](
			common.ValidationError("MISSING_IDENTIFIER", "Client identifier is required", nil),
		)
	}

	identifier := strings.ToLower(cmd.Identifier)
	if !clientIdentifierPattern.MatchString(identifier) {
		return common.Failure[common.DomainEvent](
			common.ValidationError("INVALID_IDENTIFIER_FORMAT",
				"Client identifier must be lowercase alphanumeric with hyphens",
				map[string]any{"identifier": cmd.Identifier}),
		)
	}

	// Check for duplicate identifier
	existing, err := uc.repo.FindByIdentifier(ctx, identifier)
	if err != nil {
		return common.Failure[common.DomainEvent](
			common.InternalError("DB_ERROR", "Failed to check for existing client", map[string]any{"error": err.Error()}),
		)
	}
	if existing != nil {
		return common.Failure[common.DomainEvent](
			common.BusinessRuleError("IDENTIFIER_EXISTS",
				"A client with this identifier already exists",
				map[string]any{"identifier": identifier}),
		)
	}

	// Create client
	c := &client.Client{
		Name:       cmd.Name,
		Identifier: identifier,
		Status:     client.ClientStatusActive,
	}

	// Create domain event
	event := events.NewClientCreated(execCtx, c)

	// Atomic commit
	return uc.unitOfWork.Commit(ctx, c, event, cmd)
}
