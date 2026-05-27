package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteRole removes a role and emits RoleDeleted. Roles with
// source=CODE cannot be deleted.
func DeleteRole(
	ctx context.Context,
	repo *role.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[RoleDeleted], error) {
	var zero commit.Committed[RoleDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	r, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if r == nil {
		return zero, httperror.NotFound("Role", cmd.ID)
	}
	if r.Source == role.SourceCode {
		return zero, usecase.Conflict("CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be deleted")
	}
	event := RoleDeleted{
		Metadata: usecase.NewEventMetadata(ec, RoleDeletedType, Source, subjectFor(r.ID)),
		RoleID:   r.ID,
		Name:     r.Name,
	}
	return commit.Delete(ctx, uow, r, repo, event, cmd)
}
