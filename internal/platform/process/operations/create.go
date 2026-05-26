package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code        string   `json:"code"`
	Name        string   `json:"name"`
	Description *string  `json:"description,omitempty"`
	Body        string   `json:"body,omitempty"`
	DiagramType string   `json:"diagramType,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *process.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *process.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	if strings.TrimSpace(cmd.Code) == "" {
		return usecase.Validation("CODE_REQUIRED", "Process code is required")
	}
	parts := strings.Split(cmd.Code, ":")
	if len(parts) != 3 {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"Process code must follow format: application:subdomain:process-name")
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return usecase.Validation("INVALID_CODE_FORMAT", "Process code segments cannot be empty")
		}
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "Process name is required")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ProcessCreated] {
	existing, err := uc.repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return usecase.Failure[ProcessCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[ProcessCreated](usecase.Conflict(
			"CODE_EXISTS",
			"Process with code '"+cmd.Code+"' already exists"))
	}

	p, err := process.New(cmd.Code, strings.TrimSpace(cmd.Name))
	if err != nil {
		return usecase.Failure[ProcessCreated](usecase.Validation("INVALID_CODE_FORMAT", err.Error()))
	}
	p.Description = cmd.Description
	p.Body = cmd.Body
	if cmd.DiagramType != "" {
		p.DiagramType = cmd.DiagramType
	}
	if cmd.Tags != nil {
		p.Tags = cmd.Tags
	}
	p.CreatedBy = &ec.PrincipalID

	event := ProcessCreated{
		Metadata:  usecase.NewEventMetadata(ec, ProcessCreatedType, Source, subjectFor(p.ID)),
		ProcessID: p.ID,
		Code:      p.Code,
		Name:      p.Name,
	}
	return usecasepgx.Commit[process.Process, ProcessCreated, CreateCommand](
		ctx, uc.uow, p, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ProcessCreated] = (*CreateUseCase)(nil)
