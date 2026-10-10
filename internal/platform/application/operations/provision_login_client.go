package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ProvisionLoginClientCommand asks for an OAuth login client
// (authorization_code grant) for an application's frontend.
type ProvisionLoginClientCommand struct {
	ApplicationID string
	// ClientType is "CONFIDENTIAL" or anything else for PUBLIC (the default).
	ClientType   string
	RedirectURIs []string
}

// ProvisionLoginClientResult is what the caller shows once: the client id
// and, for a CONFIDENTIAL client, the plaintext secret.
type ProvisionLoginClientResult struct {
	ClientType   string
	RedirectURIs []string
	// OAuthClientRowID is the internal row id (oac_…), ClientID the public
	// client id.
	OAuthClientRowID string
	ClientID         string
	ClientSecret     string
}

// ProvisionLoginClient creates an OAuth client (authorization_code and
// refresh_token grants, openid/profile/email) named after the application
// and linked to it, through CreateOAuthClient. PUBLIC clients (SPAs, native
// apps) enforce PKCE and have no secret; a CONFIDENTIAL client's plaintext
// secret is returned exactly once. The caller makes the permission check.
// (Moved from the application API handler so the /ui page runs the same
// code; behaviour unchanged.)
func ProvisionLoginClient(ctx context.Context, uow *usecasepgx.UnitOfWork, apps *application.Repository, oauth *platformauth.OAuthClientRepo, cmd ProvisionLoginClientCommand, ec usecase.ExecutionContext) (ProvisionLoginClientResult, error) {
	if len(cmd.RedirectURIs) == 0 {
		return ProvisionLoginClientResult{}, usecase.Validation("REDIRECT_URIS_REQUIRED", "At least one redirect URI is required")
	}
	app, err := apps.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return ProvisionLoginClientResult{}, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if app == nil {
		return ProvisionLoginClientResult{}, httperror.NotFound("Application", cmd.ApplicationID)
	}

	clientType := "PUBLIC"
	if cmd.ClientType == "CONFIDENTIAL" {
		clientType = "CONFIDENTIAL"
	}
	publicClientID := tsid.Generate(tsid.OAuthClient)
	create := authops.CreateOAuthClientCommand{
		ClientID:     publicClientID,
		ClientName:   app.Name + " Login",
		ClientType:   clientType,
		RedirectURIs: cmd.RedirectURIs,
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
		// Link the login client to the application it was provisioned under
		// (oauth_client_application_ids), so it surfaces "under" the app in the
		// OAuth-client UI — mirroring the service-account client's app scoping.
		ApplicationIDs: []string{string(app.ID)},
	}
	event, err := usecaseop.Run(ctx, uow, authops.CreateOAuthClient(oauth), create, ec)
	if err != nil {
		return ProvisionLoginClientResult{}, err
	}

	// CreateOAuthClient generates the internal row id (`oac_…`) on the
	// entity; the created event carries it. CONFIDENTIAL clients stash a
	// once-readable plaintext secret.
	res := ProvisionLoginClientResult{ClientType: clientType, RedirectURIs: cmd.RedirectURIs, OAuthClientRowID: event.OAuthClientID, ClientID: publicClientID}
	if clientType == "CONFIDENTIAL" {
		if s, ok := authops.PopStashedSecret(res.OAuthClientRowID); ok {
			res.ClientSecret = s
		}
	}
	return res, nil
}
