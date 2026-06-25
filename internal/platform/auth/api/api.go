// Package api wires admin HTTP routes for the auth subdomain via huma.
// Runtime routes (/oauth/token, /oauth/authorize, /.well-known/*, OIDC
// login/callback) remain registered by the provider package.
package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *auth.Repository
	// Applications resolves oauth_client_application_ids to {id, name}
	// display refs for OAuthClientResponse.Applications. Optional: nil
	// leaves Applications empty (clients can still use applicationIds).
	Applications *application.Repository
	UoW          *usecasepgx.UnitOfWork
	// Enc encrypts the auth-config OIDC client secret before it is persisted,
	// so a plaintext secret is never stored verbatim. May be nil when
	// FLOWCATALYST_APP_KEY is unset (saving a plaintext secret is then rejected).
	Enc *encryption.Service
}

// encryptOIDCSecretRef encrypts a plaintext OIDC client secret inline before
// storage (see encryption.EncryptSecretRef), mapping errors to API envelopes.
func encryptOIDCSecretRef(enc *encryption.Service, ref *string) (*string, error) {
	out, err := encryption.EncryptSecretRef(enc, ref)
	switch {
	case errors.Is(err, encryption.ErrNotConfigured):
		return nil, usecase.Validation("ENCRYPTION_NOT_CONFIGURED",
			"cannot store OIDC client secret: FLOWCATALYST_APP_KEY is not configured")
	case err != nil:
		return nil, usecase.Internal("ENCRYPT", "encrypt OIDC client secret", err)
	}
	return out, nil
}

// fillApplicationRefs populates each response's Applications ({id,name})
// from its ApplicationIDs, resolving names via the application repo in a
// single deduped pass. Unresolved ids (e.g. a deleted application) fall
// back to the id as the name so the SPA still renders a chip. No-op when
// the application repo isn't wired.
func (s *State) fillApplicationRefs(ctx context.Context, resps ...*OAuthClientResponse) error {
	if s.Applications == nil {
		return nil
	}
	idSet := map[string]struct{}{}
	for _, r := range resps {
		for _, id := range r.ApplicationIDs {
			if id != "" {
				idSet[id] = struct{}{}
			}
		}
	}
	if len(idSet) == 0 {
		return nil
	}
	nameByID := make(map[string]string, len(idSet))
	for id := range idSet {
		app, err := s.Applications.FindByID(ctx, id)
		if err != nil {
			return usecase.Internal("REPO", "find_application failed", err)
		}
		if app != nil {
			nameByID[id] = app.Name
		}
	}
	for _, r := range resps {
		r.Applications = apicommon.MapSlice(r.ApplicationIDs, func(id *string) OAuthClientApplicationRef {
			name := nameByID[*id]
			if name == "" {
				name = *id
			}
			return OAuthClientApplicationRef{ID: *id, Name: name}
		})
	}
	return nil
}

const (
	tagOAuth          = "oauth-clients"
	tagAnchorDomains  = "anchor-domains"
	tagAuthConfigs    = "auth-configs"
	tagIdpRoleMapping = "idp-role-mappings"
)

// Register mounts the auth admin endpoints. Anchor-only.
func Register(api huma.API, s *State) {
	gClients := apiroute.New(api, tagOAuth)
	gDomains := apiroute.New(api, tagAnchorDomains)
	gConfigs := apiroute.New(api, tagAuthConfigs)
	gMappings := apiroute.New(api, tagIdpRoleMapping)

	// OAuth clients
	apiroute.Get(gClients, "listOAuthClients", "/api/oauth-clients", "List OAuth clients", s.listOAuthClients)
	apiroute.Post(gClients, "createOAuthClient", "/api/oauth-clients", "Create an OAuth client", http.StatusCreated, s.createOAuthClient)
	apiroute.Get(gClients, "getOAuthClient", "/api/oauth-clients/{id}", "Get an OAuth client by id", s.getOAuthClient)
	apiroute.Put(gClients, "updateOAuthClient", "/api/oauth-clients/{id}", "Update an OAuth client", http.StatusNoContent, s.updateOAuthClient)
	apiroute.Post(gClients, "activateOAuthClient", "/api/oauth-clients/{id}/activate", "Activate an OAuth client", http.StatusOK, s.activateOAuthClient)
	apiroute.Post(gClients, "deactivateOAuthClient", "/api/oauth-clients/{id}/deactivate", "Deactivate an OAuth client", http.StatusOK, s.deactivateOAuthClient)
	apiroute.Post(gClients, "rotateOAuthClientSecret", "/api/oauth-clients/{id}/rotate-secret", "Rotate an OAuth client's secret", http.StatusOK, s.rotateOAuthClientSecret)
	// SDK-compatibility aliases. The Laravel/Rust client calls
	// /api/oauth-clients/{id}/regenerate-secret (same as rotate-secret) and
	// looks clients up by their client_id via /by-client-id/{clientId}.
	apiroute.Post(gClients, "regenerateOAuthClientSecret", "/api/oauth-clients/{id}/regenerate-secret", "Regenerate an OAuth client's secret (SDK alias of rotate-secret)", http.StatusOK, s.rotateOAuthClientSecret)
	apiroute.Get(gClients, "getOAuthClientByClientID", "/api/oauth-clients/by-client-id/{clientId}", "Get an OAuth client by its client_id (SDK lookup)", s.getOAuthClientByClientID)
	apiroute.Delete(gClients, "deleteOAuthClient", "/api/oauth-clients/{id}", "Delete an OAuth client", http.StatusNoContent, s.deleteOAuthClient)

	// Anchor domains
	apiroute.Get(gDomains, "listAnchorDomains", "/api/anchor-domains", "List anchor domains", s.listAnchorDomains)
	apiroute.Post(gDomains, "createAnchorDomain", "/api/anchor-domains", "Create an anchor domain", http.StatusCreated, s.createAnchorDomain)
	apiroute.Put(gDomains, "updateAnchorDomain", "/api/anchor-domains/{id}", "Update an anchor domain", http.StatusNoContent, s.updateAnchorDomain)
	apiroute.Delete(gDomains, "deleteAnchorDomain", "/api/anchor-domains/{id}", "Delete an anchor domain", http.StatusNoContent, s.deleteAnchorDomain)

	// Auth configs
	apiroute.Get(gConfigs, "listAuthConfigs", "/api/auth-configs", "List client auth configs", s.listAuthConfigs)
	apiroute.Post(gConfigs, "createAuthConfig", "/api/auth-configs", "Create a client auth config", http.StatusCreated, s.createAuthConfig)
	apiroute.Put(gConfigs, "updateAuthConfig", "/api/auth-configs/{id}", "Update a client auth config", http.StatusNoContent, s.updateAuthConfig)
	apiroute.Delete(gConfigs, "deleteAuthConfig", "/api/auth-configs/{id}", "Delete a client auth config", http.StatusNoContent, s.deleteAuthConfig)

	// IDP role mappings
	apiroute.Get(gMappings, "listIdpRoleMappings", "/api/idp-role-mappings", "List IDP role mappings", s.listIdpRoleMappings)
	apiroute.Post(gMappings, "createIdpRoleMapping", "/api/idp-role-mappings", "Create an IDP role mapping", http.StatusCreated, s.createIdpRoleMapping)
	apiroute.Delete(gMappings, "deleteIdpRoleMapping", "/api/idp-role-mappings/{id}", "Delete an IDP role mapping", http.StatusNoContent, s.deleteIdpRoleMapping)
}

// ── shared helpers ────────────────────────────────────────────────────────

func authedAnchor(ctx context.Context) (*platformauth.AuthContext, error) {
	ac := platformauth.FromContext(ctx)
	if err := platformauth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	return ac, nil
}

// ── OAuthClient ───────────────────────────────────────────────────────────

func (s *State) listOAuthClients(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[OAuthClientListResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Repo.OAuthClients.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, oauthClientFromEntity)
	ptrs := make([]*OAuthClientResponse, len(out))
	for i := range out {
		ptrs[i] = &out[i]
	}
	if err := s.fillApplicationRefs(ctx, ptrs...); err != nil {
		return nil, err
	}
	return &apicommon.Out[OAuthClientListResponse]{Body: OAuthClientListResponse{Clients: out}}, nil
}

func (s *State) getOAuthClient(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[OAuthClientResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	c, err := s.Repo.OAuthClients.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("OAuthClient", in.ID)
	}
	resp := oauthClientFromEntity(c)
	if err := s.fillApplicationRefs(ctx, &resp); err != nil {
		return nil, err
	}
	return &apicommon.Out[OAuthClientResponse]{Body: resp}, nil
}

type clientIDPathInput struct {
	ClientID string `path:"clientId"`
}

// getOAuthClientByClientID backs GET /api/oauth-clients/by-client-id/{clientId}
// (SDK lookup by the OAuth client_id rather than the internal TSID).
func (s *State) getOAuthClientByClientID(ctx context.Context, in *clientIDPathInput) (*apicommon.Out[OAuthClientResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	c, err := s.Repo.OAuthClients.FindByClientID(ctx, in.ClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_client_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("OAuthClient", in.ClientID)
	}
	resp := oauthClientFromEntity(c)
	if err := s.fillApplicationRefs(ctx, &resp); err != nil {
		return nil, err
	}
	return &apicommon.Out[OAuthClientResponse]{Body: resp}, nil
}

func (s *State) createOAuthClient(ctx context.Context, in *apicommon.In[CreateOAuthClientRequest]) (*apicommon.Out[CreateOAuthClientResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateOAuthClient(s.Repo.OAuthClients), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	// Re-fetch the persisted client so the SPA receives the full
	// OAuthClientResponse under `client` (oauth-clients.ts:56). Matches
	// Rust oauth_clients_api.rs:294-305.
	c, err := s.Repo.OAuthClients.FindByID(ctx, event.OAuthClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, usecase.Internal("REPO", "oauth client created but row not found", nil)
	}
	resp := CreateOAuthClientResponse{Client: oauthClientFromEntity(c)}
	if err := s.fillApplicationRefs(ctx, &resp.Client); err != nil {
		return nil, err
	}
	if plaintext, ok := operations.PopStashedSecret(event.OAuthClientID); ok {
		resp.ClientSecret = plaintext
	}
	return &apicommon.Out[CreateOAuthClientResponse]{Body: resp}, nil
}

type updateOAuthClientInput struct {
	ID   string `path:"id"`
	Body UpdateOAuthClientRequest
}

func (s *State) updateOAuthClient(ctx context.Context, in *updateOAuthClientInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateOAuthClient(s.Repo.OAuthClients), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// activate/deactivate carry the {success, message?} body, matching Rust's
// SuccessResponse. The SPA reads `.message` (oauth-clients.ts:109-117), which
// is preserved. Returns 200 + body rather than 204 so apiFetch does not
// resolve to undefined.
func (s *State) activateOAuthClient(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.SuccessResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ActivateOAuthClient(s.Repo.OAuthClients),
		operations.ActivateOAuthClientCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.SuccessResponse]{Body: apicommon.SuccessResponse{Success: true, Message: "OAuth client activated"}}, nil
}

func (s *State) deactivateOAuthClient(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[apicommon.SuccessResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeactivateOAuthClient(s.Repo.OAuthClients),
		operations.DeactivateOAuthClientCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.SuccessResponse]{Body: apicommon.SuccessResponse{Success: true, Message: "OAuth client deactivated"}}, nil
}

func (s *State) rotateOAuthClientSecret(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[RotateOAuthClientSecretResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.RotateOAuthClientSecret(s.Repo.OAuthClients),
		operations.RotateOAuthClientSecretCommand{ID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	// The SPA expects the public client_id string, not the internal id
	// (oauth-clients.ts:62-65). Re-fetch to obtain it.
	c, err := s.Repo.OAuthClients.FindByID(ctx, event.OAuthClientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("OAuthClient", event.OAuthClientID)
	}
	resp := RotateOAuthClientSecretResponse{ClientID: c.ClientID}
	if plaintext, ok := operations.PopStashedSecret(event.OAuthClientID); ok {
		resp.ClientSecret = plaintext
	}
	return &apicommon.Out[RotateOAuthClientSecretResponse]{Body: resp}, nil
}

func (s *State) deleteOAuthClient(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteOAuthClient(s.Repo.OAuthClients),
		operations.DeleteOAuthClientCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── AnchorDomain ──────────────────────────────────────────────────────────

func (s *State) listAnchorDomains(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AnchorDomainListResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Repo.AnchorDomains.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, anchorDomainFromEntity)
	return &apicommon.Out[AnchorDomainListResponse]{Body: AnchorDomainListResponse{Items: out}}, nil
}

func (s *State) createAnchorDomain(ctx context.Context, in *apicommon.In[CreateAnchorDomainRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateAnchorDomain(s.Repo.AnchorDomains), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.AnchorDomainID}}, nil
}

type updateAnchorDomainInput struct {
	ID   string `path:"id"`
	Body UpdateAnchorDomainRequest
}

func (s *State) updateAnchorDomain(ctx context.Context, in *updateAnchorDomainInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateAnchorDomain(s.Repo.AnchorDomains), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) deleteAnchorDomain(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteAnchorDomain(s.Repo.AnchorDomains),
		operations.DeleteAnchorDomainCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── AuthConfig ────────────────────────────────────────────────────────────

func (s *State) listAuthConfigs(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[AuthConfigListResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Repo.ClientAuthConfigs.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, authConfigFromEntity)
	return &apicommon.Out[AuthConfigListResponse]{Body: AuthConfigListResponse{Items: out}}, nil
}

func (s *State) createAuthConfig(ctx context.Context, in *apicommon.In[CreateAuthConfigRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	secretRef, err := encryptOIDCSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := platformauth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateAuthConfig(s.Repo.ClientAuthConfigs), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.AuthConfigID}}, nil
}

type updateAuthConfigInput struct {
	ID   string `path:"id"`
	Body UpdateAuthConfigRequest
}

func (s *State) updateAuthConfig(ctx context.Context, in *updateAuthConfigInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	secretRef, err := encryptOIDCSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateAuthConfig(s.Repo.ClientAuthConfigs), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) deleteAuthConfig(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteAuthConfig(s.Repo.ClientAuthConfigs),
		operations.DeleteAuthConfigCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── IdpRoleMapping ────────────────────────────────────────────────────────

func (s *State) listIdpRoleMappings(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[IdpRoleMappingListResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	rows, err := s.Repo.IdpRoleMappings.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, idpRoleMappingFromEntity)
	return &apicommon.Out[IdpRoleMappingListResponse]{Body: IdpRoleMappingListResponse{Items: out}}, nil
}

func (s *State) createIdpRoleMapping(ctx context.Context, in *apicommon.In[CreateIdpRoleMappingRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateIdpRoleMapping(s.Repo.IdpRoleMappings), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.MappingID}}, nil
}

func (s *State) deleteIdpRoleMapping(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if _, err := authedAnchor(ctx); err != nil {
		return nil, err
	}
	ec := platformauth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteIdpRoleMapping(s.Repo.IdpRoleMappings),
		operations.DeleteIdpRoleMappingCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
