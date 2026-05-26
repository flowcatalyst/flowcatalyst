package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	ApplicationCode string   `json:"applicationCode"`
	RoleName        string   `json:"roleName"`
	DisplayName     string   `json:"displayName"`
	Description     *string  `json:"description,omitempty"`
	Permissions     []string `json:"permissions,omitempty"`
	ClientManaged   bool     `json:"clientManaged"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *role.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *role.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
	}
	if strings.TrimSpace(cmd.RoleName) == "" {
		return usecase.Validation("ROLE_NAME_REQUIRED", "roleName is required")
	}
	if strings.TrimSpace(cmd.DisplayName) == "" {
		return usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName is required")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[RoleCreated] {
	fullName := cmd.ApplicationCode + ":" + cmd.RoleName
	existing, err := uc.repo.FindByName(ctx, fullName)
	if err != nil {
		return usecase.Failure[RoleCreated](usecase.Internal("REPO", "find_by_name failed", err))
	}
	if existing != nil {
		return usecase.Failure[RoleCreated](usecase.Conflict(
			"ROLE_EXISTS", "Role '"+fullName+"' already exists"))
	}
	r := role.New(cmd.ApplicationCode, cmd.RoleName, cmd.DisplayName)
	r.Description = cmd.Description
	r.ClientManaged = cmd.ClientManaged
	for _, p := range cmd.Permissions {
		r.GrantPermission(p)
	}

	event := RoleCreated{
		Metadata: usecase.NewEventMetadata(ec, RoleCreatedType, Source, subjectFor(r.ID)),
		RoleID:   r.ID,
		Name:     r.Name,
	}
	return usecasepgx.Commit[role.Role, RoleCreated, CreateCommand](
		ctx, uc.uow, r, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, RoleCreated] = (*CreateUseCase)(nil)
