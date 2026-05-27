package operations

import (
	"context"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// AttachServiceAccount associates a service-account principal with an
// application and emits [ApplicationServiceAccountProvisionedEvent].
func AttachServiceAccount(
	ctx context.Context,
	repo *application.Repository,
	principals *principal.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd AttachServiceAccountCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationServiceAccountProvisionedEvent], error) {
	var zero commit.Committed[ApplicationServiceAccountProvisionedEvent]

	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return zero, usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ServiceAccountID) == "" {
		return zero, usecase.Validation("SERVICE_ACCOUNT_ID_REQUIRED", "Service account ID is required")
	}

	app, err := repo.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if app == nil {
		return zero, httperror.NotFound("Application", cmd.ApplicationID)
	}
	if app.ServiceAccountID != nil {
		return zero, usecase.BusinessRule(
			"APPLICATION_HAS_SERVICE_ACCOUNT",
			"Application already has a service account provisioned")
	}

	// app_applications.service_account_id has a FK to iam_principals.id
	// (migration 028) — not to iam_service_accounts.id. Resolve the SA's
	// linked principal id and store that.
	saPrincipal, err := principals.FindByServiceAccount(ctx, cmd.ServiceAccountID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if saPrincipal == nil {
		return zero, httperror.NotFound("ServiceAccountPrincipal", cmd.ServiceAccountID)
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
	return commit.Save(ctx, uow, app, repo, event, cmd)
}
