package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

// DisableApplicationForClient marks the client config disabled and emits
// [ApplicationDisabledForClient].
func DisableApplicationForClient(
	ctx context.Context,
	configs *application.ClientConfigRepo,
	uow *usecasepgx.UnitOfWork,
	cmd DisableForClientCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationDisabledForClient], error) {
	var zero commit.Committed[ApplicationDisabledForClient]

	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return zero, usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return zero, usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}

	cfg, err := configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_config failed", err)
	}
	if cfg == nil {
		return zero, httperror.NotFound("ClientConfig",
			cmd.ApplicationID+":"+cmd.ClientID)
	}
	cfg.Disable()

	event := ApplicationDisabledForClient{
		Metadata:      usecase.NewEventMetadata(ec, ApplicationDisabledForClientType, Source, subjectFor(cmd.ApplicationID)),
		ApplicationID: cmd.ApplicationID,
		ClientID:      cmd.ClientID,
		ConfigID:      cfg.ID,
	}
	return commit.Save(ctx, uow, cfg, configs, event, cmd)
}
