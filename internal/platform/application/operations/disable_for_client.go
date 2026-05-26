package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DisableForClientCommand disables an application for a specific client.
// Idempotent — disabling an already-disabled config is a no-op state-wise
// but still emits the event for audit trail.
type DisableForClientCommand struct {
	ApplicationID string `json:"applicationId"`
	ClientID      string `json:"clientId"`
}

// DisableForClientUseCase implements UseCase.
type DisableForClientUseCase struct {
	configs *application.ClientConfigRepo
	uow     *usecasepgx.UnitOfWork
}

// NewDisableForClientUseCase wires the use case.
func NewDisableForClientUseCase(configs *application.ClientConfigRepo, uow *usecasepgx.UnitOfWork) *DisableForClientUseCase {
	return &DisableForClientUseCase{configs: configs, uow: uow}
}

func (uc *DisableForClientUseCase) Validate(_ context.Context, cmd DisableForClientCommand) error {
	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}
	return nil
}

func (uc *DisableForClientUseCase) Authorize(_ context.Context, _ DisableForClientCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DisableForClientUseCase) Execute(ctx context.Context, cmd DisableForClientCommand, ec usecase.ExecutionContext) usecase.Result[ApplicationDisabledForClient] {
	cfg, err := uc.configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ApplicationDisabledForClient](usecase.Internal("REPO", "find_config failed", err))
	}
	if cfg == nil {
		return usecase.Failure[ApplicationDisabledForClient](httperror.NotFound("ClientConfig",
			cmd.ApplicationID+":"+cmd.ClientID))
	}
	cfg.Disable()

	event := ApplicationDisabledForClient{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDisabledForClientType, Source, subjectFor(cmd.ApplicationID)),
		ApplicationID: cmd.ApplicationID,
		ClientID:      cmd.ClientID,
		ConfigID:      cfg.ID,
	}
	return usecasepgx.Commit[application.ClientConfig, ApplicationDisabledForClient, DisableForClientCommand](
		ctx, uc.uow, cfg, uc.configs, event, cmd,
	)
}

var _ usecase.UseCase[DisableForClientCommand, ApplicationDisabledForClient] = (*DisableForClientUseCase)(nil)
