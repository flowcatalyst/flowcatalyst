package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// RevokeAccessCommand is the input DTO.
type RevokeAccessCommand struct {
	ID string `json:"id"`
}

// RevokeAccess removes a platform-config access grant and emits [AccessRevoked].
// The coarse anchor-only permission is enforced at the controller.
func RevokeAccess(repo *platformconfig.Repository) usecaseop.Operation[RevokeAccessCommand, AccessRevoked] {
	return usecaseop.Operation[RevokeAccessCommand, AccessRevoked]{
		Name: "RevokeAccess",
		Validate: func(_ context.Context, cmd RevokeAccessCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// Anchor-only authorization is enforced at the controller; there is no
		// per-resource authz dimension.
		Authorize: usecaseop.Public[RevokeAccessCommand],
		Execute: func(ctx context.Context, cmd RevokeAccessCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AccessRevoked], error) {
			a, err := repo.FindAccessByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_access_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("PlatformConfigAccess", cmd.ID)
			}
			event := AccessRevoked{
				Metadata:        usecase.NewEventMetadata(ec, AccessRevokedType, Source, subjectFor(a.ID)),
				AccessID:        a.ID,
				ApplicationCode: a.ApplicationCode,
				RoleCode:        a.RoleCode,
			}
			return usecaseop.Delete(a, newAccessRepo(repo), event), nil
		},
	}
}
