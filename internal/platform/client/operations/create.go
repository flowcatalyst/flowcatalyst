package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var identifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*[a-z0-9]$|^[a-z0-9]$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *client.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *client.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name is required")
	}
	id := strings.ToLower(strings.TrimSpace(cmd.Identifier))
	if id == "" {
		return usecase.Validation("IDENTIFIER_REQUIRED", "identifier is required")
	}
	if !identifierPattern.MatchString(id) {
		return usecase.Validation("INVALID_IDENTIFIER",
			"identifier must be lowercase alphanumeric with optional hyphens (URL-safe)")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ClientCreated] {
	id := strings.ToLower(strings.TrimSpace(cmd.Identifier))
	existing, err := uc.repo.FindByIdentifier(ctx, id)
	if err != nil {
		return usecase.Failure[ClientCreated](usecase.Internal("REPO", "find_by_identifier failed", err))
	}
	if existing != nil {
		return usecase.Failure[ClientCreated](usecase.Conflict(
			"IDENTIFIER_EXISTS", "Client with identifier '"+id+"' already exists"))
	}
	c := client.New(strings.TrimSpace(cmd.Name), id)

	event := ClientCreated{
		Metadata:   usecase.NewEventMetadata(ec, ClientCreatedType, Source, subjectFor(c.ID)),
		ClientID:   c.ID,
		Name:       c.Name,
		Identifier: c.Identifier,
	}
	return usecasepgx.Commit[client.Client, ClientCreated, CreateCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ClientCreated] = (*CreateUseCase)(nil)
