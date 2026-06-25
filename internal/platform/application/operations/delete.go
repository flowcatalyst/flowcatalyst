package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteApplication removes an application and emits [ApplicationDeleted].
//
// An application is platform-level (no tenant ClientID), so there is no
// resource-level access check; the coarse "may delete applications" permission
// (auth.CanDeleteApplications) is enforced at the controller.
func DeleteApplication(repo *application.Repository) usecaseop.Operation[DeleteCommand, ApplicationDeleted] {
	return usecaseop.Operation[DeleteCommand, ApplicationDeleted]{
		Name: "DeleteApplication",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationDeleted], error) {
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("Application", cmd.ID)
			}
			event := ApplicationDeleted{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationDeletedType, Source, subjectFor(a.ID)),
				ApplicationID: a.ID,
				Code:          a.Code,
			}
			return usecaseop.Delete(a, repo, event), nil
		},
	}
}
