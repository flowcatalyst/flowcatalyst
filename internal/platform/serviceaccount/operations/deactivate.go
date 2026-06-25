package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeactivateCommand is the input DTO.
type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateServiceAccount marks a service account inactive and emits
// [ServiceAccountDeactivated].
func DeactivateServiceAccount(repo *serviceaccount.Repository) usecaseop.Operation[DeactivateCommand, ServiceAccountDeactivated] {
	return usecaseop.Operation[DeactivateCommand, ServiceAccountDeactivated]{
		Name: "DeactivateServiceAccount",
		Validate: func(_ context.Context, cmd DeactivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// The coarse "may write service accounts" permission is enforced at the
		// controller; this admin-managed deactivate has no per-client resource
		// check, so the operation is intentionally open.
		Authorize: usecaseop.Public[DeactivateCommand],
		Execute: func(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountDeactivated], error) {
			sa, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ID)
			}
			sa.Deactivate()
			event := ServiceAccountDeactivated{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeactivatedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
			}
			return usecaseop.Save(sa, repo, event), nil
		},
	}
}
