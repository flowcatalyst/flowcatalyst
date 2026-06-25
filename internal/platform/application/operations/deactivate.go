package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeactivateCommand is the input DTO.
type DeactivateCommand struct {
	ID string `json:"id"`
}

// DeactivateApplication marks an application inactive and emits [ApplicationDeactivated].
//
// An application is platform-level (no tenant ClientID), so there is no
// resource-level access check; the coarse "may write applications" permission
// (auth.CanWriteApplications) is enforced at the controller.
func DeactivateApplication(repo *application.Repository) usecaseop.Operation[DeactivateCommand, ApplicationDeactivated] {
	return usecaseop.Operation[DeactivateCommand, ApplicationDeactivated]{
		Name: "DeactivateApplication",
		Validate: func(_ context.Context, cmd DeactivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeactivateCommand],
		Execute: func(ctx context.Context, cmd DeactivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationDeactivated], error) {
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("Application", cmd.ID)
			}
			a.Deactivate()
			event := ApplicationDeactivated{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationDeactivatedType, Source, subjectFor(a.ID)),
				ApplicationID: a.ID,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}
