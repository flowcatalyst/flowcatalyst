package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ActivateCommand is the input DTO.
type ActivateCommand struct {
	ID string `json:"id"`
}

// ActivateApplication marks an application active and emits [ApplicationActivated].
//
// An application is platform-level (no tenant ClientID), so there is no
// resource-level access check; the coarse "may write applications" permission
// (auth.CanWriteApplications) is enforced at the controller.
func ActivateApplication(repo *application.Repository) usecaseop.Operation[ActivateCommand, ApplicationActivated] {
	return usecaseop.Operation[ActivateCommand, ApplicationActivated]{
		Name: "ActivateApplication",
		Validate: func(_ context.Context, cmd ActivateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ActivateCommand],
		Execute: func(ctx context.Context, cmd ActivateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationActivated], error) {
			a, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if a == nil {
				return nil, httperror.NotFound("Application", cmd.ID)
			}
			a.Activate()
			event := ApplicationActivated{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationActivatedType, Source, subjectFor(a.ID)),
				ApplicationID: a.ID,
			}
			return usecaseop.Save(a, repo, event), nil
		},
	}
}
