package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code string `json:"code"`
	// ApplicationCode optionally links the connection to a registered
	// application. Omitted means "shared" — usable from any application (not
	// to be confused with ClientID nil, which this codebase calls global).
	ApplicationCode  *string `json:"applicationCode,omitempty"`
	Name             string  `json:"name"`
	Description      *string `json:"description,omitempty"`
	ServiceAccountID string  `json:"serviceAccountId"`
	ExternalID       *string `json:"externalId,omitempty"`
	ClientID         *string `json:"clientId,omitempty"`
}

// CreateConnection validates cmd, enforces anchor-only authorization and
// (applicationCode, clientID, code) uniqueness, persists the connection, and
// emits [ConnectionCreated]. A connection created through this UI/API path
// is source UI (the entity default; mirrors subscription.New/CreateSubscription).
//
// TODO(wave-3c): validate that ServiceAccountID exists once
// service_account is ported.
func CreateConnection(repo *connection.Repository, apps *application.Repository) usecaseop.Operation[CreateCommand, ConnectionCreated] {
	return usecaseop.Operation[CreateCommand, ConnectionCreated]{
		Name: "CreateConnection",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "Connection code is required")
			}
			if !validate.CodePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"Code must start with lowercase letter, contain only lowercase alphanumeric and hyphens")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "Connection name is required")
			}
			if strings.TrimSpace(cmd.ServiceAccountID) == "" {
				return usecase.Validation("SERVICE_ACCOUNT_REQUIRED", "serviceAccountId is required")
			}
			return nil
		},
		// Resource-level authorization (the coarse "may create connections"
		// permission is enforced at the controller). A connection bound to a
		// client may only be created by a principal with access to that client;
		// a platform-wide connection (nil ClientID) requires anchor. This is
		// exactly auth.CheckScopeAccess on the target client.
		Authorize: func(ctx context.Context, cmd CreateCommand) error {
			return auth.CheckScopeAccess(auth.FromContext(ctx), cmd.ClientID)
		},
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ConnectionCreated], error) {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))

			if cmd.ApplicationCode != nil {
				app, err := apps.FindByCode(ctx, *cmd.ApplicationCode)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_application_by_code failed", err)
				}
				if app == nil {
					return nil, httperror.NotFound("Application", *cmd.ApplicationCode)
				}
				if err := requireApplicationAccess(ctx, app); err != nil {
					return nil, err
				}
			}

			existing, err := repo.FindByCode(ctx, code, cmd.ApplicationCode, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS",
					"Connection with code '"+code+"' already exists")
			}

			c := connection.New(code, strings.TrimSpace(cmd.Name), cmd.ServiceAccountID)
			c.ApplicationCode = cmd.ApplicationCode
			c.Description = cmd.Description
			c.ExternalID = cmd.ExternalID
			c.ClientID = cmd.ClientID

			event := ConnectionCreated{
				Metadata:     usecase.NewEventMetadata(ec, ConnectionCreatedType, Source, subjectFor(c.ID)),
				ConnectionID: c.ID,
				Code:         c.Code,
				Name:         c.Name,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// requireApplicationAccess enforces that the caller may act on the named
// application before a connection is linked to it. Create/update never
// checked this before this addition — only the target CLIENT's scope was
// enforced (CheckScopeAccess), so a client-scoped caller holding the
// connection-create/update permission could link a connection to an
// application it has no access to at all.
func requireApplicationAccess(ctx context.Context, app *application.Application) error {
	if !auth.FromContext(ctx).CanAccessApplication(app.ID) {
		return httperror.Forbidden("Not authorised for application '" + app.Code + "'")
	}
	return nil
}
