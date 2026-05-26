package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// AssignApplicationAccessCommand replaces the user's accessible-application set.
type AssignApplicationAccessCommand struct {
	UserID         string   `json:"userId"`
	ApplicationIDs []string `json:"applicationIds"`
}

// AssignApplicationAccessUseCase implements UseCase.
type AssignApplicationAccessUseCase struct {
	principals   *principal.Repository
	applications *application.Repository
	uow          *usecasepgx.UnitOfWork
}

// NewAssignApplicationAccessUseCase wires the use case.
func NewAssignApplicationAccessUseCase(principals *principal.Repository, applications *application.Repository, uow *usecasepgx.UnitOfWork) *AssignApplicationAccessUseCase {
	return &AssignApplicationAccessUseCase{principals: principals, applications: applications, uow: uow}
}

func (uc *AssignApplicationAccessUseCase) Validate(_ context.Context, cmd AssignApplicationAccessCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	return nil
}

func (uc *AssignApplicationAccessUseCase) Authorize(_ context.Context, _ AssignApplicationAccessCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AssignApplicationAccessUseCase) Execute(ctx context.Context, cmd AssignApplicationAccessCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationAccessAssigned] {
	p, err := uc.principals.FindByID(ctx, cmd.UserID)
	if err != nil {
		return usecase.Failure[ApplicationAccessAssigned](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if p == nil {
		return usecase.Failure[ApplicationAccessAssigned](httperror.NotFound("User", cmd.UserID))
	}
	if p.Type != principal.TypeUser {
		return usecase.Failure[ApplicationAccessAssigned](usecase.BusinessRule("NOT_A_USER",
			"Principal is not a user"))
	}

	// Validate every requested application exists + is active.
	for _, appID := range cmd.ApplicationIDs {
		app, err := uc.applications.FindByID(ctx, appID)
		if err != nil {
			return usecase.Failure[ApplicationAccessAssigned](usecase.Internal("REPO", "find_application failed", err))
		}
		if app == nil {
			return usecase.Failure[ApplicationAccessAssigned](usecase.Validation("APPLICATION_NOT_FOUND",
				"Application not found: "+appID))
		}
		if !app.Active {
			return usecase.Failure[ApplicationAccessAssigned](usecase.BusinessRule("APPLICATION_INACTIVE",
				"Application is not active: "+appID))
		}
	}

	// Delta against current set.
	added := stringDifference(cmd.ApplicationIDs, p.AccessibleApplicationIDs)
	removed := stringDifference(p.AccessibleApplicationIDs, cmd.ApplicationIDs)

	// Replace wholesale. Defensive copy so the in-memory aggregate isn't
	// aliased by the caller-provided slice.
	p.AccessibleApplicationIDs = append([]string(nil), cmd.ApplicationIDs...)
	p.UpdatedAt = time.Now().UTC()

	event := ApplicationAccessAssigned{
		Metadata:       usecase.NewEventMetadata(ec, ApplicationAccessType, Source, subjectFor(p.ID)),
		UserID:         p.ID,
		ApplicationIDs: cmd.ApplicationIDs,
		Added:          added,
		Removed:        removed,
	}
	return usecasepgx.Commit[principal.Principal, ApplicationAccessAssigned, AssignApplicationAccessCommand](
		ctx, uc.uow, p, uc.principals, event, cmd,
	)
}

var _ usecase.UseCase[AssignApplicationAccessCommand, ApplicationAccessAssigned] = (*AssignApplicationAccessUseCase)(nil)
