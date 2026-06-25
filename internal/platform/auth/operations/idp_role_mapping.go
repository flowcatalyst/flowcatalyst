package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateIdpRoleMappingCommand struct {
	IdpType          string `json:"idpType"`
	IdpRoleName      string `json:"idpRoleName"`
	PlatformRoleName string `json:"platformRoleName"`
}

// CreateIdpRoleMapping validates the command, persists the mapping, and emits
// [IdpRoleMappingCreated]. IDP role mappings are platform-level config with no
// per-client resource dimension (Authorize: Public); the controller gates
// writes with auth.RequireAnchor.
func CreateIdpRoleMapping(repo *auth.IdpRoleMappingRepo) usecaseop.Operation[CreateIdpRoleMappingCommand, IdpRoleMappingCreated] {
	return usecaseop.Operation[CreateIdpRoleMappingCommand, IdpRoleMappingCreated]{
		Name: "CreateIdpRoleMapping",
		Validate: func(_ context.Context, cmd CreateIdpRoleMappingCommand) error {
			for k, v := range map[string]string{
				"idpType": cmd.IdpType, "idpRoleName": cmd.IdpRoleName, "platformRoleName": cmd.PlatformRoleName,
			} {
				if strings.TrimSpace(v) == "" {
					return usecase.Validation("FIELD_REQUIRED", k+" is required")
				}
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateIdpRoleMappingCommand],
		Execute: func(_ context.Context, cmd CreateIdpRoleMappingCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdpRoleMappingCreated], error) {
			m := auth.NewIdpRoleMapping(cmd.IdpType, cmd.IdpRoleName, cmd.PlatformRoleName)
			event := IdpRoleMappingCreated{
				Metadata:         usecase.NewEventMetadata(ec, IdpRoleMappingCreatedType, Source, mappingSubject(m.ID)),
				MappingID:        m.ID,
				IdpType:          m.IdpType,
				IdpRoleName:      m.IdpRoleName,
				PlatformRoleName: m.PlatformRoleName,
			}
			return usecaseop.Save(m, repo, event), nil
		},
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteIdpRoleMappingCommand struct {
	ID string `json:"id"`
}

// DeleteIdpRoleMapping removes the mapping and emits [IdpRoleMappingDeleted].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func DeleteIdpRoleMapping(repo *auth.IdpRoleMappingRepo) usecaseop.Operation[DeleteIdpRoleMappingCommand, IdpRoleMappingDeleted] {
	return usecaseop.Operation[DeleteIdpRoleMappingCommand, IdpRoleMappingDeleted]{
		Name: "DeleteIdpRoleMapping",
		Validate: func(_ context.Context, cmd DeleteIdpRoleMappingCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteIdpRoleMappingCommand],
		Execute: func(ctx context.Context, cmd DeleteIdpRoleMappingCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdpRoleMappingDeleted], error) {
			m, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if m == nil {
				return nil, httperror.NotFound("IdpRoleMapping", cmd.ID)
			}
			event := IdpRoleMappingDeleted{
				Metadata:    usecase.NewEventMetadata(ec, IdpRoleMappingDeletedType, Source, mappingSubject(m.ID)),
				MappingID:   m.ID,
				IdpRoleName: m.IdpRoleName,
			}
			return usecaseop.Delete(m, repo, event), nil
		},
	}
}
