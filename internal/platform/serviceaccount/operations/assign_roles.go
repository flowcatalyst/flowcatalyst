package operations

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// AssignRolesCommand replaces the service account's role list (declarative).
type AssignRolesCommand struct {
	ServiceAccountID string   `json:"serviceAccountId"`
	Roles            []string `json:"roles"`
}

// serviceAccountRoleSource tags role rows written through the
// service-account admin endpoint, distinguishing them from IDP/SDK-sourced
// rows that may share the same principal.
const serviceAccountRoleSource = "ADMIN_ASSIGNED"

// AssignRolesToServiceAccount replaces the role assignments wholesale and
// emits [ServiceAccountRolesAssigned] with the set-difference (added/removed).
//
// A service account's roles live in iam_principal_roles keyed by its linked
// SERVICE principal — NOT the iam_service_accounts row — because that
// principal is what token minting reads (auth/provider.BuildClaims). So the
// write targets that principal via principal.RolesPersister, which rewrites
// the junction in the same transaction as the event. (The previous version
// mutated the service-account aggregate, whose Persist never touched the
// junction, so assignments were silently dropped.)
func AssignRolesToServiceAccount(
	saRepo *serviceaccount.Repository,
	principals *principal.Repository,
) usecaseop.Operation[AssignRolesCommand, ServiceAccountRolesAssigned] {
	return usecaseop.Operation[AssignRolesCommand, ServiceAccountRolesAssigned]{
		Name: "AssignRolesToServiceAccount",
		Validate: func(_ context.Context, cmd AssignRolesCommand) error {
			if strings.TrimSpace(cmd.ServiceAccountID) == "" {
				return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
			}
			return nil
		},
		// The coarse anchor-only permission is enforced at the controller; this
		// admin-managed assignment has no per-client resource check, so the
		// operation is intentionally open.
		Authorize: usecaseop.Public[AssignRolesCommand],
		Execute: func(ctx context.Context, cmd AssignRolesCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ServiceAccountRolesAssigned], error) {
			sa, err := saRepo.FindByID(ctx, cmd.ServiceAccountID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if sa == nil {
				return nil, httperror.NotFound("ServiceAccount", cmd.ServiceAccountID)
			}

			// Roles attach to the linked SERVICE principal. FindByServiceAccount
			// hydrates its current role set so we can diff + replace it.
			p, err := principals.FindByServiceAccount(ctx, cmd.ServiceAccountID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_service_account failed", err)
			}
			if p == nil {
				return nil, usecase.Internal("PRINCIPAL", "service account has no linked principal",
					errors.New("no SERVICE principal for service account "+cmd.ServiceAccountID))
			}

			// Set-difference for added/removed against the principal's current roles.
			currentSet := make(map[string]struct{}, len(p.Roles))
			for _, ra := range p.Roles {
				currentSet[ra.Role] = struct{}{}
			}
			newSet := make(map[string]struct{}, len(cmd.Roles))
			for _, r := range cmd.Roles {
				newSet[r] = struct{}{}
			}
			added := []string{}
			for r := range newSet {
				if _, ok := currentSet[r]; !ok {
					added = append(added, r)
				}
			}
			removed := []string{}
			for r := range currentSet {
				if _, ok := newSet[r]; !ok {
					removed = append(removed, r)
				}
			}

			// Replace role assignments wholesale.
			now := time.Now().UTC()
			src := serviceAccountRoleSource
			newAssignments := make([]serviceaccount.RoleAssignment, 0, len(cmd.Roles))
			for _, r := range cmd.Roles {
				newAssignments = append(newAssignments, serviceaccount.RoleAssignment{
					Role:             r,
					AssignmentSource: &src,
					AssignedAt:       now,
				})
			}
			p.Roles = newAssignments
			p.UpdatedAt = now

			event := ServiceAccountRolesAssigned{
				Metadata:         usecase.NewEventMetadata(ec, ServiceAccountRolesAssignedType, Source, subjectFor(sa.ID)),
				ServiceAccountID: sa.ID,
				RolesAdded:       added,
				RolesRemoved:     removed,
			}
			return usecaseop.Save(p, principal.RolesPersister{Repository: principals}, event), nil
		},
	}
}
