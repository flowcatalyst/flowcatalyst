package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AssignRolesCommand replaces the service account's role list (declarative).
type AssignRolesCommand struct {
	ServiceAccountID string   `json:"serviceAccountId"`
	Roles            []string `json:"roles"`
}

// AssignRolesToServiceAccount replaces the role assignments wholesale and
// emits [ServiceAccountRolesAssigned] with the set-difference (added/removed).
func AssignRolesToServiceAccount(
	ctx context.Context,
	repo *serviceaccount.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AssignRolesCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ServiceAccountRolesAssigned], error) {
	var zero commit.Committed[ServiceAccountRolesAssigned]

	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return zero, usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}

	sa, err := repo.FindByID(ctx, cmd.ServiceAccountID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return zero, httperror.NotFound("ServiceAccount", cmd.ServiceAccountID)
	}

	// Set-difference for added/removed.
	currentSet := make(map[string]struct{}, len(sa.Roles))
	for _, ra := range sa.Roles {
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
	newAssignments := make([]serviceaccount.RoleAssignment, 0, len(cmd.Roles))
	for _, r := range cmd.Roles {
		newAssignments = append(newAssignments, serviceaccount.RoleAssignment{
			Role:       r,
			AssignedAt: now,
		})
	}
	sa.Roles = newAssignments
	sa.UpdatedAt = now

	event := ServiceAccountRolesAssigned{
		Metadata:         usecase.NewEventMetadata(ec, ServiceAccountRolesAssignedType, Source, subjectFor(sa.ID)),
		ServiceAccountID: sa.ID,
		RolesAdded:       added,
		RolesRemoved:     removed,
	}
	return commit.Save(ctx, uow, sa, repo, event, cmd)
}
