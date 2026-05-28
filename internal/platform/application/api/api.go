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
	huma.Register(api, huma.Operation{
		OperationID:   "listApplications",
		Method:        http.MethodGet,
		Path:          "/api/applications",
		Summary:       "List applications",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications",
		Summary:       "Create an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getApplicationByCode",
		Method:        http.MethodGet,
		Path:          "/api/applications/by-code/{code}",
		Summary:       "Get an application by code",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByCode)

	huma.Register(api, huma.Operation{
		OperationID:   "getApplication",
		Method:        http.MethodGet,
		Path:          "/api/applications/{id}",
		Summary:       "Get an application by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateApplication",
		Method:        http.MethodPut,
		Path:          "/api/applications/{id}",
		Summary:       "Update an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "activateApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/activate",
		Summary:       "Activate an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.activate)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivateApplication",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/deactivate",
		Summary:       "Deactivate an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.deactivate)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteApplication",
		Method:        http.MethodDelete,
		Path:          "/api/applications/{id}",
		Summary:       "Delete an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	huma.Register(api, huma.Operation{
		OperationID:   "attachApplicationServiceAccount",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/service-account",
		Summary:       "Attach a service account to an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.attachServiceAccount)

	huma.Register(api, huma.Operation{
		OperationID:   "listApplicationClientConfigs",
		Method:        http.MethodGet,
		Path:          "/api/applications/{id}/clients",
		Summary:       "List per-client configurations for an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listClientConfigs)

	huma.Register(api, huma.Operation{
		OperationID:   "enableApplicationForClient",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/clients/{clientId}/enable",
		Summary:       "Enable an application for a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.enableForClient)

	huma.Register(api, huma.Operation{
		OperationID:   "disableApplicationForClient",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/clients/{clientId}/disable",
		Summary:       "Disable an application for a client",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.disableForClient)

	huma.Register(api, huma.Operation{
		OperationID:   "provisionApplicationServiceAccount",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/provision-service-account",
		Summary:       "Create + attach a dedicated service account for the application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.provisionServiceAccount)

	huma.Register(api, huma.Operation{
		OperationID:   "provisionApplicationLoginClient",
		Method:        http.MethodPost,
		Path:          "/api/applications/{id}/provision-login-client",
		Summary:       "Create a public OAuth login client for the application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.provisionLoginClient)

	huma.Register(api, huma.Operation{
		OperationID:   "listApplicationRoles",
		Method:        http.MethodGet,
		Path:          "/api/applications/by-id/{id}/roles",
		Summary:       "List roles registered against an application",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listApplicationRoles)

	huma.Register(api, huma.Operation{
		OperationID:   "getApplicationClientConfig",
		Method:        http.MethodGet,
		Path:          "/api/applications/{id}/clients/{clientId}",
		Summary:       "Get a single application-client config",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getClientConfig)
}

type listInput struct {
	Type   string `query:"type"`
	Active string `query:"active"`
}

type listOutput struct {
	Body ApplicationListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	var appType, active *string
	if in.Type != "" {
		appType = &in.Type
	}
	if in.Active != "" {
		active = &in.Active
	}
	rows, err := s.Repo.FindWithFilters(ctx, appType, active)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]ApplicationResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ApplicationListResponse{Applications: out, Total: len(out)}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body ApplicationResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(a)}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(a)}, nil
}

type createInput struct {
	Body CreateApplicationRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ApplicationID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateApplicationRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateApplication(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) activate(ctx context.Context, in *idInput) (*getOutput, error) {
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

func (s *State) deactivate(ctx context.Context, in *idInput) (*getOutput, error) {
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
func (s *State) refreshedApp(ctx context.Context, id string) (*getOutput, error) {
	a, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if a == nil {
		return nil, httperror.NotFound("Application", id)
	}
	return &getOutput{Body: fromEntity(a)}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteApplications(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteApplication(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type attachSAInput struct {
	ID   string `path:"id"`
	Body AttachServiceAccountRequest
}

func (s *State) attachServiceAccount(ctx context.Context, in *attachSAInput) (*emptyOutput, error) {
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
	return &emptyOutput{}, nil
}

type listConfigsInput struct {
	ID string `path:"id"`
}

type listConfigsOutput struct {
	Body ClientConfigListResponse
}

func (s *State) listClientConfigs(ctx context.Context, in *listConfigsInput) (*listConfigsOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	rows, err := s.ClientConfigRepo.FindByApplication(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "list client configs failed", err)
	}
	out := make([]ClientConfigResponse, 0, len(rows))
	for i := range rows {
		out = append(out, clientConfigFromEntity(&rows[i]))
	}
	return &listConfigsOutput{Body: ClientConfigListResponse{Items: out}}, nil
}

type clientToggleInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

func (s *State) enableForClient(ctx context.Context, in *clientToggleInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.EnableApplicationForClient(ctx, s.Repo, s.ClientRepo, s.ClientConfigRepo, s.UoW,
		operations.EnableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) disableForClient(ctx context.Context, in *clientToggleInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DisableApplicationForClient(ctx, s.ClientConfigRepo, s.UoW,
		operations.DisableForClientCommand{ApplicationID: in.ID, ClientID: in.ClientID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

// ── provisioning ────────────────────────────────────────────────────────

type provisionSAOutput struct {
	Body ApplicationProvisionServiceAccountResponse
}

// provisionServiceAccount creates a dedicated service account, its
// SERVICE principal, attaches it to the application, and creates a
// confidential OAuth client — atomically. Mirrors Rust's three-step
// transactional flow. The OAuth client secret is returned once
// (plaintext); it's stored hashed.
func (s *State) provisionServiceAccount(ctx context.Context, in *idInput) (*provisionSAOutput, error) {
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
	return &provisionSAOutput{Body: ApplicationProvisionServiceAccountResponse{
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

type provisionLoginClientOutput struct {
	Body ApplicationProvisionLoginClientResponse
}

// provisionLoginClient creates an OAuth client (authorization_code grant)
// for use as a login client by the application's frontend. PUBLIC clients
// (the default — SPAs, native apps) enforce PKCE and have no secret;
// CONFIDENTIAL clients get a plaintext secret returned exactly once.
func (s *State) provisionLoginClient(ctx context.Context, in *provisionLoginClientInput) (*provisionLoginClientOutput, error) {
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

	return &provisionLoginClientOutput{Body: ApplicationProvisionLoginClientResponse{
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

type appRolesOutput struct {
	Body ApplicationRolesResponse
}

func (s *State) listApplicationRoles(ctx context.Context, in *idInput) (*appRolesOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadApplications(ac); err != nil {
		return nil, err
	}
	if s.Roles == nil {
		return &appRolesOutput{Body: ApplicationRolesResponse{Roles: []string{}}}, nil
	}
	rows, err := s.Roles.FindByApplicationID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "roles_by_application_id failed", err)
	}
	out := make([]string, 0, len(rows))
	for i := range rows {
		out = append(out, rows[i].Name)
	}
	return &appRolesOutput{Body: ApplicationRolesResponse{Roles: out}}, nil
}

type singleConfigInput struct {
	ID       string `path:"id"`
	ClientID string `path:"clientId"`
}

type singleConfigOutput struct {
	Body ClientConfigResponse
}

func (s *State) getClientConfig(ctx context.Context, in *singleConfigInput) (*singleConfigOutput, error) {
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
	return &singleConfigOutput{Body: clientConfigFromEntity(cfg)}, nil
}
