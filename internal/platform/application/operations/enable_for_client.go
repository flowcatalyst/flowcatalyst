package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// EnableForClientCommand enables an application for a specific client.
// Idempotent — re-enables an existing disabled config.
type EnableForClientCommand struct {
	ApplicationID string `json:"applicationId"`
	ClientID      string `json:"clientId"`
}

// EnableForClientUseCase implements UseCase.
type EnableForClientUseCase struct {
	apps    *application.Repository
	clients *client.Repository
	configs *application.ClientConfigRepo
	uow     *usecasepgx.UnitOfWork
}

// NewEnableForClientUseCase wires the use case.
func NewEnableForClientUseCase(apps *application.Repository, clients *client.Repository, configs *application.ClientConfigRepo, uow *usecasepgx.UnitOfWork) *EnableForClientUseCase {
	return &EnableForClientUseCase{apps: apps, clients: clients, configs: configs, uow: uow}
}

func (uc *EnableForClientUseCase) Validate(_ context.Context, cmd EnableForClientCommand) error {
	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}
	return nil
}

func (uc *EnableForClientUseCase) Authorize(_ context.Context, _ EnableForClientCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *EnableForClientUseCase) Execute(ctx context.Context, cmd EnableForClientCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationEnabledForClient] {
	app, err := uc.apps.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return usecase.Failure[ApplicationEnabledForClient](usecase.Internal("REPO", "find_application failed", err))
	}
	if app == nil {
		return usecase.Failure[ApplicationEnabledForClient](httperror.NotFound("Application", cmd.ApplicationID))
	}
	c, err := uc.clients.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ApplicationEnabledForClient](usecase.Internal("REPO", "find_client failed", err))
	}
	if c == nil {
		return usecase.Failure[ApplicationEnabledForClient](httperror.NotFound("Client", cmd.ClientID))
	}

	existing, err := uc.configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ApplicationEnabledForClient](usecase.Internal("REPO", "find_existing_config failed", err))
	}
	var cfg *application.ClientConfig
	if existing != nil {
		existing.Enable()
		cfg = existing
	} else {
		cfg = application.NewClientConfig(cmd.ApplicationID, cmd.ClientID)
	}

	event := ApplicationEnabledForClient{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationEnabledForClientType, Source, subjectFor(app.ID)),
		ApplicationID: cmd.ApplicationID,
		ClientID:      cmd.ClientID,
		ConfigID:      cfg.ID,
	}
	return usecasepgx.Commit[application.ClientConfig, ApplicationEnabledForClient, EnableForClientCommand](
		ctx, uc.uow, cfg, uc.configs, event, cmd,
	)
}

var _ usecase.UseCase[EnableForClientCommand, ApplicationEnabledForClient] = (*EnableForClientUseCase)(nil)
