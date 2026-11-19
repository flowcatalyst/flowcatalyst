package operations

import (
	"context"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appupdate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// AppLookup resolves an application code (*application.Repository).
type AppLookup interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// ReachApplication resolves an application code and checks the caller may act
// on its updates (auth.CheckAppUpdateReach). An unknown code is not found only
// to a caller who reaches every application; anyone else gets the same 403 as
// for an application they do not reach, so the answer is no existence oracle.
func ReachApplication(ctx context.Context, apps AppLookup, code string) (*application.Application, error) {
	ac := auth.FromContext(ctx)
	if !appupdate.ValidApplicationCode(code) {
		return nil, usecase.Validation("INVALID_APPLICATION_CODE", "applicationCode is not an application code")
	}
	app, err := apps.FindByCode(ctx, code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil {
		if ac != nil && ac.IsAnchor() && ac.AllApplications {
			return nil, httperror.NotFound("Application", code)
		}
		return nil, auth.CheckAppUpdateReach(ac, "\x00")
	}
	if err := auth.CheckAppUpdateReach(ac, string(app.ID)); err != nil {
		return nil, err
	}
	return app, nil
}

// ReachUpdate loads an update the caller may act on. Another application's
// update is not found, exactly like an id that does not exist.
func ReachUpdate(ctx context.Context, repo *appupdate.Repository, id string, withAssets bool) (*appupdate.Update, error) {
	if err := requireAnchor(ctx); err != nil {
		return nil, err
	}
	u, err := repo.FindByID(ctx, id, withAssets)
	if err != nil {
		return nil, usecase.Internal("REPO", "find update failed", err)
	}
	if u == nil || !auth.FromContext(ctx).CanAccessApplication(u.ApplicationID) {
		return nil, httperror.NotFound("AppUpdate", id)
	}
	return u, nil
}

// requireAnchor refuses a client-scoped caller before anything is looked up.
func requireAnchor(ctx context.Context) error {
	ac := auth.FromContext(ctx)
	if ac == nil || !ac.IsAnchor() {
		return usecase.Authorization("SCOPE_FORBIDDEN", "app updates need anchor scope")
	}
	return nil
}

// ReachInstallLink loads a link the caller may act on. Another application's
// link is not found, exactly like an id that does not exist.
func ReachInstallLink(ctx context.Context, repo *appupdate.LinkRepository, id string) (*appupdate.InstallLink, error) {
	if err := requireAnchor(ctx); err != nil {
		return nil, err
	}
	l, err := repo.FindLink(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find install link failed", err)
	}
	if l == nil || !auth.FromContext(ctx).CanAccessApplication(l.ApplicationID) {
		return nil, installLinkNotFound(id)
	}
	return l, nil
}

func installLinkNotFound(id string) error { return httperror.NotFound("AppInstallLink", id) }

// ReachBinary loads a binary the caller may read. Another application's
// binary is not found, exactly like an id that does not exist.
func ReachBinary(ctx context.Context, repo *appupdate.BinaryRepository, id string) (*appupdate.Binary, error) {
	if err := requireAnchor(ctx); err != nil {
		return nil, err
	}
	b, err := repo.FindBinary(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find binary failed", err)
	}
	if b == nil || !auth.FromContext(ctx).CanAccessApplication(b.ApplicationID) {
		return nil, httperror.NotFound("AppBinary", id)
	}
	return b, nil
}
