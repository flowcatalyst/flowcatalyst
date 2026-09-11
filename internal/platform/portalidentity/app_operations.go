package portalidentity

import (
	"context"
	"net/url"
	"strings"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Portal-app operations are Authorize-Public for the same reason as the
// identity operations: the controller applies the client-delegable portal
// permission for the app's client.

type CreateAppCommand struct {
	ClientID    string  `json:"clientId"`
	Code        string  `json:"code"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

// CreateApp registers a portal app for a client and emits [AppChanged]
// (created). Codes are unique per client.
func CreateApp(apps *AppRepository, clients *client.Repository) usecaseop.Operation[CreateAppCommand, AppChanged] {
	return usecaseop.Operation[CreateAppCommand, AppChanged]{
		Name: "CreatePortalApp",
		Validate: func(_ context.Context, cmd CreateAppCommand) error {
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
			}
			if !ValidAppCode(NormalizeAppCode(cmd.Code)) {
				return usecase.Validation("CODE_INVALID",
					"code must be 1-100 lower-case letters, digits, '-' or '_', starting with a letter or digit")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateAppCommand],
		Execute: func(ctx context.Context, cmd CreateAppCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AppChanged], error) {
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}
			existing, err := apps.FindByClientAndCode(ctx, cmd.ClientID, cmd.Code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_portal_app failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS", "portal app code '"+existing.Code+"' already exists for this client")
			}
			app := NewApp(cmd.ClientID, cmd.Code, cmd.Name)
			app.Description = trimmedOrNil(cmd.Description)
			return usecaseop.Save(app, apps, appEvent(ec, AppCreatedType, app)), nil
		},
	}
}

type UpdateAppCommand struct {
	ClientID    string  `json:"clientId"`
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Active      *bool   `json:"active,omitempty"`
}

// UpdateApp renames/describes/(de)activates a portal app and emits
// [AppChanged] (updated). The code is immutable — portal apps are
// configured with it. An inactive app refuses logins and new grants.
func UpdateApp(apps *AppRepository) usecaseop.Operation[UpdateAppCommand, AppChanged] {
	return usecaseop.Operation[UpdateAppCommand, AppChanged]{
		Name: "UpdatePortalApp",
		Validate: func(_ context.Context, cmd UpdateAppCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateAppCommand],
		Execute: func(ctx context.Context, cmd UpdateAppCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AppChanged], error) {
			app, err := findClientApp(ctx, apps, cmd.ClientID, cmd.ID)
			if err != nil {
				return nil, err
			}
			if cmd.Name != nil {
				app.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.Description != nil {
				app.Description = trimmedOrNil(cmd.Description)
			}
			if cmd.Active != nil {
				app.Active = *cmd.Active
			}
			return usecaseop.Save(app, apps, appEvent(ec, AppUpdatedType, app)), nil
		},
	}
}

type DeleteAppCommand struct {
	ClientID string `json:"clientId"`
	ID       string `json:"id"`
}

// DeleteAppResult reports what went with the app.
type DeleteAppResult struct {
	AppID string
	// DeletedOAuthClientIDs are the OAuth client_ids that fronted the app.
	DeletedOAuthClientIDs []string
}

// DeleteApp removes a portal app, its grants (FK cascade), AND every OAuth
// client linked to it, in one transaction. The OAuth clients go too rather
// than being unlinked: an unlinked portal OAuth client is a legacy
// client-wide portal, so unlinking would silently WIDEN who can sign in
// through it.
func DeleteApp(apps *AppRepository, oauthClients *platformauth.OAuthClientRepo) usecaseop.TxOperation[DeleteAppCommand, DeleteAppResult] {
	return usecaseop.TxOperation[DeleteAppCommand, DeleteAppResult]{
		Name: "DeletePortalApp",
		Validate: func(_ context.Context, cmd DeleteAppCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteAppCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd DeleteAppCommand, ec usecase.ExecutionContext) (DeleteAppResult, error) {
			var zero DeleteAppResult
			app, err := findClientApp(ctx, apps, cmd.ClientID, cmd.ID)
			if err != nil {
				return zero, err
			}
			portalClients, err := oauthClients.FindByPortalClient(ctx, app.ClientID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_portal_oauth_clients failed", err)
			}
			res := DeleteAppResult{AppID: app.ID, DeletedOAuthClientIDs: []string{}}
			for n := range portalClients {
				oc := &portalClients[n]
				if oc.PortalAppID == nil || *oc.PortalAppID != app.ID {
					continue
				}
				if r := usecasepgx.CommitDeleteScoped(ctx, s, oc, oauthClients,
					authops.NewOAuthClientDeletedEvent(ec, oc.ID, oc.ClientID), cmd); !usecase.IsSuccess(r) {
					_, e := usecase.Into(r)
					return zero, e
				}
				res.DeletedOAuthClientIDs = append(res.DeletedOAuthClientIDs, oc.ClientID)
			}
			if r := usecasepgx.CommitDeleteScoped(ctx, s, app, apps, appEvent(ec, AppDeletedType, app), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return res, nil
		},
	}
}

func findClientApp(ctx context.Context, apps *AppRepository, clientID, id string) (*App, error) {
	app, err := apps.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_portal_app failed", err)
	}
	if app == nil || (clientID != "" && app.ClientID != clientID) {
		return nil, httperror.NotFound("PortalApp", id)
	}
	return app, nil
}

func appEvent(ec usecase.ExecutionContext, eventType string, app *App) AppChanged {
	return AppChanged{
		Metadata: usecase.NewEventMetadata(ec, eventType, EventSource, appSubjectFor(app.ID)),
		Type:     eventType,
		AppID:    app.ID, ClientID: app.ClientID, Code: app.Code, Name: app.Name,
	}
}

func trimmedOrNil(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	return &t
}

// ── create app + its OAuth client ────────────────────────────────────────

type CreateAppWithOAuthClientCommand struct {
	CreateAppCommand
	// RedirectURIs are the portal's OAuth callback URL(s) — registered on
	// the new OAuth client (/portal/authorize matches them exactly). May be
	// empty (added later on the OAuth client), but the portal cannot sign
	// anyone in until one is registered.
	RedirectURIs []string `json:"redirectUris,omitempty"`
	// ClientType is CONFIDENTIAL (default — a server-side portal holding a
	// secret) or PUBLIC (a browser-only portal; PKCE alone).
	ClientType string `json:"clientType,omitempty"`
}

// CreateAppWithOAuthClientResult carries the new ids and — for a
// CONFIDENTIAL client — the plaintext secret, returned exactly once (it is
// stored hashed).
type CreateAppWithOAuthClientResult struct {
	AppID            string
	OAuthClientRowID string
	OAuthClientID    string
	ClientType       string
	ClientSecret     string
}

// CreateAppWithOAuthClient registers a portal app AND provisions its portal
// OAuth client — flagged for the app's client, linked to the app,
// authorization_code only, PKCE required — in one transaction, so a portal
// is ready to wire up the moment it exists. Either both rows land or
// neither does.
func CreateAppWithOAuthClient(apps *AppRepository, clients *client.Repository, oauthClients *platformauth.OAuthClientRepo) usecaseop.TxOperation[CreateAppWithOAuthClientCommand, CreateAppWithOAuthClientResult] {
	createApp := CreateApp(apps, clients)
	return usecaseop.TxOperation[CreateAppWithOAuthClientCommand, CreateAppWithOAuthClientResult]{
		Name: "CreatePortalAppWithOAuthClient",
		Validate: func(ctx context.Context, cmd CreateAppWithOAuthClientCommand) error {
			if err := createApp.Validate(ctx, cmd.CreateAppCommand); err != nil {
				return err
			}
			if t := cmd.ClientType; t != "" {
				if _, ok := platformauth.ParseOAuthClientType(t); !ok {
					return usecase.Validation("INVALID_CLIENT_TYPE", "clientType must be PUBLIC or CONFIDENTIAL")
				}
			}
			for _, raw := range cmd.RedirectURIs {
				if !validRedirectURI(raw) {
					return usecase.Validation("REDIRECT_URI_INVALID",
						"redirectUris must be absolute http(s) URLs without wildcards: "+raw)
				}
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateAppWithOAuthClientCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CreateAppWithOAuthClientCommand, ec usecase.ExecutionContext) (CreateAppWithOAuthClientResult, error) {
			var zero CreateAppWithOAuthClientResult
			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return zero, httperror.NotFound("Client", cmd.ClientID)
			}
			existing, err := apps.FindByClientAndCode(ctx, cmd.ClientID, cmd.Code)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_portal_app failed", err)
			}
			if existing != nil {
				return zero, usecase.Conflict("CODE_EXISTS", "portal app code '"+existing.Code+"' already exists for this client")
			}

			app := NewApp(cmd.ClientID, cmd.Code, cmd.Name)
			app.Description = trimmedOrNil(cmd.Description)

			clientType := platformauth.OAuthClientConfidential
			if cmd.ClientType != "" {
				clientType, _ = platformauth.ParseOAuthClientType(cmd.ClientType)
			}
			oc := platformauth.NewOAuthClient(tsid.Generate(tsid.OAuthClient), app.Name+" (portal)", clientType)
			oc.RedirectURIs = trimmedURIs(cmd.RedirectURIs)
			oc.GrantTypes = []string{"authorization_code"} // portal logins never get refresh tokens
			oc.Scopes = []string{"openid", "profile", "email"}
			oc.PKCERequired = true
			owner, appID := app.ClientID, app.ID
			oc.PortalClientID, oc.PortalAppID = &owner, &appID
			var secret string
			if clientType == platformauth.OAuthClientConfidential {
				plaintext, ref, serr := authops.GenerateClientSecret()
				if serr != nil {
					return zero, usecase.Internal("SECRET", "generate client secret failed", serr)
				}
				oc.SetSecretRef(ref)
				secret = plaintext
			}

			if r := usecasepgx.CommitScoped(ctx, s, app, apps, appEvent(ec, AppCreatedType, app), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			if r := usecasepgx.CommitScoped(ctx, s, oc, oauthClients,
				authops.NewOAuthClientCreatedEvent(ec, oc.ID, oc.ClientID, oc.ClientName), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			return CreateAppWithOAuthClientResult{
				AppID: app.ID, OAuthClientRowID: oc.ID, OAuthClientID: oc.ClientID,
				ClientType: string(clientType), ClientSecret: secret,
			}, nil
		},
	}
}

// validRedirectURI accepts an absolute http(s) URL with a concrete host.
func validRedirectURI(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") &&
		u.Host != "" && !strings.Contains(u.Host, "*") && u.Fragment == ""
}

func trimmedURIs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		if t := strings.TrimSpace(raw); t != "" {
			out = append(out, t)
		}
	}
	return out
}
