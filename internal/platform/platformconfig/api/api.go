// Package api wires HTTP routes for platform_config via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *platformconfig.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "platform-config"

// Register mounts the platform_config endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listPlatformConfigProperties", "/api/platform-config/{app}", "List platform-config properties for an application", s.listProperties)
	apiroute.Get(g, "getPlatformConfigProperty", "/api/config/{app}/{section}/{property}", "Get a single platform-config property", s.getProperty)
	// The SPA calls /api/config/...; the legacy /api/platform-config/...
	// path is kept for the list/access admin routes below.
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
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, in.App, ac.Roles, false)
		if err != nil {
			return nil, usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return nil, httperror.Forbidden("No read access to platform config for " + in.App)
		}
	}
	rows, err := s.Repo.FindConfigsByApplication(ctx, in.App)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_configs_by_application failed", err)
	}
	out := make([]ConfigResponse, 0, len(rows))
	for i := range rows {
		c := rows[i]
		if !ac.IsAnchor() && c.ValueType == platformconfig.ValueSecret {
			c.Value = "***"
		}
		out = append(out, configFromEntity(&c))
	}
	return &apicommon.Out[ConfigListResponse]{Body: ConfigListResponse{Items: out}}, nil
}

// scopeFor derives the config scope from an optional clientId, matching the
// set-property operation's own scoping logic.
func scopeFor(clientID string) (platformconfig.Scope, *string) {
	if clientID != "" {
		cid := clientID
		return platformconfig.ScopeClient, &cid
	}
	return platformconfig.ScopeGlobal, nil
}

type propertyInput struct {
	App      string `path:"app"`
	Section  string `path:"section"`
	Property string `path:"property"`
	ClientID string `query:"clientId"`
}

func (s *State) getProperty(ctx context.Context, in *propertyInput) (*apicommon.Out[ConfigResponse], error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, in.App, ac.Roles, false)
		if err != nil {
			return nil, usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return nil, httperror.Forbidden("No read access to platform config for " + in.App)
		}
	}
	scope, clientID := scopeFor(in.ClientID)
	c, err := s.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", in.App+"/"+in.Section+"/"+in.Property)
	}
	if !ac.IsAnchor() && c.ValueType == platformconfig.ValueSecret {
		c.Value = "***"
	}
	return &apicommon.Out[ConfigResponse]{Body: configFromEntity(c)}, nil
}

type setPropertyInput struct {
	App      string `path:"app"`
	Section  string `path:"section"`
	Property string `path:"property"`
	ClientID string `query:"clientId"`
	Body     SetPropertyRequest
}

func (s *State) setProperty(ctx context.Context, in *setPropertyInput) (*apicommon.Out[ConfigResponse], error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, in.App, ac.Roles, true)
		if err != nil {
			return nil, usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return nil, httperror.Forbidden("No write access to platform config for " + in.App)
		}
	}
	// The SPA passes clientId as a query param; the body carries only
	// value/valueType/description. Fold the query value into the command.
	if in.ClientID != "" {
		cid := in.ClientID
		in.Body.ClientID = &cid
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.SetProperty(ctx, s.Repo, s.UoW, in.Body.toCommand(in.App, in.Section, in.Property), ec); err != nil {
		return nil, err
	}
	scope, clientID := scopeFor(in.ClientID)
	c, err := s.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, usecase.Internal("REPO", "config missing after set", nil)
	}
	return &apicommon.Out[ConfigResponse]{Body: configFromEntity(c)}, nil
}

func (s *State) deleteProperty(ctx context.Context, in *propertyInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(ctx, in.App, ac.Roles, true)
		if err != nil {
			return nil, usecase.Internal("REPO", "has_access failed", err)
		}
		if !ok {
			return nil, httperror.Forbidden("No write access to platform config for " + in.App)
		}
	}
	scope, clientID := scopeFor(in.ClientID)
	c, err := s.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return &apicommon.Empty{}, nil // idempotent
	}
	tx, err := s.UoW.Pool().Begin(ctx)
	if err != nil {
		return nil, usecase.Internal("TX", "begin failed", err)
	}
	defer tx.Rollback(ctx)
	if err := s.Repo.Delete(ctx, c, usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		return nil, usecase.Internal("REPO", "delete failed", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, usecase.Internal("TX", "commit failed", err)
	}
	return &apicommon.Empty{}, nil
}

type listAccessInput struct {
	App string `path:"app"`
}

func (s *State) listAccess(ctx context.Context, in *listAccessInput) (*apicommon.Out[AccessListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAccessByApplication(ctx, in.App)
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
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.GrantAccess(ctx, s.Repo, s.UoW, in.Body.toCommand(in.App), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().AccessID}}, nil
}

func (s *State) revokeAccess(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RevokeAccess(ctx, s.Repo, s.UoW, operations.RevokeAccessCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
