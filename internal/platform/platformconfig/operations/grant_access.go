package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// GrantAccessCommand is the input DTO.
type GrantAccessCommand struct {
	ApplicationCode string `json:"applicationCode"`
	RoleCode        string `json:"roleCode"`
	CanWrite        bool   `json:"canWrite"`
}

// GrantAccess creates or updates a platform-config access grant for a
// role and emits [AccessGranted].
func GrantAccess(
	ctx context.Context,
	repo *platformconfig.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd GrantAccessCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AccessGranted], error) {
	var zero commit.Committed[AccessGranted]

	if strings.TrimSpace(cmd.ApplicationCode) == "" {
		return zero, usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
	}
	if strings.TrimSpace(cmd.RoleCode) == "" {
		return zero, usecase.Validation("ROLE_REQUIRED", "roleCode is required")
	}

	existing, err := repo.FindAccessByRole(ctx, cmd.ApplicationCode, cmd.RoleCode)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_access_by_role failed", err)
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
	return commit.Save(ctx, uow, a, newAccessRepo(repo), event, cmd)
}
