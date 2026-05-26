package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code               string                            `json:"code"`
	Name               string                            `json:"name"`
	Description        *string                           `json:"description,omitempty"`
	Scope              *string                           `json:"scope,omitempty"`
	ApplicationID      *string                           `json:"applicationId,omitempty"`
	WebhookCredentials *serviceaccount.WebhookCredentials `json:"webhookCredentials,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if !codePattern.MatchString(code) {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name is required")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountCreated] {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))

	existing, err := uc.repo.FindByCode(ctx, code)
	if err != nil {
		return usecase.Failure[ServiceAccountCreated](usecase.Internal("REPO", "find_by_code failed", err))
	}
	if existing != nil {
		return usecase.Failure[ServiceAccountCreated](usecase.Conflict(
			"CODE_EXISTS", "Service account with code '"+code+"' already exists"))
	}

	sa := serviceaccount.New(code, strings.TrimSpace(cmd.Name))
	sa.Description = cmd.Description
	sa.Scope = cmd.Scope
	sa.ApplicationID = cmd.ApplicationID
	if cmd.WebhookCredentials != nil {
		sa.WebhookCredentials = *cmd.WebhookCredentials
	}

	event := ServiceAccountCreated{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountCreatedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		Code:             sa.Code,
		Name:             sa.Name,
	}
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountCreated, CreateCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ServiceAccountCreated] = (*CreateUseCase)(nil)
