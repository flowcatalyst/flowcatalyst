package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// EnableForClientCommand enables an application for a specific client.
// Idempotent — re-enables an existing disabled config.
type EnableForClientCommand struct {
	ApplicationID string `json:"applicationId"`
	ClientID      string `json:"clientId"`
}

// EnableApplicationForClient ensures a client config row exists in the
// enabled state and emits [ApplicationEnabledForClient].
func EnableApplicationForClient(
	ctx context.Context,
	apps *application.Repository,
	clients *client.Repository,
	configs *application.ClientConfigRepo,
	uow *usecasepgx.UnitOfWork,
	cmd EnableForClientCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ApplicationEnabledForClient], error) {
	var zero commit.Committed[ApplicationEnabledForClient]

	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return zero, usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return zero, usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}

	app, err := apps.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_application failed", err)
	}
	if app == nil {
		return zero, httperror.NotFound("Application", cmd.ApplicationID)
	}
	c, err := clients.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_client failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ClientID)
	}

	existing, err := configs.FindByApplicationAndClient(ctx, cmd.ApplicationID, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_existing_config failed", err)
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
	return commit.Save(ctx, uow, cfg, configs, event, cmd)
}
