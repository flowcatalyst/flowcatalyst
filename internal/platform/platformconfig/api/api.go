// Package api wires HTTP routes for platform_config via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
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
	huma.Register(api, huma.Operation{
		OperationID:   "listPlatformConfigProperties",
		Method:        http.MethodGet,
		Path:          "/api/platform-config/{app}",
		Summary:       "List platform-config properties for an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listProperties)

	huma.Register(api, huma.Operation{
		OperationID:   "getPlatformConfigProperty",
		Method:        http.MethodGet,
		Path:          "/api/config/{app}/{section}/{property}",
		Summary:       "Get a single platform-config property",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getProperty)

	// The SPA calls /api/config/...; the legacy /api/platform-config/...
	// path is kept for the list/access admin routes below.
	huma.Register(api, huma.Operation{
		OperationID:   "setPlatformConfigProperty",
		Method:        http.MethodPut,
		Path:          "/api/config/{app}/{section}/{property}",
		Summary:       "Set a platform-config property",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.setProperty)

	huma.Register(api, huma.Operation{
		OperationID:   "deletePlatformConfigProperty",
		Method:        http.MethodDelete,
		Path:          "/api/config/{app}/{section}/{property}",
		Summary:       "Delete a platform-config property",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.deleteProperty)

	huma.Register(api, huma.Operation{
		OperationID:   "listPlatformConfigAccess",
		Method:        http.MethodGet,
		Path:          "/api/platform-config/{app}/access",
		Summary:       "List access grants for an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listAccess)

	huma.Register(api, huma.Operation{
		OperationID:   "grantPlatformConfigAccess",
		Method:        http.MethodPost,
		Path:          "/api/platform-config/{app}/access",
		Summary:       "Grant access to platform-config for a role",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.grantAccess)

	huma.Register(api, huma.Operation{
		OperationID:   "revokePlatformConfigAccess",
		Method:        http.MethodDelete,
		Path:          "/api/platform-config/access/{id}",
		Summary:       "Revoke a platform-config access grant",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.revokeAccess)
}

type listPropsInput struct {
	App string `path:"app"`
}

type listPropsOutput struct {
	Body ConfigListResponse
}

func (s *State) listProperties(ctx context.Context, in *listPropsInput) (*listPropsOutput, error) {
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
	return &listPropsOutput{Body: ConfigListResponse{Items: out}}, nil
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

type configOutput struct {
	Body ConfigResponse
}

func (s *State) getProperty(ctx context.Context, in *propertyInput) (*configOutput, error) {
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
	return &configOutput{Body: configFromEntity(c)}, nil
}

type setPropertyInput struct {
	App      string `path:"app"`
	Section  string `path:"section"`
	Property string `path:"property"`
	ClientID string `query:"clientId"`
	Body     SetPropertyRequest
}

func (s *State) setProperty(ctx context.Context, in *setPropertyInput) (*configOutput, error) {
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
	return &configOutput{Body: configFromEntity(c)}, nil
}

func (s *State) deleteProperty(ctx context.Context, in *propertyInput) (*emptyOutput, error) {
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
		return &emptyOutput{}, nil // idempotent
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
	return &emptyOutput{}, nil
}

type listAccessInput struct {
	App string `path:"app"`
}

type listAccessOutput struct {
	Body AccessListResponse
}

func (s *State) listAccess(ctx context.Context, in *listAccessInput) (*listAccessOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAccessByApplication(ctx, in.App)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_access_by_application failed", err)
	}
	out := make([]AccessResponse, 0, len(rows))
	for i := range rows {
		out = append(out, accessFromEntity(&rows[i]))
	}
	return &listAccessOutput{Body: AccessListResponse{Items: out}}, nil
}

type grantAccessInput struct {
	App  string `path:"app"`
	Body GrantAccessRequest
}

type grantAccessOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) grantAccess(ctx context.Context, in *grantAccessInput) (*grantAccessOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.GrantAccess(ctx, s.Repo, s.UoW, in.Body.toCommand(in.App), ec)
	if err != nil {
		return nil, err
	}
	return &grantAccessOutput{Body: apicommon.CreatedResponse{ID: committed.Event().AccessID}}, nil
}

type revokeAccessInput struct {
	ID string `path:"id"`
}

type emptyOutput struct{}

func (s *State) revokeAccess(ctx context.Context, in *revokeAccessInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RevokeAccess(ctx, s.Repo, s.UoW, operations.RevokeAccessCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
