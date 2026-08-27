// Package api wires the HTTP routes for the service_account subdomain via huma.
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps. Principals + OAuthClients are needed because creating
// a service account also provisions its linked SERVICE principal and a
// confidential OAuth client.
type State struct {
	Repo         *serviceaccount.Repository
	Principals   *principal.Repository
	OAuthClients *platformauth.OAuthClientRepo
	UoW          *usecasepgx.UnitOfWork
	// Auth mints the admin-requested bearer on POST /{id}/token. Optional —
	// nil disables that endpoint (fail closed).
	Auth *authservice.AuthService
	// FlattenPermissions resolves the linked principal's roles into the
	// permission ceiling carried as the minted token's "scope" claim — the
	// same computation the client_credentials grant runs. Optional; when nil
	// the token carries no scope claim (permissions derived from roles
	// downstream, the legacy behaviour).
	FlattenPermissions func(ctx context.Context, roleNames []string) ([]string, error)
	// Audit records admin token mints (an admin obtaining a live credential
	// for another identity must leave a trail). Optional.
	Audit *audit.Repository
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
	// /regenerate-auth-token + /regenerate-signing-secret paths match
	// fcsdk. Both are registered against the same handlers.
	for _, p := range []string{"regenerate-token", "regenerate-auth-token"} {
		apiroute.Post(g, "regenerateServiceAccountAuthToken_"+p, "/api/service-accounts/{id}/"+p,
			"Regenerate a service account's auth token", http.StatusOK, s.regenerateAuthToken)
	}

	for _, p := range []string{"regenerate-secret", "regenerate-signing-secret"} {
		apiroute.Post(g, "regenerateServiceAccountSigningSecret_"+p, "/api/service-accounts/{id}/"+p,
			"Regenerate a service account's signing secret", http.StatusOK, s.regenerateSigningSecret)
	}

	apiroute.Post(g, "mintServiceAccountToken", "/api/service-accounts/{id}/token",
		"Mint a short-lived bearer token for the service account (anchor-only, audited)", http.StatusOK, s.mintToken)
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
	// Coarse permission at the controller; the orchestration runs inside one
	// transaction and has no per-client resource check (admin-managed create).
	if err := auth.CanWriteServiceAccounts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	res, err := usecaseop.RunTx(ctx, s.UoW,
		operations.CreateServiceAccountWithCredentials(s.Repo, s.Principals, s.OAuthClients),
		in.Body.toCommand(), ec)
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
	if err := auth.CanWriteServiceAccounts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateServiceAccount(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) deactivate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteServiceAccounts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeactivateServiceAccount(s.Repo), operations.DeactivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteServiceAccounts(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteServiceAccount(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
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
	if err := auth.RequireAnchor(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	// The operation resolves the linked SERVICE principal, computes the
	// added/removed diff, and writes iam_principal_roles in one transaction;
	// the 404 for an unknown id is raised there too.
	ev, err := usecaseop.Run(ctx, s.UoW, operations.AssignRolesToServiceAccount(s.Repo, s.Principals),
		operations.AssignRolesCommand{ServiceAccountID: in.ID, Roles: in.Body.Roles}, ec)
	if err != nil {
		return nil, err
	}
	roles, err := s.serviceAccountRoles(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[ServiceAccountRolesAssignedResponse]{Body: ServiceAccountRolesAssignedResponse{
		Roles:        roleDTOs(roles),
		AddedRoles:   ev.RolesAdded,
		RemovedRoles: ev.RolesRemoved,
	}}, nil
}

func (s *State) regenerateAuthToken(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[RegenerateAuthTokenResponse], error) {
	if err := auth.RequireAnchor(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	// The sink is a local: the plaintext cannot outlive this request, and it
	// is only read below, on the success path — a failed commit discloses
	// nothing.
	var token string
	if _, err := usecaseop.Run(ctx, s.UoW, operations.RegenerateAuthToken(s.Repo),
		operations.RegenerateAuthTokenCommand{
			ServiceAccountID: in.ID,
			Disclose:         func(plaintext string) { token = plaintext },
		}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[RegenerateAuthTokenResponse]{Body: RegenerateAuthTokenResponse{
		ID:        in.ID,
		AuthToken: token,
	}}, nil
}

// mintToken is POST /api/service-accounts/{id}/token: mints the same
// short-lived, authority-bearing bearer the account would obtain from the
// client_credentials grant — scope = the linked principal's full permission
// ceiling — WITHOUT exposing the client secret. Anchor-only (this hands out
// the service account's live authority) and recorded in the audit trail.
// Nothing is persisted; the token simply expires.
func (s *State) mintToken(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ServiceAccountTokenResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if s.Auth == nil {
		return nil, usecase.Internal("TOKEN", "token minting is not wired", nil)
	}
	sa, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", in.ID)
	}
	if !sa.Active {
		return nil, usecase.Validation("SERVICE_ACCOUNT_INACTIVE",
			"the service account is deactivated — reactivate it before minting a token")
	}
	p, err := s.Principals.FindByServiceAccount(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p == nil {
		return nil, usecase.Internal("PRINCIPAL", "service account has no linked principal", nil)
	}
	if !p.Active {
		return nil, usecase.Validation("SERVICE_ACCOUNT_INACTIVE",
			"the service account's principal is deactivated")
	}

	// Same grant computation as the client_credentials path: the principal's
	// full permission ceiling, flattened from its roles.
	var granted []string
	if s.FlattenPermissions != nil {
		names := make([]string, 0, len(p.Roles))
		for _, ra := range p.Roles {
			names = append(names, ra.Role)
		}
		if granted, err = s.FlattenPermissions(ctx, names); err != nil {
			return nil, usecase.Internal("SCOPE", "permission flattening failed", err)
		}
	}
	token, err := s.Auth.GenerateAccessTokenWithScope(p, granted)
	if err != nil {
		return nil, usecase.Internal("TOKEN", "token mint failed", err)
	}

	// A bearer was handed out for this account — a use of its credentials.
	// Best-effort: a failed stamp must not fail the mint.
	_ = s.Repo.TouchLastUsed(ctx, sa.ID)

	// Best-effort audit: who obtained a credential for which account. Never
	// the token itself.
	if s.Audit != nil {
		actor := ac.PrincipalID
		_ = s.Audit.Insert(ctx, &audit.Log{
			ID:          tsid.Generate(tsid.AuditLog),
			EntityType:  "SERVICE_ACCOUNT",
			EntityID:    sa.ID,
			Operation:   "TOKEN_MINTED_BY_ADMIN",
			PrincipalID: &actor,
			PerformedAt: time.Now().UTC(),
		})
	}

	resp := ServiceAccountTokenResponse{
		AccessToken: token,
		TokenType:   "Bearer",
		// Mirrors the /oauth/token response contract (expires_in 3600).
		ExpiresIn: 3600,
	}
	if len(granted) > 0 {
		j := strings.Join(granted, " ")
		resp.Scope = &j
	}
	return &apicommon.Out[ServiceAccountTokenResponse]{Body: resp}, nil
}

func (s *State) regenerateSigningSecret(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[RegenerateSigningSecretResponse], error) {
	if err := auth.RequireAnchor(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	var secret string
	if _, err := usecaseop.Run(ctx, s.UoW, operations.RegenerateSigningSecret(s.Repo),
		operations.RegenerateSigningSecretCommand{
			ServiceAccountID: in.ID,
			Disclose:         func(plaintext string) { secret = plaintext },
		}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[RegenerateSigningSecretResponse]{Body: RegenerateSigningSecretResponse{
		ID:            in.ID,
		SigningSecret: secret,
	}}, nil
}
