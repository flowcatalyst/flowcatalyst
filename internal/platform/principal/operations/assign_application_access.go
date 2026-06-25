package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type AssignApplicationAccessCommand struct {
	UserID         string   `json:"userId"`
	ApplicationIDs []string `json:"applicationIds"`
	// AllApplications, when non-nil, sets the principal's all-applications flag.
	// Nil leaves it unchanged.
	AllApplications *bool `json:"allApplications,omitempty"`
}

// AssignApplicationAccess replaces a principal's per-application access set and
// emits [ApplicationAccessAssigned].
//
// Resource-level authorization (requireUserAdmin against the loaded principal —
// the use-case move of the controller's auth.RequireUserAdmin +
// blockNonClientTarget) runs post-load in Execute. The application BOUNDING for
// non-anchor admins (assertAssignableApplications, preserved apps, the
// all-applications grant guard) is command-shaping the controller still performs
// before building the desired set.
func AssignApplicationAccess(repo *principal.Repository, applications *application.Repository) usecaseop.Operation[AssignApplicationAccessCommand, ApplicationAccessAssigned] {
	return usecaseop.Operation[AssignApplicationAccessCommand, ApplicationAccessAssigned]{
		Name: "AssignApplicationAccess",
		Validate: func(_ context.Context, cmd AssignApplicationAccessCommand) error {
			if strings.TrimSpace(cmd.UserID) == "" {
				return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[AssignApplicationAccessCommand],
		Execute: func(ctx context.Context, cmd AssignApplicationAccessCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationAccessAssigned], error) {
			p, err := repo.FindByID(ctx, cmd.UserID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.UserID)
			}
			if err := requireUserAdmin(ctx, p); err != nil {
				return nil, err
			}
			// Both users and service accounts carry per-application scope (the
			// all_applications flag + accessible application list live on the unified
			// principal). Service accounts in particular are the main case for confining
			// a principal to specific applications, so they must be assignable here —
			// only USER/SERVICE principals are.
			if p.Type != principal.TypeUser && p.Type != principal.TypeService {
				return nil, usecase.BusinessRule("NOT_ASSIGNABLE", "Application access can only be assigned to users or service accounts")
			}

			for _, appID := range cmd.ApplicationIDs {
				app, err := applications.FindByID(ctx, appID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_application failed", err)
				}
				if app == nil {
					return nil, usecase.Validation("APPLICATION_NOT_FOUND", "Application not found: "+appID)
				}
				if !app.Active {
					return nil, usecase.BusinessRule("APPLICATION_INACTIVE", "Application is not active: "+appID)
				}
			}

			added := stringDifference(cmd.ApplicationIDs, p.AccessibleApplicationIDs)
			removed := stringDifference(p.AccessibleApplicationIDs, cmd.ApplicationIDs)

			p.AccessibleApplicationIDs = append([]string(nil), cmd.ApplicationIDs...)
			if cmd.AllApplications != nil {
				p.AllApplications = *cmd.AllApplications
			}
			p.UpdatedAt = time.Now().UTC()

			event := ApplicationAccessAssigned{
				Metadata:       usecase.NewEventMetadata(ec, ApplicationAccessType, Source, subjectFor(p.ID)),
				UserID:         p.ID,
				ApplicationIDs: cmd.ApplicationIDs,
				Added:          added,
				Removed:        removed,
			}
			// AppAccessPersister rewrites the iam_principal_application_access
			// junction from p.AccessibleApplicationIDs in the same tx as the event;
			// the base principal Persist writes only the iam_principals row.
			return usecaseop.Save(p, principal.AppAccessPersister{Repository: repo}, event), nil
		},
	}
}
