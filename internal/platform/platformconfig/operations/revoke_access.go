package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RevokeAccessCommand is the input DTO.
type RevokeAccessCommand struct {
	ID string `json:"id"`
}

// RevokeAccessUseCase implements UseCase.
type RevokeAccessUseCase struct {
	repo *platformconfig.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewRevokeAccessUseCase wires the use case.
func NewRevokeAccessUseCase(repo *platformconfig.Repository, uow *usecasepgx.UnitOfWork) *RevokeAccessUseCase {
	return &RevokeAccessUseCase{repo: repo, uow: uow}
}

func (uc *RevokeAccessUseCase) Validate(_ context.Context, cmd RevokeAccessCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *RevokeAccessUseCase) Authorize(_ context.Context, _ RevokeAccessCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *RevokeAccessUseCase) Execute(ctx context.Context, cmd RevokeAccessCommand, ec usecase.ExecutionContext) usecase.Result[AccessRevoked] {
	a, err := uc.repo.FindAccessByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[AccessRevoked](usecase.Internal("REPO", "find_access_by_id failed", err))
	}
	if a == nil {
		return usecase.Failure[AccessRevoked](httperror.NotFound("PlatformConfigAccess", cmd.ID))
	}
	event := AccessRevoked{
		Metadata:        usecase.NewEventMetadata(ec, AccessRevokedType, Source, subjectFor(a.ID)),
		AccessID:        a.ID,
		ApplicationCode: a.ApplicationCode,
		RoleCode:        a.RoleCode,
	}
	return usecasepgx.CommitDelete[platformconfig.Access, AccessRevoked, RevokeAccessCommand](
		ctx, uc.uow, a, newAccessRepo(uc.repo), event, cmd,
	)
}

var _ usecase.UseCase[RevokeAccessCommand, AccessRevoked] = (*RevokeAccessUseCase)(nil)
