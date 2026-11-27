package portalidentity

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
)

type AppGrantCommand struct {
	ClientID    string `json:"clientId"`
	IdentityID  string `json:"identityId"`
	PortalAppID string `json:"portalAppId"`
}

// GrantApp gives an existing identity access to one of its client's portal
// apps and emits [IdentityAppGranted]. Idempotent.
func GrantApp(repo *Repository, apps *AppRepository) usecaseop.Operation[AppGrantCommand, IdentityAppGranted] {
	return usecaseop.Operation[AppGrantCommand, IdentityAppGranted]{
		Name:      "GrantPortalIdentityApp",
		Validate:  validateAppGrant,
		Authorize: usecaseop.Public[AppGrantCommand],
		Execute: func(ctx context.Context, cmd AppGrantCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityAppGranted], error) {
			ident, app, err := loadGrantTargets(ctx, repo, apps, cmd)
			if err != nil {
				return nil, err
			}
			ident.Grant(app.ID, SourceAdmin)
			event := IdentityAppGranted{
				Metadata:   usecase.NewEventMetadata(ec, IdentityAppGrantedType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID, ClientID: ident.ClientID,
				AppID: app.ID, AppCode: app.Code, Source_: string(SourceAdmin),
			}
			return usecaseop.Save(ident, repo, event), nil
		},
	}
}

// RevokeApp removes an identity's access to one portal app (the identity —
// and its access to the client's other portals — stays) and emits
// [IdentityAppRevoked]. Idempotent.
func RevokeApp(repo *Repository, apps *AppRepository) usecaseop.Operation[AppGrantCommand, IdentityAppRevoked] {
	return usecaseop.Operation[AppGrantCommand, IdentityAppRevoked]{
		Name:      "RevokePortalIdentityApp",
		Validate:  validateAppGrant,
		Authorize: usecaseop.Public[AppGrantCommand],
		Execute: func(ctx context.Context, cmd AppGrantCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityAppRevoked], error) {
			ident, app, err := loadGrantTargets(ctx, repo, apps, cmd)
			if err != nil {
				return nil, err
			}
			ident.Revoke(app.ID)
			event := IdentityAppRevoked{
				Metadata:   usecase.NewEventMetadata(ec, IdentityAppRevokedType, EventSource, subjectFor(ident.ID)),
				IdentityID: ident.ID, ClientID: ident.ClientID,
				AppID: app.ID, AppCode: app.Code,
			}
			return usecaseop.Save(ident, repo, event), nil
		},
	}
}

func validateAppGrant(_ context.Context, cmd AppGrantCommand) error {
	if strings.TrimSpace(cmd.ClientID) == "" || strings.TrimSpace(cmd.IdentityID) == "" ||
		strings.TrimSpace(cmd.PortalAppID) == "" {
		return usecase.Validation("TARGET_REQUIRED", "clientId, identityId and portalAppId are required")
	}
	return nil
}

func loadGrantTargets(ctx context.Context, repo *Repository, apps *AppRepository, cmd AppGrantCommand) (*Identity, *App, error) {
	ident, err := repo.FindByID(ctx, cmd.IdentityID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_identity failed", err)
	}
	if ident == nil || ident.ClientID != cmd.ClientID {
		return nil, nil, httperror.NotFound("PortalIdentity", cmd.IdentityID)
	}
	app, err := apps.FindByID(ctx, cmd.PortalAppID)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_portal_app failed", err)
	}
	if app == nil || app.ClientID != cmd.ClientID {
		return nil, nil, httperror.NotFound("PortalApp", cmd.PortalAppID)
	}
	return ident, app, nil
}

// loadClientApp resolves an app for a grant: it must exist, belong to the
// client, and be active.
func loadClientApp(ctx context.Context, apps *AppRepository, clientID, appID string) (*App, error) {
	if apps == nil {
		return nil, usecase.Internal("PORTAL_APPS", "portal app repo not wired", nil)
	}
	app, err := apps.FindByID(ctx, appID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_portal_app failed", err)
	}
	if app == nil || app.ClientID != clientID {
		return nil, httperror.NotFound("PortalApp", appID)
	}
	if !app.Active {
		return nil, usecase.Validation("PORTAL_APP_INACTIVE", "portal app '"+app.Code+"' is inactive")
	}
	return app, nil
}
