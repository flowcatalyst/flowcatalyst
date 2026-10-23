// Package api wires HTTP routes for platform_config via huma.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *platformconfig.Repository
	UoW  *usecasepgx.UnitOfWork
	// Apps resolves the {app} path code to an application, for confining
	// application-scoped principals (see confineApp).
	Apps AppLookup
	// Audit records every secret reveal. Required for the reveal endpoint: a
	// reveal that cannot be recorded is refused.
	Audit *audit.Repository
}

// AppLookup is the slice of the application repository the confinement needs.
type AppLookup interface {
	FindByCode(ctx context.Context, code string) (*application.Application, error)
}

// confineApp keeps an application-scoped principal (one that does not hold
// all-applications access, e.g. an application's service account) to its own
// application's config. Anchor tier alone is not enough: every application's
// service account is anchor tier, and without this it could read (secret
// values included), write, delete or grant access to any other application's
// config. An application code that names no application is refused for such a
// principal, so it cannot probe or squat codes it does not own.
func (s *State) confineApp(ctx context.Context, ac *auth.AuthContext, appCode string) error {
	// Non-anchor callers are already gated by the role-based access grants and
	// see secrets masked; the gap is anchor tier, which skips both.
	if !ac.IsAnchor() || !ac.IsApplicationScoped() {
		return nil
	}
	app, err := s.Apps.FindByCode(ctx, appCode)
	if err != nil {
		return usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil || !ac.CanAccessApplication(string(app.ID)) {
		return httperror.Forbidden("Not authorised for application '" + appCode + "'")
	}
	return nil
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
	ac := auth.FromContext(ctx)
	if err := s.confineApp(ctx, ac, in.App); err != nil {
		return nil, err
	}
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
		out = append(out, configFromEntity(&rows[i]))
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
	if err := s.confineApp(ctx, ac, in.App); err != nil {
		return nil, err
	}
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
	if err := s.confineApp(ctx, ac, in.App); err != nil {
		return nil, err
	}
	// The SPA passes clientId as a query param; the body carries only
	// value/valueType/description. Fold the query value into the command.
	if in.ClientID != "" {
		cid := in.ClientID
		in.Body.ClientID = &cid
	}
	// Authorization (anchor, or non-anchor with write access to the app) runs
	// inside the use case's Authorize phase.
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.SetProperty(s.Repo), in.Body.toCommand(in.App, in.Section, in.Property), ec); err != nil {
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
	if err := s.confineApp(ctx, ac, in.App); err != nil {
		return nil, err
	}
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
	if err := auth.CanReadPlatformConfig(ac); err != nil {
		return nil, err
	}
	if err := s.confineApp(ctx, ac, in.App); err != nil {
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
	// Coarse anchor-only authorization at the controller.
	if err := auth.CanUpdatePlatformConfig(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	if err := s.confineApp(ctx, auth.FromContext(ctx), in.App); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.GrantAccess(s.Repo), in.Body.toCommand(in.App), ec)
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
	if _, err := usecaseop.Run(ctx, s.UoW, operations.RevokeAccess(s.Repo), operations.RevokeAccessCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type revealOutput struct {
	// CacheControl keeps the plaintext out of every shared or browser cache.
	CacheControl string `header:"Cache-Control"`
	Body         SecretResponse
}

// authorizeReveal is the reveal rule (see revealSecret).
func (s *State) authorizeReveal(ctx context.Context, ac *auth.AuthContext, app, clientID string) error {
	if err := s.confineApp(ctx, ac, app); err != nil {
		return err
	}
	if !(ac.IsAnchor() && ac.IsApplicationScoped()) {
		if err := auth.CanRevealPlatformConfigSecret(ac); err != nil {
			return err
		}
	}
	if clientID != "" && !ac.CanAccessClient(clientID) {
		return httperror.Forbidden("Not authorised for client '" + clientID + "'")
	}
	return nil
}

// revealSecret returns a SECRET's plaintext. It is the only place one leaves
// the platform, so it is strict:
//   - the caller is confined to its own application (confineApp), and an
//     application's own service account may reveal its application's secrets;
//     every other caller must hold platform:admin:config:reveal;
//   - a client-scoped secret needs access to that client;
//   - the reveal is audited, and refused if the audit row cannot be written;
//   - the response is never cached.
func (s *State) revealSecret(ctx context.Context, in *propertyInput) (*revealOutput, error) {
	ac := auth.FromContext(ctx)
	if err := s.authorizeReveal(ctx, ac, in.App, in.ClientID); err != nil {
		return nil, err
	}
	scope, clientID := scopeFor(in.ClientID)
	c, err := s.Repo.FindByCoordinate(ctx, in.App, in.Section, in.Property, scope, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_coordinate failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Config", in.App+"/"+in.Section+"/"+in.Property)
	}
	if c.ValueType != platformconfig.ValueSecret {
		return nil, usecase.Validation("NOT_A_SECRET", "this property is not a SECRET; read it with GET /api/config/{app}/{section}/{property}")
	}
	value, err := s.Repo.RevealValue(c)
	if err != nil {
		return nil, usecase.Internal("REVEAL", "secret could not be decrypted", err)
	}
	if s.Audit == nil {
		return nil, usecase.Internal("AUDIT", "secret reveal is not wired to the audit log", nil)
	}
	detail, _ := json.Marshal(map[string]string{
		"applicationCode": c.ApplicationCode, "section": c.Section, "property": c.Property,
	})
	actor := ac.PrincipalID
	if err := s.Audit.Insert(ctx, &audit.Log{
		ID:            tsid.Generate(tsid.AuditLog),
		EntityType:    "PLATFORM_CONFIG",
		EntityID:      c.ID,
		Operation:     "SECRET_REVEALED",
		OperationJSON: detail,
		PrincipalID:   &actor,
		ClientID:      ids.StringPtr(c.ClientID),
		PerformedAt:   time.Now().UTC(),
	}); err != nil {
		return nil, usecase.Internal("AUDIT", "secret reveal could not be audited", err)
	}
	return &revealOutput{
		CacheControl: "no-store",
		Body: SecretResponse{
			ApplicationCode: c.ApplicationCode,
			Section:         c.Section,
			Property:        c.Property,
			ClientID:        ids.StringPtr(c.ClientID),
			Value:           value,
			UpdatedAt:       jsontime.New(c.UpdatedAt),
		},
	}, nil
}
