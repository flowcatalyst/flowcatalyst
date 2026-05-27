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

// UpdateCommand is the input DTO. Permissions: nil means "don't touch";
// empty slice means "clear".
type UpdateCommand struct {
	ID            string   `json:"id"`
	DisplayName   *string  `json:"displayName,omitempty"`
	Description   *string  `json:"description,omitempty"`
	Permissions   []string `json:"permissions,omitempty"`
	ClientManaged *bool    `json:"clientManaged,omitempty"`
}

// UpdateRole mutates an existing role and emits RoleUpdated. Roles with
// source=CODE are immutable.
func UpdateRole(
	ctx context.Context,
	repo *role.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[RoleUpdated], error) {
	var zero commit.Committed[RoleUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.DisplayName != nil && strings.TrimSpace(*cmd.DisplayName) == "" {
		return zero, usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName cannot be empty")
	}

	r, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if r == nil {
		return zero, httperror.NotFound("Role", cmd.ID)
	}
	if r.Source == role.SourceCode {
		return zero, usecase.Conflict("CODE_ROLE_IMMUTABLE", "Roles with source=CODE cannot be modified")
	}
	if cmd.DisplayName != nil {
		r.DisplayName = strings.TrimSpace(*cmd.DisplayName)
	}
	if cmd.Description != nil {
		r.Description = cmd.Description
	}
	if cmd.Permissions != nil {
		r.Permissions = cmd.Permissions
	}
	if cmd.ClientManaged != nil {
		r.ClientManaged = *cmd.ClientManaged
	}

	event := RoleUpdated{
		Metadata: usecase.NewEventMetadata(ec, RoleUpdatedType, Source, subjectFor(r.ID)),
		RoleID:   r.ID,
		Name:     r.Name,
	}
	return commit.Save(ctx, uow, r, repo, event, cmd)
}
