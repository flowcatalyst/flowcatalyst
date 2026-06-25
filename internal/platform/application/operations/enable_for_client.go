package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// EnableForClientCommand enables an application for a specific client.
// Idempotent — re-enables an existing disabled config.
type EnableForClientCommand struct {
	ApplicationID string `json:"applicationId"`
	ClientID      string `json:"clientId"`
}

// EnableApplicationForClient ensures a client config row exists in the
// enabled state and emits [ApplicationEnabledForClient].
//
// This op binds an application TO A CLIENT, so the resource is the client:
// the use case enforces per-client access (auth.CheckScopeAccess on the
// target client). The coarse anchor requirement (auth.RequireAnchor) is
// enforced at the controller.
func EnableApplicationForClient(
	apps *application.Repository,
	clients *client.Repository,
	configs *application.ClientConfigRepo,
) usecaseop.Operation[EnableForClientCommand, ApplicationEnabledForClient] {
	return usecaseop.Operation[EnableForClientCommand, ApplicationEnabledForClient]{
		Name: "EnableApplicationForClient",
		Validate: func(_ context.Context, cmd EnableForClientCommand) error {
			if strings.TrimSpace(cmd.ApplicationID) == "" {
				return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd EnableForClientCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), &cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd EnableForClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationEnabledForClient], error) {
			app, err := apps.FindByID(ctx, cmd.ApplicationID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_application failed", err)
			}
			if app == nil {
				return nil, httperror.NotFound("Application", cmd.ApplicationID)
			}
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}

			existing, err := configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_existing_config failed", err)
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
			return usecaseop.Save(cfg, configs, event), nil
		},
	}
}
