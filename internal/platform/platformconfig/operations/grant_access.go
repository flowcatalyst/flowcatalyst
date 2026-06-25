package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// GrantAccessCommand is the input DTO.
type GrantAccessCommand struct {
	ApplicationCode string `json:"applicationCode"`
	RoleCode        string `json:"roleCode"`
	CanWrite        bool   `json:"canWrite"`
}

// GrantAccess creates or updates a platform-config access grant for a
// role and emits [AccessGranted]. The coarse anchor-only permission is
// enforced at the controller.
func GrantAccess(repo *platformconfig.Repository) usecaseop.Operation[GrantAccessCommand, AccessGranted] {
	return usecaseop.Operation[GrantAccessCommand, AccessGranted]{
		Name: "GrantAccess",
		Validate: func(_ context.Context, cmd GrantAccessCommand) error {
			if strings.TrimSpace(cmd.ApplicationCode) == "" {
				return usecase.Validation("APPLICATION_REQUIRED", "applicationCode is required")
			}
			if strings.TrimSpace(cmd.RoleCode) == "" {
				return usecase.Validation("ROLE_REQUIRED", "roleCode is required")
			}
			return nil
		},
		// Anchor-only authorization is enforced at the controller; there is no
		// per-resource authz dimension.
		Authorize: usecaseop.Public[GrantAccessCommand],
		Execute: func(ctx context.Context, cmd GrantAccessCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AccessGranted], error) {
			existing, err := repo.FindAccessByRole(ctx, cmd.ApplicationCode, cmd.RoleCode)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_access_by_role failed", err)
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
			return usecaseop.Save(a, newAccessRepo(repo), event), nil
		},
	}
}
