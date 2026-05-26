package operations

import (
	"context"
	"regexp"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// codePattern matches the Rust regex: start with lowercase letter, then
// lowercase alphanumeric + hyphens.
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code             string  `json:"code"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	ServiceAccountID string  `json:"serviceAccountId"`
	ExternalID       *string `json:"externalId,omitempty"`
	ClientID         *string `json:"clientId,omitempty"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *connection.Repository
	uow  *usecasepgx.UnitOfWork
	// TODO(wave-3c): inject *serviceaccount.Repository to validate ServiceAccountID exists.
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *connection.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))
	if code == "" {
		return usecase.Validation("CODE_REQUIRED", "Connection code is required")
	}
	if !codePattern.MatchString(code) {
		return usecase.Validation("INVALID_CODE_FORMAT",
			"Code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "Connection name is required")
	}
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return usecase.Validation("SERVICE_ACCOUNT_REQUIRED", "serviceAccountId is required")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[ConnectionCreated] {
	code := strings.ToLower(strings.TrimSpace(cmd.Code))

	existing, err := uc.repo.FindByCodeAndClient(ctx, code, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ConnectionCreated](usecase.Internal("REPO", "find_by_code_and_client failed", err))
	}
	if existing != nil {
		return usecase.Failure[ConnectionCreated](usecase.Conflict(
			"CODE_EXISTS",
			"Connection with code '"+code+"' already exists"))
	}

	// TODO(wave-3c): validate that ServiceAccountID exists once service_account is ported.

	c := connection.New(code, strings.TrimSpace(cmd.Name), cmd.ServiceAccountID)
	c.Description = cmd.Description
	c.ExternalID = cmd.ExternalID
	c.ClientID = cmd.ClientID

	event := ConnectionCreated{
		Metadata:     usecase.NewEventMetadata(ec, ConnectionCreatedType, Source, subjectFor(c.ID)),
		ConnectionID: c.ID,
		Code:         c.Code,
		Name:         c.Name,
	}
	return usecasepgx.Commit[connection.Connection, ConnectionCreated, CreateCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, ConnectionCreated] = (*CreateUseCase)(nil)
