package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// CreateRole creates a new role and emits RoleCreated.
func CreateRole(
	ctx context.Context,
	repo *role.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[RoleCreated], error) {
	var zero commit.Committed[RoleCreated]

	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return zero, usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
	}
	if strings.TrimSpace(cmd.RoleName) == "" {
		return zero, usecase.Validation("ROLE_NAME_REQUIRED", "roleName is required")
	}
	if strings.TrimSpace(cmd.DisplayName) == "" {
		return zero, usecase.Validation("DISPLAY_NAME_REQUIRED", "displayName is required")
	}

	fullName := cmd.ApplicationCode + ":" + cmd.RoleName
	existing, err := repo.FindByName(ctx, fullName)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_name failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("ROLE_EXISTS", "Role '"+fullName+"' already exists")
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
	return commit.Save(ctx, uow, r, repo, event, cmd)
}
