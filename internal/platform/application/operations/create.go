package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code           string  `json:"code"`
	Name           string  `json:"name"`
	Type           string  `json:"type,omitempty"`
	Description    *string `json:"description,omitempty"`
	IconURL        *string `json:"iconUrl,omitempty"`
	Website        *string `json:"website,omitempty"`
	DefaultBaseURL *string `json:"defaultBaseUrl,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *application.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *application.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name is required")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationCreated] {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))

	existing, err := uc.repo.FindByCode(ctx, code)
	if err != nil {
		return usecase.Failure[ApplicationCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[ApplicationCreated](usecase.Conflict(
			"CODE_EXISTS", "Application with code '"+code+"' already exists"))
	}

	var a *application.Application
	if cmd.Type == string(application.TypeIntegration) {
		a = application.NewIntegration(code, strings.TrimSpace(cmd.Name))
	} else {
		a = application.New(code, strings.TrimSpace(cmd.Name))
	}
	a.Description = cmd.Description
	a.IconURL = cmd.IconURL
	a.Website = cmd.Website
	a.DefaultBaseURL = cmd.DefaultBaseURL

	event := ApplicationCreated{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationCreatedType, Source, subjectFor(a.ID)),
		ApplicationID: a.ID,
		Code:          a.Code,
		Name:          a.Name,
	}
	return usecasepgx.Commit[application.Application, ApplicationCreated, CreateCommand](
		ctx, uc.uow, a, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ApplicationCreated] = (*CreateUseCase)(nil)
