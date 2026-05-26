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

// AttachServiceAccountCommand attaches a service account to an application.
// Called from the orchestrating provision_service_account handler.
type AttachServiceAccountCommand struct {
	ApplicationID      string `json:"applicationId"`
	ServiceAccountID   string `json:"serviceAccountId"`
	ServiceAccountCode string `json:"serviceAccountCode"`
}

// AttachServiceAccountUseCase implements UseCase.
type AttachServiceAccountUseCase struct {
	repo       *application.Repository
	principals *principal.Repository
	uow        *usecasepgx.UnitOfWork
}

// NewAttachServiceAccountUseCase wires the use case.
func NewAttachServiceAccountUseCase(repo *application.Repository, principals *principal.Repository, uow *usecasepgx.UnitOfWork) *AttachServiceAccountUseCase {
	return &AttachServiceAccountUseCase{repo: repo, principals: principals, uow: uow}
}

func (uc *AttachServiceAccountUseCase) Validate(_ context.Context, cmd AttachServiceAccountCommand) error {
	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}
	return nil
}

func (uc *AttachServiceAccountUseCase) Authorize(_ context.Context, _ AttachServiceAccountCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *AttachServiceAccountUseCase) Execute(ctx context.Context, cmd AttachServiceAccountCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationServiceAccountProvisionedEvent] {
	app, err := uc.repo.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return usecase.Failure[ApplicationServiceAccountProvisionedEvent](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if app == nil {
		return usecase.Failure[ApplicationServiceAccountProvisionedEvent](httperror.NotFound("Application", cmd.ApplicationID))
	}
	if app.ServiceAccountID != nil {
		return usecase.Failure[ApplicationServiceAccountProvisionedEvent](usecase.BusinessRule(
			"APPLICATION_HAS_SERVICE_ACCOUNT",
			"Application already has a service account provisioned"))
	}

	// app_applications.service_account_id has a FK to iam_principals.id
	// (migration 028) — not to iam_service_accounts.id. Resolve the SA's
	// linked principal id and store that.
	saPrincipal, err := uc.principals.FindByServiceAccount(ctx, cmd.ServiceAccountID)
	if err != nil {
		return usecase.Failure[ApplicationServiceAccountProvisionedEvent](usecase.Internal("REPO", "find_by_service_account failed", err))
	}
	if saPrincipal == nil {
		return usecase.Failure[ApplicationServiceAccountProvisionedEvent](httperror.NotFound("ServiceAccountPrincipal", cmd.ServiceAccountID))
	}
	app.ServiceAccountID = &saPrincipal.ID
	app.UpdatedAt = time.Now().UTC()

	event := ApplicationServiceAccountProvisionedEvent{
		Metadata:           usecase.NewEventMetadata(ec, ApplicationServiceAccountProvisioned, Source, subjectFor(app.ID)),
		ApplicationID:      app.ID,
		ApplicationCode:    app.Code,
		ServiceAccountID:   cmd.ServiceAccountID,
		ServiceAccountCode: cmd.ServiceAccountCode,
	}
	return usecasepgx.Commit[application.Application, ApplicationServiceAccountProvisionedEvent, AttachServiceAccountCommand](
		ctx, uc.uow, app, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[AttachServiceAccountCommand, ApplicationServiceAccountProvisionedEvent] = (*AttachServiceAccountUseCase)(nil)
