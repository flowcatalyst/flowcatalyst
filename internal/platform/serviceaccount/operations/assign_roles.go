package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AssignRolesCommand replaces the service account's role list (declarative).
type AssignRolesCommand struct {
	ServiceAccountID string   `json:"serviceAccountId"`
	Roles            []string `json:"roles"`
}

// AssignRolesUseCase implements UseCase.
type AssignRolesUseCase struct {
	repo *serviceaccount.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewAssignRolesUseCase wires the use case.
func NewAssignRolesUseCase(repo *serviceaccount.Repository, uow *usecasepgx.UnitOfWork) *AssignRolesUseCase {
	return &AssignRolesUseCase{repo: repo, uow: uow}
}

func (uc *AssignRolesUseCase) Validate(_ context.Context, cmd AssignRolesCommand) error {
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}
	return nil
}

func (uc *AssignRolesUseCase) Authorize(_ context.Context, _ AssignRolesCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AssignRolesUseCase) Execute(ctx context.Context, cmd AssignRolesCommand, ec usecase.ExecutionContext) usecase.Result[ServiceAccountRolesAssigned] {
	sa, err := uc.repo.FindByID(ctx, cmd.ServiceAccountID)
	if err != nil {
		return usecase.Failure[ServiceAccountRolesAssigned](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if sa == nil {
		return usecase.Failure[ServiceAccountRolesAssigned](httperror.NotFound("ServiceAccount", cmd.ServiceAccountID))
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
	return usecasepgx.Commit[serviceaccount.ServiceAccount, ServiceAccountRolesAssigned, AssignRolesCommand](
		ctx, uc.uow, sa, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[AssignRolesCommand, ServiceAccountRolesAssigned] = (*AssignRolesUseCase)(nil)
