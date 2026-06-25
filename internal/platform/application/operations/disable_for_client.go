package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DisableForClientCommand disables an application for a specific client.
// Idempotent — disabling an already-disabled config is a no-op state-wise
// but still emits the event for audit trail.
type DisableForClientCommand struct {
	ApplicationID string `json:"applicationId"`
	ClientID      string `json:"clientId"`
}

// DisableApplicationForClient marks the client config disabled and emits
// [ApplicationDisabledForClient].
//
// This op binds an application TO A CLIENT, so the resource is the client:
// the use case enforces per-client access (auth.CheckScopeAccess on the
// target client). The coarse anchor requirement (auth.RequireAnchor) is
// enforced at the controller.
func DisableApplicationForClient(configs *application.ClientConfigRepo) usecaseop.Operation[DisableForClientCommand, ApplicationDisabledForClient] {
	return usecaseop.Operation[DisableForClientCommand, ApplicationDisabledForClient]{
		Name: "DisableApplicationForClient",
		Validate: func(_ context.Context, cmd DisableForClientCommand) error {
			if strings.TrimSpace(cmd.ApplicationID) == "" {
				return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd DisableForClientCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), &cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd DisableForClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ApplicationDisabledForClient], error) {
			cfg, err := configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_config failed", err)
			}
			if cfg == nil {
				return nil, httperror.NotFound("ClientConfig",
					cmd.ApplicationID+":"+cmd.ClientID)
			}
			cfg.Disable()

			event := ApplicationDisabledForClient{
				Metadata:      usecase.NewEventMetadata(ec, ApplicationDisabledForClientType, Source, subjectFor(cmd.ApplicationID)),
				ApplicationID: cmd.ApplicationID,
				ClientID:      cmd.ClientID,
				ConfigID:      cfg.ID,
			}
			return usecaseop.Save(cfg, configs, event), nil
		},
	}
}
