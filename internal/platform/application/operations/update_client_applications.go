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

// UpdateClientApplicationsCommand replaces a client's full enabled-app
// set in one call. enabled_application_ids is the desired final state.
//
// Mirrors Rust crates/fc-platform/src/application/operations/update_client_applications.rs.
type UpdateClientApplicationsCommand struct {
	ClientID              string   `json:"clientId"`
	EnabledApplicationIDs []string `json:"enabledApplicationIds"`
}

// UpdateClientApplications computes the diff between the client's current
// configs and the requested desired set; for newly-desired apps it flips
// an existing disabled row to enabled (or creates a fresh enabled row);
// for currently-enabled apps not in the desired set it flips to disabled.
// All persistence + the rollup [ClientApplicationsUpdated] event happen
// in one transaction.
//
// This op binds applications TO A CLIENT, so the resource is the client:
// the use case enforces per-client access (auth.CheckScopeAccess on the
// target client). The coarse permission is enforced at the controller.
func UpdateClientApplications(
	apps *application.Repository,
	clients *client.Repository,
	configs *application.ClientConfigRepo,
) usecaseop.Operation[UpdateClientApplicationsCommand, ClientApplicationsUpdated] {
	return usecaseop.Operation[UpdateClientApplicationsCommand, ClientApplicationsUpdated]{
		Name: "UpdateClientApplications",
		Validate: func(_ context.Context, cmd UpdateClientApplicationsCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
			}
			return nil
		},
		Authorize: func(ctx context.Context, cmd UpdateClientApplicationsCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), &cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd UpdateClientApplicationsCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientApplicationsUpdated], error) {
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}

			// Every requested app must exist before we touch any rows.
			for _, appID := range cmd.EnabledApplicationIDs {
				if strings.TrimSpace(appID) == "" {
					return nil, usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID must be non-empty")
				}
				app, err := apps.FindByID(ctx, appID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_application failed", err)
				}
				if app == nil {
					return nil, httperror.NotFound("Application", appID)
				}
			}

			current, err := configs.FindByClient(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_configs failed", err)
			}

			currentByApp := make(map[string]*application.ClientConfig, len(current))
			currentlyEnabled := make(map[string]struct{}, len(current))
			for i := range current {
				row := current[i]
				currentByApp[row.ApplicationID] = &row
				if row.Enabled {
					currentlyEnabled[row.ApplicationID] = struct{}{}
				}
			}

			desired := make(map[string]struct{}, len(cmd.EnabledApplicationIDs))
			for _, id := range cmd.EnabledApplicationIDs {
				desired[id] = struct{}{}
			}

			var toPersist []application.ClientConfig
			var enabledAdded []string
			var disabledRemoved []string

			// Enable: requested but not currently enabled. Flip an existing
			// disabled row, or create a fresh one.
			for _, appID := range cmd.EnabledApplicationIDs {
				if _, on := currentlyEnabled[appID]; on {
					continue
				}
				if existing, ok := currentByApp[appID]; ok {
					existing.Enable()
					toPersist = append(toPersist, *existing)
				} else {
					fresh := application.NewClientConfig(appID, cmd.ClientID)
					toPersist = append(toPersist, *fresh)
				}
				enabledAdded = append(enabledAdded, appID)
			}

			// Disable: currently enabled but not in desired set.
			for appID := range currentlyEnabled {
				if _, want := desired[appID]; want {
					continue
				}
				existing := currentByApp[appID]
				existing.Disable()
				toPersist = append(toPersist, *existing)
				disabledRemoved = append(disabledRemoved, appID)
			}

			event := ClientApplicationsUpdated{
				Metadata:        usecase.NewEventMetadata(ec, ClientApplicationsUpdatedType, Source, "platform.client."+cmd.ClientID),
				ClientID:        cmd.ClientID,
				EnabledIDs:      append([]string(nil), cmd.EnabledApplicationIDs...),
				EnabledAdded:    enabledAdded,
				DisabledRemoved: disabledRemoved,
			}

			if len(toPersist) == 0 {
				// Empty diff still emits the rollup so the audit trail records
				// the request. Matches Rust's behaviour.
				return usecaseop.Emit(event), nil
			}
			return usecaseop.SaveAll(toPersist, configs, event), nil
		},
	}
}
