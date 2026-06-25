package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteServiceAccount removes a service account and emits [ServiceAccountDeleted].
func DeleteServiceAccount(repo *serviceaccount.Repository) usecaseop.Operation[DeleteCommand, ServiceAccountDeleted] {
	return usecaseop.Operation[DeleteCommand, ServiceAccountDeleted]{
		Name: "DeleteServiceAccount",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// The coarse "may delete service accounts" permission is enforced at the
		// controller; this admin-managed delete has no per-client resource
		// check, so the operation is intentionally open.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountDeleted], error) {
			sa, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ID)
			}
			event := ServiceAccountDeleted{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountDeletedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				Code:             sa.Code,
			}
			return usecaseop.Delete(sa, repo, event), nil
		},
	}
}
