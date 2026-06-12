// Package api wires the HTTP routes for the service_account subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps. Principals + OAuthClients are needed because creating
// a service account also provisions its linked SERVICE principal and a
// confidential OAuth client (matching the Rust platform).
type State struct {
	Repo         *serviceaccount.Repository
	Principals   *principal.Repository
	OAuthClients *platformauth.OAuthClientRepo
	UoW          *usecasepgx.UnitOfWork
}

const tag = "service-accounts"

// Register mounts the service-account endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listServiceAccounts", "/api/service-accounts", "List service accounts", s.list)
	apiroute.Post(g, "createServiceAccount", "/api/service-accounts", "Create a service account", http.StatusCreated, s.create)
	apiroute.Get(g, "getServiceAccountByCode", "/api/service-accounts/code/{code}", "Get a service account by code", s.getByCode)
	apiroute.Get(g, "getServiceAccount", "/api/service-accounts/{id}", "Get a service account by id", s.getByID)
	apiroute.Put(g, "updateServiceAccount", "/api/service-accounts/{id}", "Update a service account", http.StatusNoContent, s.update)
	apiroute.Post(g, "deactivateServiceAccount", "/api/service-accounts/{id}/deactivate", "Deactivate a service account", http.StatusNoContent, s.deactivate)
	apiroute.Delete(g, "deleteServiceAccount", "/api/service-accounts/{id}", "Delete a service account", http.StatusNoContent, s.delete)
	apiroute.Get(g, "listServiceAccountRoles", "/api/service-accounts/{id}/roles", "List a service account's roles", s.listRoles)
	apiroute.Put(g, "assignServiceAccountRoles", "/api/service-accounts/{id}/roles", "Assign roles to a service account", http.StatusOK, s.assignRoles)

	// The SPA calls /regenerate-token + /regenerate-secret; the longer
	// /regenerate-auth-token + /regenerate-signing-secret paths match the
	// Rust platform + fcsdk. Both are registered against the same handlers.
	for _, p := range []string{"regenerate-token", "regenerate-auth-token"} {
		apiroute.Post(g, "regenerateServiceAccountAuthToken_"+p, "/api/service-accounts/{id}/"+p,
			"Regenerate a service account's auth token", http.StatusOK, s.regenerateAuthToken)
	}

	for _, p := range []string{"regenerate-secret", "regenerate-signing-secret"} {
		apiroute.Post(g, "regenerateServiceAccountSigningSecret_"+p, "/api/service-accounts/{id}/"+p,
			"Regenerate a service account's signing secret", http.StatusOK, s.regenerateSigningSecret)
	}
}

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[ServiceAccountListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ServiceAccountListResponse]{Body: ServiceAccountListResponse{ServiceAccounts: out, Total: len(out)}}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*apicommon.Out[ServiceAccountResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	sa, err := s.Repo.FindByCode(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", in.Code)
	}
	return &apicommon.Out[ServiceAccountResponse]{Body: fromEntity(sa)}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ServiceAccountResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	sa, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", in.ID)
	}
	resp := fromEntity(sa)
	// Surface the linked SERVICE principal's id so the UI can manage this
	// account's application access via /api/principals/{id}/application-access
	// (roles + app-access live on the principal, not the service-account row).
	if p, err := s.Principals.FindByServiceAccount(ctx, in.ID); err == nil && p != nil {
		resp.PrincipalID = &p.ID
	}
	return &apicommon.Out[ServiceAccountResponse]{Body: resp}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateServiceAccountRequest]) (*apicommon.Out[CreateServiceAccountResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	res, err := operations.CreateServiceAccountWithCredentials(
		ctx, s.Repo, s.Principals, s.OAuthClients, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[CreateServiceAccountResponse]{Body: CreateServiceAccountResponse{
		ServiceAccount: fromEntity(res.ServiceAccount),
		PrincipalID:    res.PrincipalID,
		OAuth:          ServiceAccountOAuthSecrets{ClientID: res.OAuthClientID, ClientSecret: res.OAuthClientSecret},
		Webhook:        ServiceAccountWebhookSecrets{AuthToken: res.AuthToken, SigningSecret: res.SigningSecret},
	}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateServiceAccountRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateServiceAccount(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) deactivate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeactivateServiceAccount(ctx, s.Repo, s.UoW, operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteServiceAccount(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) listRoles(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ServiceAccountRoleListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	sa, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", in.ID)
	}
	// Roles live on the linked SERVICE principal (iam_principal_roles), not
	// the service-account row itself.
	roles, err := s.serviceAccountRoles(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[ServiceAccountRoleListResponse]{Body: ServiceAccountRoleListResponse{Roles: roleDTOs(roles)}}, nil
}

// serviceAccountRoles returns the role assignments of the service account's
// linked SERVICE principal (where they're actually stored), or nil if no
// linked principal exists.
func (s *State) serviceAccountRoles(ctx context.Context, saID string) ([]serviceaccount.RoleAssignment, error) {
	p, err := s.Principals.FindByServiceAccount(ctx, saID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p == nil {
		return nil, nil
	}
	return p.Roles, nil
}

type assignRolesInput struct {
	ID   string `path:"id"`
	Body AssignRolesRequest
}

func (s *State) assignRoles(ctx context.Context, in *assignRolesInput) (*apicommon.Out[ServiceAccountRolesAssignedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	// The operation resolves the linked SERVICE principal, computes the
	// added/removed diff, and writes iam_principal_roles in one transaction;
	// the 404 for an unknown id is raised there too.
	committed, err := operations.AssignRolesToServiceAccount(ctx, s.Repo, s.Principals, s.UoW,
		operations.AssignRolesCommand{ServiceAccountID: in.ID, Roles: in.Body.Roles}, ec)
	if err != nil {
		return nil, err
	}
	roles, err := s.serviceAccountRoles(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	ev := committed.Event()
	return &apicommon.Out[ServiceAccountRolesAssignedResponse]{Body: ServiceAccountRolesAssignedResponse{
		Roles:        roleDTOs(roles),
		AddedRoles:   ev.RolesAdded,
		RemovedRoles: ev.RolesRemoved,
	}}, nil
}

func (s *State) regenerateAuthToken(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[RegenerateAuthTokenResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RegenerateAuthToken(ctx, s.Repo, s.UoW,
		operations.RegenerateAuthTokenCommand{ServiceAccountID: in.ID}, ec); err != nil {
		return nil, err
	}
	resp := RegenerateAuthTokenResponse{ID: in.ID}
	if token, ok := operations.PopStashedSecret(in.ID, "token"); ok {
		resp.AuthToken = token
	}
	return &apicommon.Out[RegenerateAuthTokenResponse]{Body: resp}, nil
}

func (s *State) regenerateSigningSecret(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[RegenerateSigningSecretResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.RegenerateSigningSecret(ctx, s.Repo, s.UoW,
		operations.RegenerateSigningSecretCommand{ServiceAccountID: in.ID}, ec); err != nil {
		return nil, err
	}
	resp := RegenerateSigningSecretResponse{ID: in.ID}
	if secret, ok := operations.PopStashedSecret(in.ID, "signing_secret"); ok {
		resp.SigningSecret = secret
	}
	return &apicommon.Out[RegenerateSigningSecretResponse]{Body: resp}, nil
}
