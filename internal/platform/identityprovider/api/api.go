// Package api wires the HTTP routes for the identity_provider subdomain
// via danielgtaylor/huma/v2. Anchor-only.
package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps for the identity_provider handlers.
type State struct {
	Repo *identityprovider.Repository
	UoW  *usecasepgx.UnitOfWork
	// Enc encrypts the OIDC client secret before it is persisted. Without
	// it, a plaintext secret would be stored verbatim (and then fail to
	// decrypt at login). May be nil when FLOWCATALYST_APP_KEY is unset, in
	// which case saving a plaintext secret is rejected.
	Enc *encryption.Service
}

// encryptSecretRef encrypts a plaintext OIDC client secret inline before it is
// stored (see encryption.EncryptSecretRef), mapping its errors to the API's
// validation/internal envelopes.
func encryptSecretRef(enc *encryption.Service, ref *string) (*string, error) {
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

const tag = "identity-providers"

// Register mounts the IDP endpoints on the supplied huma API.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listIdentityProviders", "/api/identity-providers", "List identity providers", s.list)
	apiroute.Post(g, "createIdentityProvider", "/api/identity-providers", "Create an identity provider", http.StatusCreated, s.create)
	apiroute.Get(g, "getIdentityProvider", "/api/identity-providers/{id}", "Get an identity provider by id", s.getByID)
	apiroute.Put(g, "updateIdentityProvider", "/api/identity-providers/{id}", "Update an identity provider", http.StatusOK, s.update)
	apiroute.Delete(g, "deleteIdentityProvider", "/api/identity-providers/{id}", "Delete an identity provider", http.StatusNoContent, s.delete)
}

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[IdentityProviderListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[IdentityProviderListResponse]{Body: IdentityProviderListResponse{IdentityProviders: out, Total: len(out)}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[IdentityProviderResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		return nil, err
	}
	ip, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", in.ID)
	}
	return &apicommon.Out[IdentityProviderResponse]{Body: fromEntity(ip)}, nil
}

// create returns the full provider (201), not just `{id}`: the SPA's
// create toast reads the response `name`, and a bare id renders as
// "undefined".
func (s *State) create(ctx context.Context, in *apicommon.In[CreateIdentityProviderRequest]) (*apicommon.Out[IdentityProviderResponse], error) {
	// Coarse anchor-only permission at the controller; the use case enforces
	// validation and uniqueness. The handler also encrypts the secret.
	if err := auth.CanWriteIdentityProviders(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	secretRef, err := encryptSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateIdentityProvider(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	id := event.IdentityProviderID
	ip, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create reload failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", id)
	}
	return &apicommon.Out[IdentityProviderResponse]{Body: fromEntity(ip)}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateIdentityProviderRequest
}

// update returns the updated provider with 200 (not 204): the SPA's
// detail page sets `provider.value = updated` after PUT, and its card is
// gated on a truthy provider — a 204/undefined collapses the view.
func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Out[IdentityProviderResponse], error) {
	if err := auth.CanWriteIdentityProviders(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	secretRef, err := encryptSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateIdentityProvider(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	ip, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-update reload failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", in.ID)
	}
	return &apicommon.Out[IdentityProviderResponse]{Body: fromEntity(ip)}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteIdentityProviders(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteIdentityProvider(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
