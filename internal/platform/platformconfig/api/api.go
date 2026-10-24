// Package api wires HTTP routes for platform_config via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// State bundles deps. Svc holds the access rules the UI shares.
type State struct {
	Svc *platformconfig.Service
}

const tag = "platform-config"

// Register mounts the platform_config endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listPlatformConfigProperties", "/api/platform-config/{app}", "List platform-config properties for an application", s.listProperties)
	apiroute.Get(g, "getPlatformConfigProperty", "/api/config/{app}/{section}/{property}", "Get a single platform-config property", s.getProperty)
	// The SPA calls /api/config/...; the legacy /api/platform-config/...
	// path is kept for the list/access admin routes below.
	apiroute.Get(g, "revealPlatformConfigSecret", "/api/config/{app}/{section}/{property}/secret",
		"Read a SECRET platform-config value in plaintext (audited; the only endpoint that returns one)", s.revealSecret)
	apiroute.Put(g, "setPlatformConfigProperty", "/api/config/{app}/{section}/{property}", "Set a platform-config property", http.StatusOK, s.setProperty)
	apiroute.Delete(g, "deletePlatformConfigProperty", "/api/config/{app}/{section}/{property}", "Delete a platform-config property", http.StatusNoContent, s.deleteProperty)
	apiroute.Get(g, "listPlatformConfigAccess", "/api/platform-config/{app}/access", "List access grants for an application", s.listAccess)
	apiroute.Post(g, "grantPlatformConfigAccess", "/api/platform-config/{app}/access", "Grant access to platform-config for a role", http.StatusCreated, s.grantAccess)
	apiroute.Delete(g, "revokePlatformConfigAccess", "/api/platform-config/access/{id}", "Revoke a platform-config access grant", http.StatusNoContent, s.revokeAccess)
}

type listPropsInput struct {
	App string `path:"app"`
}

func (s *State) listProperties(ctx context.Context, in *listPropsInput) (*apicommon.Out[ConfigListResponse], error) {
	rows, err := s.Svc.List(ctx, auth.FromContext(ctx), in.App)
	if err != nil {
		return nil, err
	}
	out := make([]ConfigResponse, 0, len(rows))
	for i := range rows {
		out = append(out, configFromEntity(&rows[i]))
	}
	return &apicommon.Out[ConfigListResponse]{Body: ConfigListResponse{Items: out}}, nil
}

type propertyInput struct {
	App      string `path:"app"`
	Section  string `path:"section"`
	Property string `path:"property"`
	ClientID string `query:"clientId" doc:"Scope to one client: the client's id (clt_...) or its identifier (e.g. acme). The platform stores the id, so both name the same value. Omit for the application's global value. An unknown client is 404."`
}

func (s *State) getProperty(ctx context.Context, in *propertyInput) (*apicommon.Out[ConfigResponse], error) {
	ac := auth.FromContext(ctx)
	if err := s.Svc.AuthorizeRead(ctx, ac, in.App); err != nil {
		return nil, err
	}
	cid, err := s.Svc.ResolveClientID(ctx, in.ClientID)
	if err != nil {
		return nil, err
	}
	scope, clientID := platformconfig.ScopeFor(cid)
	c, err := s.Svc.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", in.App+"/"+in.Section+"/"+in.Property)
	}
	return &apicommon.Out[ConfigResponse]{Body: configFromEntity(c)}, nil
}

type setPropertyInput struct {
	App      string `path:"app"`
	Section  string `path:"section"`
	Property string `path:"property"`
	ClientID string `query:"clientId" doc:"Scope to one client: the client's id (clt_...) or its identifier (e.g. acme); the platform stores the id. Omit for the application's global value. An unknown client is 404."`
	Body     SetPropertyRequest
}

func (s *State) setProperty(ctx context.Context, in *setPropertyInput) (*apicommon.Out[ConfigResponse], error) {
	// Gate before anything is looked up, so an unauthorised caller cannot probe
	// which clients exist (the use case checks again, and decides for callers
	// below anchor tier by their access grants).
	if err := s.Svc.AuthorizeUpdate(ctx, auth.FromContext(ctx), in.App); err != nil {
		return nil, err
	}
	// The SPA passes clientId as a query param; the body carries only
	// value/valueType/description. Fold the query value into the command.
	if in.ClientID != "" {
		cid := in.ClientID
		in.Body.ClientID = &cid
	}
	// Canonicalise the client (id or identifier) before anything is written;
	// an unknown client is refused here, not stored.
	var clientRef string
	if in.Body.ClientID != nil {
		clientRef = *in.Body.ClientID
	}
	resolved, err := s.Svc.ResolveClientID(ctx, clientRef)
	if err != nil {
		return nil, err
	}
	if resolved == "" {
		in.Body.ClientID = nil
	} else {
		in.Body.ClientID = &resolved
	}
	// Authorization (anchor, or non-anchor with write access to the app) runs
	// inside the use case's Authorize phase.
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.Svc.UoW, operations.SetProperty(s.Svc.Repo), in.Body.toCommand(in.App, in.Section, in.Property), ec); err != nil {
		return nil, err
	}
	scope, clientID := platformconfig.ScopeFor(resolved)
	c, err := s.Svc.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, usecase.Internal("REPO", "config missing after set", nil)
	}
	return &apicommon.Out[ConfigResponse]{Body: configFromEntity(c)}, nil
}

func (s *State) deleteProperty(ctx context.Context, in *propertyInput) (*apicommon.Empty, error) {
	if err := s.Svc.Delete(ctx, auth.FromContext(ctx), in.App, in.Section, in.Property, in.ClientID); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type listAccessInput struct {
	App string `path:"app"`
}

func (s *State) listAccess(ctx context.Context, in *listAccessInput) (*apicommon.Out[AccessListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadPlatformConfig(ac); err != nil {
		return nil, err
	}
	if err := s.Svc.ConfineApp(ctx, ac, in.App); err != nil {
		return nil, err
	}
	rows, err := s.Svc.Repo.FindAccessByApplication(ctx, in.App)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_access_by_application failed", err)
	}
	out := apicommon.MapSlice(rows, accessFromEntity)
	return &apicommon.Out[AccessListResponse]{Body: AccessListResponse{Items: out}}, nil
}

type grantAccessInput struct {
	App  string `path:"app"`
	Body GrantAccessRequest
}

func (s *State) grantAccess(ctx context.Context, in *grantAccessInput) (*apicommon.Out[apicommon.CreatedResponse], error) {
	// Coarse anchor-only authorization at the controller.
	if err := auth.CanUpdatePlatformConfig(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if err := s.Svc.ConfineApp(ctx, auth.FromContext(ctx), in.App); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.Svc.UoW, operations.GrantAccess(s.Svc.Repo), in.Body.toCommand(in.App), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.AccessID}}, nil
}

func (s *State) revokeAccess(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	// Coarse anchor-only authorization at the controller.
	if err := auth.CanUpdatePlatformConfig(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	// A grant names no application in the request, so an application-scoped
	// principal (which may grant only for its own application) may not revoke.
	if ac := auth.FromContext(ctx); ac.IsAnchor() && ac.IsApplicationScoped() {
		return nil, httperror.Forbidden("Not authorised to revoke platform config access")
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.Svc.UoW, operations.RevokeAccess(s.Svc.Repo), operations.RevokeAccessCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type revealOutput struct {
	// CacheControl keeps the plaintext out of every shared or browser cache.
	CacheControl string `header:"Cache-Control"`
	Body         SecretResponse
}

// revealSecret returns a SECRET's plaintext: platformconfig.Service.Reveal's
// rules (permission, confinement, audit-or-refuse), never cached.
func (s *State) revealSecret(ctx context.Context, in *propertyInput) (*revealOutput, error) {
	r, err := s.Svc.Reveal(ctx, auth.FromContext(ctx), in.App, in.Section, in.Property, in.ClientID)
	if err != nil {
		return nil, err
	}
	c := r.Config
	return &revealOutput{
		CacheControl: "no-store",
		Body: SecretResponse{
			ApplicationCode: c.ApplicationCode,
			Section:         c.Section,
			Property:        c.Property,
			ClientID:        ids.StringPtr(c.ClientID),
			Value:           r.Value,
			UpdatedAt:       jsontime.New(c.UpdatedAt),
		},
	}, nil
}
