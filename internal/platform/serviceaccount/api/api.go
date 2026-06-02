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
	huma.Register(api, huma.Operation{
		OperationID:   "listServiceAccounts",
		Method:        http.MethodGet,
		Path:          "/api/service-accounts",
		Summary:       "List service accounts",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createServiceAccount",
		Method:        http.MethodPost,
		Path:          "/api/service-accounts",
		Summary:       "Create a service account",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getServiceAccountByCode",
		Method:        http.MethodGet,
		Path:          "/api/service-accounts/code/{code}",
		Summary:       "Get a service account by code",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByCode)

	huma.Register(api, huma.Operation{
		OperationID:   "getServiceAccount",
		Method:        http.MethodGet,
		Path:          "/api/service-accounts/{id}",
		Summary:       "Get a service account by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateServiceAccount",
		Method:        http.MethodPut,
		Path:          "/api/service-accounts/{id}",
		Summary:       "Update a service account",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deactivateServiceAccount",
		Method:        http.MethodPost,
		Path:          "/api/service-accounts/{id}/deactivate",
		Summary:       "Deactivate a service account",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.deactivate)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteServiceAccount",
		Method:        http.MethodDelete,
		Path:          "/api/service-accounts/{id}",
		Summary:       "Delete a service account",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)

	huma.Register(api, huma.Operation{
		OperationID:   "listServiceAccountRoles",
		Method:        http.MethodGet,
		Path:          "/api/service-accounts/{id}/roles",
		Summary:       "List a service account's roles",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listRoles)

	huma.Register(api, huma.Operation{
		OperationID:   "assignServiceAccountRoles",
		Method:        http.MethodPut,
		Path:          "/api/service-accounts/{id}/roles",
		Summary:       "Assign roles to a service account",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.assignRoles)

	// The SPA calls /regenerate-token + /regenerate-secret; the longer
	// /regenerate-auth-token + /regenerate-signing-secret paths match the
	// Rust platform + fcsdk. Both are registered against the same handlers.
	for _, p := range []string{"regenerate-token", "regenerate-auth-token"} {
		huma.Register(api, huma.Operation{
			OperationID:   "regenerateServiceAccountAuthToken_" + p,
			Method:        http.MethodPost,
			Path:          "/api/service-accounts/{id}/" + p,
			Summary:       "Regenerate a service account's auth token",
			Tags:          []string{tag},
			DefaultStatus: http.StatusOK,
		}, s.regenerateAuthToken)
	}

	for _, p := range []string{"regenerate-secret", "regenerate-signing-secret"} {
		huma.Register(api, huma.Operation{
			OperationID:   "regenerateServiceAccountSigningSecret_" + p,
			Method:        http.MethodPost,
			Path:          "/api/service-accounts/{id}/" + p,
			Summary:       "Regenerate a service account's signing secret",
			Tags:          []string{tag},
			DefaultStatus: http.StatusOK,
		}, s.regenerateSigningSecret)
	}
}

type emptyInput struct{}

type listOutput struct {
	Body ServiceAccountListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]ServiceAccountResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ServiceAccountListResponse{ServiceAccounts: out, Total: len(out)}}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

type getOutput struct {
	Body ServiceAccountResponse
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(sa)}, nil
}

type getInput struct {
	ID string `path:"id"`
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(sa)}, nil
}

type createInput struct {
	Body CreateServiceAccountRequest
}

type createOutput struct {
	Body CreateServiceAccountResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
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
	return &createOutput{Body: CreateServiceAccountResponse{
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

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateServiceAccount(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) deactivate(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeactivateServiceAccount(ctx, s.Repo, s.UoW, operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteServiceAccounts(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteServiceAccount(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type listRolesOutput struct {
	Body ServiceAccountRoleListResponse
}

func (s *State) listRoles(ctx context.Context, in *idInput) (*listRolesOutput, error) {
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
	return &listRolesOutput{Body: ServiceAccountRoleListResponse{Roles: roleDTOs(roles)}}, nil
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

type rolesAssignedOutput struct {
	Body ServiceAccountRolesAssignedResponse
}

func (s *State) assignRoles(ctx context.Context, in *assignRolesInput) (*rolesAssignedOutput, error) {
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
	return &rolesAssignedOutput{Body: ServiceAccountRolesAssignedResponse{
		Roles:        roleDTOs(roles),
		AddedRoles:   ev.RolesAdded,
		RemovedRoles: ev.RolesRemoved,
	}}, nil
}

type regenerateAuthTokenOutput struct {
	Body RegenerateAuthTokenResponse
}

func (s *State) regenerateAuthToken(ctx context.Context, in *idInput) (*regenerateAuthTokenOutput, error) {
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
	return &regenerateAuthTokenOutput{Body: resp}, nil
}

type regenerateSigningSecretOutput struct {
	Body RegenerateSigningSecretResponse
}

func (s *State) regenerateSigningSecret(ctx context.Context, in *idInput) (*regenerateSigningSecretOutput, error) {
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
	return &regenerateSigningSecretOutput{Body: resp}, nil
}
