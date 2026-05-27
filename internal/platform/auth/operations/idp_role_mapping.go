package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateIdpRoleMappingCommand struct {
	IdpType          string `json:"idpType"`
	IdpRoleName      string `json:"idpRoleName"`
	PlatformRoleName string `json:"platformRoleName"`
}

func CreateIdpRoleMapping(
	ctx context.Context,
	repo *auth.IdpRoleMappingRepo,
	uow *usecasepgx.UnitOfWork,
	cmd CreateIdpRoleMappingCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[IdpRoleMappingCreated], error) {
	var zero commit.Committed[IdpRoleMappingCreated]
	for k, v := range map[string]string{
		"idpType": cmd.IdpType, "idpRoleName": cmd.IdpRoleName, "platformRoleName": cmd.PlatformRoleName,
	} {
		if strings.TrimSpace(v) == "" {
			return zero, usecase.Validation("FIELD_REQUIRED", k+" is required")
		}
	}
	m := auth.NewIdpRoleMapping(cmd.IdpType, cmd.IdpRoleName, cmd.PlatformRoleName)
	event := IdpRoleMappingCreated{
		Metadata:         usecase.NewEventMetadata(ec, IdpRoleMappingCreatedType, Source, mappingSubject(m.ID)),
		MappingID:        m.ID,
		IdpType:          m.IdpType,
		IdpRoleName:      m.IdpRoleName,
		PlatformRoleName: m.PlatformRoleName,
	}
	return commit.Save(ctx, uow, m, repo, event, cmd)
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteIdpRoleMappingCommand struct {
	ID string `json:"id"`
}

func DeleteIdpRoleMapping(
	ctx context.Context,
	repo *auth.IdpRoleMappingRepo,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteIdpRoleMappingCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[IdpRoleMappingDeleted], error) {
	var zero commit.Committed[IdpRoleMappingDeleted]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	m, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if m == nil {
		return zero, httperror.NotFound("IdpRoleMapping", cmd.ID)
	}
	event := IdpRoleMappingDeleted{
		Metadata:    usecase.NewEventMetadata(ec, IdpRoleMappingDeletedType, Source, mappingSubject(m.ID)),
		MappingID:   m.ID,
		IdpRoleName: m.IdpRoleName,
	}
	return commit.Delete(ctx, uow, m, repo, event, cmd)
}
