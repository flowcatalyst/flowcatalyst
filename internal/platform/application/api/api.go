// Package api wires the HTTP routes for the application subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo             *application.Repository
	ClientConfigRepo *application.ClientConfigRepo
	ClientRepo       *client.Repository
	Principals       *principal.Repository
	Roles            *role.Repository
	ServiceAccounts  *serviceaccount.Repository
	OAuthClients     *platformauth.OAuthClientRepo
	UoW              *usecasepgx.UnitOfWork
}

const tag = "applications"

// Register mounts the application endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listApplications", "/api/applications", "List applications", s.list)
	apiroute.Post(g, "createApplication", "/api/applications", "Create an application", http.StatusCreated, s.create)
	apiroute.Get(g, "getApplicationByCode", "/api/applications/by-code/{code}", "Get an application by code", s.getByCode)
	apiroute.Get(g, "getApplication", "/api/applications/{id}", "Get an application by id", s.getByID)
	apiroute.Put(g, "updateApplication", "/api/applications/{id}", "Update an application", http.StatusNoContent, s.update)
	apiroute.Post(g, "activateApplication", "/api/applications/{id}/activate", "Activate an application", http.StatusOK, s.activate)
	apiroute.Post(g, "deactivateApplication", "/api/applications/{id}/deactivate", "Deactivate an application", http.StatusOK, s.deactivate)
	apiroute.Delete(g, "deleteApplication", "/api/applications/{id}", "Delete an application", http.StatusNoContent, s.delete)
	apiroute.Post(g, "attachApplicationServiceAccount", "/api/applications/{id}/service-account", "Attach a service account to an application", http.StatusNoContent, s.attachServiceAccount)
	apiroute.Get(g, "listApplicationClientConfigs", "/api/applications/{id}/clients", "List per-client configurations for an application", s.listClientConfigs)
	apiroute.Post(g, "enableApplicationForClient", "/api/applications/{id}/clients/{clientId}/enable", "Enable an application for a client", http.StatusNoContent, s.enableForClient)
	apiroute.Post(g, "disableApplicationForClient", "/api/applications/{id}/clients/{clientId}/disable", "Disable an application for a client", http.StatusNoContent, s.disableForClient)
	apiroute.Post(g, "provisionApplicationServiceAccount", "/api/applications/{id}/provision-service-account", "Create + attach a dedicated service account for the application", http.StatusCreated, s.provisionServiceAccount)
	apiroute.Post(g, "provisionApplicationLoginClient", "/api/applications/{id}/provision-login-client", "Create a public OAuth login client for the application", http.StatusCreated, s.provisionLoginClient)
	apiroute.Get(g, "listApplicationRoles", "/api/applications/by-id/{id}/roles", "List roles registered against an application", s.listApplicationRoles)
	apiroute.Get(g, "getApplicationClientConfig", "/api/applications/{id}/clients/{clientId}", "Get a single application-client config", s.getClientConfig)
}

type listInput struct {
	Type   string `query:"type"`
	Active string `query:"active"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[ApplicationListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	appType := apicommon.OptStr(in.Type)
	active := apicommon.OptStr(in.Active)
	rows, err := s.Repo.FindWithFilters(ctx, appType, active)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ApplicationListResponse]{Body: ApplicationListResponse{Applications: out, Total: len(out)}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	a, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", in.ID)
	}
	return &apicommon.Out[ApplicationResponse]{Body: fromEntity(a)}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*apicommon.Out[ApplicationResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	a, err := s.Repo.FindByCode(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", in.Code)
	}
	return &apicommon.Out[ApplicationResponse]{Body: fromEntity(a)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateApplicationRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().ApplicationID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateApplicationRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) activate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ActivateApplication(ctx, s.Repo, s.UoW, operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return s.refreshedApp(ctx, in.ID)
}

func (s *State) deactivate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeactivateApplication(ctx, s.Repo, s.UoW, operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return s.refreshedApp(ctx, in.ID)
}

// refreshedApp re-fetches an application and returns it as the standard
// ApplicationResponse body. Used by mutations whose result the SPA reads.
func (s *State) refreshedApp(ctx context.Context, id string) (*apicommon.Out[ApplicationResponse], error) {
	a, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", id)
	}
	return &apicommon.Out[ApplicationResponse]{Body: fromEntity(a)}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteApplication(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type attachSAInput struct {
	ID   string `path:"id"`
	Body AttachServiceAccountRequest
}

func (s *State) attachServiceAccount(ctx context.Context, in *attachSAInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.AttachServiceAccount(ctx, s.Repo, s.Principals, s.UoW,
		operations.AttachServiceAccountCommand{
			ApplicationID:      in.ID,
			ServiceAccountID:   in.Body.ServiceAccountID,
			ServiceAccountCode: in.Body.ServiceAccountCode,
		}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) listClientConfigs(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ClientConfigListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	rows, err := s.ClientConfigRepo.FindByApplication(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list client configs failed", err)
	}
	out := apicommon.MapSlice(rows, clientConfigFromEntity)
	return &apicommon.Out[ClientConfigListResponse]{Body: ClientConfigListResponse{Items: out}}, nil
}

type clientToggleInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

func (s *State) enableForClient(ctx context.Context, in *clientToggleInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.EnableApplicationForClient(ctx, s.Repo, s.ClientRepo, s.ClientConfigRepo, s.UoW,
		operations.EnableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) disableForClient(ctx context.Context, in *clientToggleInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DisableApplicationForClient(ctx, s.ClientConfigRepo, s.UoW,
		operations.DisableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── provisioning ────────────────────────────────────────────────────────

// provisionServiceAccount creates a dedicated service account, its
// SERVICE principal, attaches it to the application, and creates a
// confidential OAuth client — atomically. Mirrors Rust's three-step
// transactional flow. The OAuth client secret is returned once
// (plaintext); it's stored hashed.
func (s *State) provisionServiceAccount(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationProvisionServiceAccountResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	result, err := operations.ProvisionServiceAccount(ctx, s.Repo, s.ServiceAccounts, s.Principals, s.OAuthClients, s.UoW,
		operations.ProvisionServiceAccountCommand{ApplicationID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[ApplicationProvisionServiceAccountResponse]{Body: ApplicationProvisionServiceAccountResponse{
		Message: "Service account provisioned",
		ServiceAccount: ApplicationServiceAccountCredentials{
			PrincipalID: result.ServicePrincipalID,
			Name:        result.ServiceAccountName,
			OAuthClient: ApplicationOAuthClientCredentials{
				ID:           result.OAuthClientRowID,
				ClientID:     result.OAuthClientID,
				ClientSecret: result.OAuthClientSecret,
			},
		},
	}}, nil
}

type provisionLoginClientInput struct {
	ID   string `path:"id"`
	Body ProvisionLoginClientRequest
}

// provisionLoginClient creates an OAuth client (authorization_code grant)
// for use as a login client by the application's frontend. PUBLIC clients
// (the default — SPAs, native apps) enforce PKCE and have no secret;
// CONFIDENTIAL clients get a plaintext secret returned exactly once.
func (s *State) provisionLoginClient(ctx context.Context, in *provisionLoginClientInput) (*apicommon.Out[ApplicationProvisionLoginClientResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if len(in.Body.RedirectURIs) == 0 {
		return nil, usecase.Validation("REDIRECT_URIS_REQUIRED", "At least one redirect URI is required")
	}
	app, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if app == nil {
		return nil, httperror.NotFound("Application", in.ID)
	}

	clientType := "PUBLIC"
	if in.Body.ClientType == "CONFIDENTIAL" {
		clientType = "CONFIDENTIAL"
	}

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	publicClientID := tsid.Generate(tsid.OAuthClient)
	cmd := authops.CreateOAuthClientCommand{
		ClientID:     publicClientID,
		ClientName:   app.Name + " Login",
		ClientType:   clientType,
		RedirectURIs: in.Body.RedirectURIs,
		GrantTypes:   []string{"authorization_code", "refresh_token"},
		Scopes:       []string{"openid", "profile", "email"},
	}
	committed, err := authops.CreateOAuthClient(ctx, s.OAuthClients, s.UoW, cmd, ec)
	if err != nil {
		return nil, err
	}

	// CreateOAuthClient generates the internal row id (`oac_…`) on the
	// entity; the created event carries it. CONFIDENTIAL clients stash a
	// once-readable plaintext secret.
	rowID := committed.Event().OAuthClientID
	var secret string
	if clientType == "CONFIDENTIAL" {
		if s, ok := authops.PopStashedSecret(rowID); ok {
			secret = s
		}
	}

	return &apicommon.Out[ApplicationProvisionLoginClientResponse]{Body: ApplicationProvisionLoginClientResponse{
		Message: "Login client provisioned",
		LoginClient: ApplicationLoginClientCredentials{
			ClientType:   clientType,
			RedirectURIs: in.Body.RedirectURIs,
			OAuthClient: ApplicationOAuthClientCredentials{
				ID:           rowID,
				ClientID:     publicClientID,
				ClientSecret: secret,
			},
		},
	}}, nil
}

// ── application roles + single client config ────────────────────────────

func (s *State) listApplicationRoles(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ApplicationRolesResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	if s.Roles == nil {
		return &apicommon.Out[ApplicationRolesResponse]{Body: ApplicationRolesResponse{Roles: []string{}}}, nil
	}
	rows, err := s.Roles.FindByApplicationID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "roles_by_application_id failed", err)
	}
	out := apicommon.MapSlice(rows, func(r *role.Role) string { return r.Name })
	return &apicommon.Out[ApplicationRolesResponse]{Body: ApplicationRolesResponse{Roles: out}}, nil
}

type singleConfigInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

func (s *State) getClientConfig(ctx context.Context, in *singleConfigInput) (*apicommon.Out[ClientConfigResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	cfg, err := s.ClientConfigRepo.FindByApplicationAndClient(ctx, in.ID, in.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_config failed", err)
	}
	if cfg == nil {
		return nil, httperror.NotFound("ClientConfig", in.ID+":"+in.ClientID)
	}
	return &apicommon.Out[ClientConfigResponse]{Body: clientConfigFromEntity(cfg)}, nil
}
