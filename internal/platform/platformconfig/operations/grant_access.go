package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// GrantAccessCommand is the input DTO.
type GrantAccessCommand struct {
	ApplicationCode string `json:"applicationCode"`
	RoleCode        string `json:"roleCode"`
	CanWrite        bool   `json:"canWrite"`
}

// GrantAccessUseCase implements UseCase.
type GrantAccessUseCase struct {
	repo *platformconfig.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewGrantAccessUseCase wires the use case.
func NewGrantAccessUseCase(repo *platformconfig.Repository, uow *usecasepgx.UnitOfWork) *GrantAccessUseCase {
	return &GrantAccessUseCase{repo: repo, uow: uow}
}

func (uc *GrantAccessUseCase) Validate(_ context.Context, cmd GrantAccessCommand) error {
	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
	}
	if strings.TrimSpace(cmd.RoleCode) == "" {
		return usecase.Validation("ROLE_REQUIRED", "roleCode is required")
	}
	return nil
}

func (uc *GrantAccessUseCase) Authorize(_ context.Context, _ GrantAccessCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *GrantAccessUseCase) Execute(ctx context.Context, cmd GrantAccessCommand, ec usecase.ExecutionContext) usecase.Result[AccessGranted] {
	existing, err := uc.repo.FindAccessByRole(ctx, cmd.ApplicationCode, cmd.RoleCode)
	if err != nil {
		return usecase.Failure[AccessGranted](usecase.Internal("REPO", "find_access_by_role failed", err))
	}
	var a *platformconfig.Access
	if existing != nil {
		a = existing
		a.CanRead = true
		a.CanWrite = cmd.CanWrite
	} else {
		a = platformconfig.NewAccess(cmd.ApplicationCode, cmd.RoleCode)
		a.CanWrite = cmd.CanWrite
	}

	event := AccessGranted{
		Metadata:        usecase.NewEventMetadata(ec, AccessGrantedType, Source, subjectFor(a.ID)),
		AccessID:        a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
		CanWrite:        a.CanWrite,
	}
	return usecasepgx.Commit[platformconfig.Access, AccessGranted, GrantAccessCommand](
		ctx, uc.uow, a, newAccessRepo(uc.repo), event, cmd,
	)
}

var _ usecase.UseCase[GrantAccessCommand, AccessGranted] = (*GrantAccessUseCase)(nil)
