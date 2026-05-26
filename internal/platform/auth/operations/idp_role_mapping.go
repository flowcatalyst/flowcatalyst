package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateIdpRoleMappingCommand struct {
	IdpType          string `json:"idpType"`
	IdpRoleName      string `json:"idpRoleName"`
	PlatformRoleName string `json:"platformRoleName"`
}

type CreateIdpRoleMappingUseCase struct {
	repo *auth.IdpRoleMappingRepo
	uow  *usecasepgx.UnitOfWork
}

func NewCreateIdpRoleMappingUseCase(repo *auth.IdpRoleMappingRepo, uow *usecasepgx.UnitOfWork) *CreateIdpRoleMappingUseCase {
	return &CreateIdpRoleMappingUseCase{repo: repo, uow: uow}
}

func (uc *CreateIdpRoleMappingUseCase) Validate(_ context.Context, cmd CreateIdpRoleMappingCommand) error {
	for k, v := range map[string]string{
		"idpType": cmd.IdpType, "idpRoleName": cmd.IdpRoleName, "platformRoleName": cmd.PlatformRoleName,
	} {
		if strings.TrimSpace(v) == "" {
			return usecase.Validation("FIELD_REQUIRED", k+" is required")
		}
	}
	return nil
}

func (uc *CreateIdpRoleMappingUseCase) Authorize(_ context.Context, _ CreateIdpRoleMappingCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateIdpRoleMappingUseCase) Execute(ctx context.Context, cmd CreateIdpRoleMappingCommand, ec usecase.ExecutionContext) usecase.Result[IdpRoleMappingCreated] {
	m := auth.NewIdpRoleMapping(cmd.IdpType, cmd.IdpRoleName, cmd.PlatformRoleName)
	event := IdpRoleMappingCreated{
		Metadata:         usecase.NewEventMetadata(ec, IdpRoleMappingCreatedType, Source, mappingSubject(m.ID)),
		MappingID:        m.ID,
		IdpType:          m.IdpType,
		IdpRoleName:      m.IdpRoleName,
		PlatformRoleName: m.PlatformRoleName,
	}
	return usecasepgx.Commit[auth.IdpRoleMapping, IdpRoleMappingCreated, CreateIdpRoleMappingCommand](
		ctx, uc.uow, m, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateIdpRoleMappingCommand, IdpRoleMappingCreated] = (*CreateIdpRoleMappingUseCase)(nil)

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteIdpRoleMappingCommand struct {
	ID string `json:"id"`
}

type DeleteIdpRoleMappingUseCase struct {
	repo *auth.IdpRoleMappingRepo
	uow  *usecasepgx.UnitOfWork
}

func NewDeleteIdpRoleMappingUseCase(repo *auth.IdpRoleMappingRepo, uow *usecasepgx.UnitOfWork) *DeleteIdpRoleMappingUseCase {
	return &DeleteIdpRoleMappingUseCase{repo: repo, uow: uow}
}

func (uc *DeleteIdpRoleMappingUseCase) Validate(_ context.Context, cmd DeleteIdpRoleMappingCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeleteIdpRoleMappingUseCase) Authorize(_ context.Context, _ DeleteIdpRoleMappingCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteIdpRoleMappingUseCase) Execute(ctx context.Context, cmd DeleteIdpRoleMappingCommand, ec usecase.ExecutionContext) usecase.Result[IdpRoleMappingDeleted] {
	m, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[IdpRoleMappingDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if m == nil {
		return usecase.Failure[IdpRoleMappingDeleted](httperror.NotFound("IdpRoleMapping", cmd.ID))
	}
	event := IdpRoleMappingDeleted{
		Metadata:    usecase.NewEventMetadata(ec, IdpRoleMappingDeletedType, Source, mappingSubject(m.ID)),
		MappingID:   m.ID,
		IdpRoleName: m.IdpRoleName,
	}
	return usecasepgx.CommitDelete[auth.IdpRoleMapping, IdpRoleMappingDeleted, DeleteIdpRoleMappingCommand](
		ctx, uc.uow, m, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteIdpRoleMappingCommand, IdpRoleMappingDeleted] = (*DeleteIdpRoleMappingUseCase)(nil)
