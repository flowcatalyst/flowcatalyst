package pages

import (
	"context"
	"errors"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	idpops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// IdpStore is the slice of the identity provider repository the pages read
// (the calls GET /api/identity-providers and GET /api/identity-providers/{id}
// make).
type IdpStore interface {
	FindAll(ctx context.Context) ([]identityprovider.IdentityProvider, error)
	FindByID(ctx context.Context, id string) (*identityprovider.IdentityProvider, error)
}

// IdpCommands are the identity provider operations the page runs; IdpOps runs
// the API's use cases. The permission check is the page's, as it is the API
// handler's: the use cases are Authorize-Public.
type IdpCommands interface {
	Create(ctx context.Context, cmd idpops.CreateCommand) (idpops.CreateResult, error)
	Update(ctx context.Context, cmd idpops.UpdateCommand) (idpops.UpdateResult, error)
	Delete(ctx context.Context, id string) error
}

// IdpOps runs the identity provider use cases through the unit of work,
// exactly as internal/platform/identityprovider/api does, including the
// encryption of a plaintext client secret before it is stored.
type IdpOps struct {
	Repo       *identityprovider.Repository
	Mappings   *emaildomainmapping.Repository
	Principals *principal.Repository
	// Enc may be nil (FLOWCATALYST_APP_KEY unset): saving a plaintext secret
	// is then refused, never stored as given.
	Enc *encryption.Service
	UoW *usecasepgx.UnitOfWork
}

func (o IdpOps) deps() idpops.Deps {
	return idpops.Deps{Repo: o.Repo, MoveDeps: edmops.MoveDeps{Mappings: o.Mappings, IDPs: o.Repo, Principals: o.Principals}}
}

// encryptSecretRef is the API handler's mapping of the encryption errors to
// the validation and internal envelopes.
func (o IdpOps) encryptSecretRef(ref *string) (*string, error) {
	out, err := encryption.EncryptSecretRef(o.Enc, ref)
	switch {
	case errors.Is(err, encryption.ErrNotConfigured):
		return nil, usecase.Validation("ENCRYPTION_NOT_CONFIGURED",
			"cannot store OIDC client secret: FLOWCATALYST_APP_KEY is not configured")
	case errors.Is(err, encryption.ErrUnsupportedScheme):
		return nil, usecase.Validation("UNSUPPORTED_SECRET_SCHEME", err.Error())
	case err != nil:
		return nil, usecase.Internal("ENCRYPT", "encrypt OIDC client secret", err)
	}
	return out, nil
}

// Create runs CreateIdentityProvider.
func (o IdpOps) Create(ctx context.Context, cmd idpops.CreateCommand) (idpops.CreateResult, error) {
	secret, err := o.encryptSecretRef(cmd.OIDCClientSecretRef)
	if err != nil {
		return idpops.CreateResult{}, err
	}
	cmd.OIDCClientSecretRef = secret
	return usecaseop.RunTx(ctx, o.UoW, idpops.CreateIdentityProvider(o.deps()), cmd, auth.NewExecutionContext(ctx))
}

// Update runs UpdateIdentityProvider.
func (o IdpOps) Update(ctx context.Context, cmd idpops.UpdateCommand) (idpops.UpdateResult, error) {
	secret, err := o.encryptSecretRef(cmd.OIDCClientSecretRef)
	if err != nil {
		return idpops.UpdateResult{}, err
	}
	cmd.OIDCClientSecretRef = secret
	return usecaseop.RunTx(ctx, o.UoW, idpops.UpdateIdentityProvider(o.deps()), cmd, auth.NewExecutionContext(ctx))
}

// Delete runs DeleteIdentityProvider.
func (o IdpOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, idpops.DeleteIdentityProvider(o.deps()), idpops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}
