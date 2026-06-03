// Package api wires the HTTP routes for the identity_provider subdomain
// via danielgtaylor/huma/v2. Anchor-only.
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
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

// externalSecretSchemes are secret-manager reference prefixes that are stored
// verbatim and resolved at read time — never encrypted inline.
var externalSecretSchemes = []string{"aws-sm://", "aws-ps://", "gcp-sm://", "vault://", "env://", "literal:"}

// encryptSecretRef converts an incoming OIDC client-secret value into its
// at-rest form. A plaintext secret — optionally carrying the SecretRefInput
// "encrypt:" directive — is encrypted inline as "encrypted:<blob>", matching
// the Rust/TS producers (which Decrypt already reads). Already-encrypted blobs
// and external provider references pass through unchanged. A nil pointer
// (field omitted) is preserved so an update leaves the stored secret untouched.
func encryptSecretRef(enc *encryption.Service, ref *string) (*string, error) {
	if ref == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*ref)
	if v == "" {
		return ref, nil // empty clears the secret; preserve as-is
	}
	if strings.HasPrefix(v, "encrypted:") {
		return &v, nil // already an inline ciphertext — idempotent
	}
	for _, scheme := range externalSecretSchemes {
		if strings.HasPrefix(v, scheme) {
			return &v, nil // external reference, resolved at read time
		}
	}
	// Plaintext (optionally with the "encrypt:" directive) → encrypt inline.
	plaintext := strings.TrimPrefix(v, "encrypt:")
	if enc == nil {
		return nil, usecase.Validation("ENCRYPTION_NOT_CONFIGURED",
			"cannot store OIDC client secret: FLOWCATALYST_APP_KEY is not configured")
	}
	blob, err := enc.Encrypt(plaintext)
	if err != nil {
		return nil, usecase.Internal("ENCRYPT", "encrypt OIDC client secret", err)
	}
	out := "encrypted:" + blob
	return &out, nil
}

const tag = "identity-providers"

// Register mounts the IDP endpoints on the supplied huma API.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listIdentityProviders",
		Method:        http.MethodGet,
		Path:          "/api/identity-providers",
		Summary:       "List identity providers",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createIdentityProvider",
		Method:        http.MethodPost,
		Path:          "/api/identity-providers",
		Summary:       "Create an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getIdentityProvider",
		Method:        http.MethodGet,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Get an identity provider by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateIdentityProvider",
		Method:        http.MethodPut,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Update an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteIdentityProvider",
		Method:        http.MethodDelete,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Delete an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type emptyInput struct{}

type listOutput struct {
	Body IdentityProviderListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]IdentityProviderResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: IdentityProviderListResponse{IdentityProviders: out, Total: len(out)}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body IdentityProviderResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(ip)}, nil
}

type createInput struct {
	Body CreateIdentityProviderRequest
}

type createOutput struct {
	Body IdentityProviderResponse
}

// create returns the full provider (201), not just `{id}`: the SPA's
// create toast reads the response `name`, and a bare id renders as
// "undefined".
func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	secretRef, err := encryptSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateIdentityProvider(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	id := committed.Event().IdentityProviderID
	ip, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create reload failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", id)
	}
	return &createOutput{Body: fromEntity(ip)}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateIdentityProviderRequest
}

type updateOutput struct {
	Body IdentityProviderResponse
}

type emptyOutput struct{}

// update returns the updated provider with 200 (not 204): the SPA's
// detail page sets `provider.value = updated` after PUT, and its card is
// gated on a truthy provider — a 204/undefined collapses the view.
func (s *State) update(ctx context.Context, in *updateInput) (*updateOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	secretRef, err := encryptSecretRef(s.Enc, in.Body.OIDCClientSecretRef)
	if err != nil {
		return nil, err
	}
	in.Body.OIDCClientSecretRef = secretRef
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateIdentityProvider(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	ip, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-update reload failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", in.ID)
	}
	return &updateOutput{Body: fromEntity(ip)}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteIdentityProvider(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
