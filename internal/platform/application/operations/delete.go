package operations

import (
	"context"
	"fmt"
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
//
// The delete is refused (409 APPLICATION_HAS_REFERENCES) while anything still
// references the application: access grants, enabled per-client configs,
// service accounts, application-scoped roles or principals' application refs.
// None of those columns has a foreign key, so deleting would leave them
// dangling (owner decision #53, matching Rust). A disabled client config does
// not block the delete; the repository removes it in the same transaction as
// the application row (owner decision #55).
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
			refs, err := repo.CountReferences(ctx, a.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "count_references failed", err)
			}
			if blockers := referenceBlockers(refs); len(blockers) > 0 {
				return nil, usecase.Conflict("APPLICATION_HAS_REFERENCES",
					fmt.Sprintf("Cannot delete application '%s' — %s still reference it. Remove those before deleting.",
						a.Code, strings.Join(blockers, ", ")))
			}
			event := ApplicationDeleted{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationDeletedType, Source, subjectFor(string(a.ID))),
				ApplicationID: string(a.ID),
				Code:          a.Code,
			}
			return usecaseop.Delete(a, repo, event), nil
		},
	}
}

// referenceBlockers names each non-zero reference count, in Rust's order and
// wording ("2 access grants"), for the APPLICATION_HAS_REFERENCES message.
func referenceBlockers(refs application.References) []string {
	counts := []struct {
		label string
		n     int64
	}{
		{"access grants", refs.AccessGrants},
		{"client configs", refs.ClientConfigs},
		{"service accounts", refs.ServiceAccounts},
		{"application roles", refs.Roles},
		{"principal refs", refs.PrincipalRefs},
	}
	var blockers []string
	for _, c := range counts {
		if c.n > 0 {
			blockers = append(blockers, fmt.Sprintf("%d %s", c.n, c.label))
		}
	}
	return blockers
}
