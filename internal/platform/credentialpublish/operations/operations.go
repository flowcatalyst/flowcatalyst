// Package operations holds the credential publishing domain operations:
// switching publishing on and off for a service account or an OAuth client, and
// asking the publisher to try again (docs/credential-publishing.md §7). The
// operations that change a credential queue its publish themselves, in their own
// packages; these are the owner-level choices.
package operations

import (
	"context"
	"errors"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// publication is the small aggregate these operations save: one owner's
// publishing change, carried to its repository's Persist. Choosing this over a
// usecasepgx TxOperation keeps each operation a plain usecaseop.Operation with
// one event, like every other single-aggregate operation, while the writes
// still happen in the commit's transaction (QueueInTx and DisableInTx take
// it): the enable or disable, its items and its event land together or not at all.
type publication struct {
	Owner   credentialpublish.Owner
	Pending credentialpublish.Pending
	Disable bool
}

// IDStr satisfies usecase.HasID.
func (p publication) IDStr() string { return string(p.Owner.Type) + "/" + p.Owner.ID }

// publicationRepo persists a publication change in the commit's transaction.
// There is no aggregate row to delete.
type publicationRepo struct{}

func (publicationRepo) Persist(ctx context.Context, p *publication, tx *usecasepgx.DbTx) error {
	if p.Disable {
		return credentialpublish.DisableInTx(ctx, tx.Inner(), p.Owner)
	}
	// No encryption service: enabling queues only the "not known" client secret,
	// never a value to hold.
	return credentialpublish.QueueInTx(ctx, tx.Inner(), nil, p.Owner, p.Pending)
}

func (publicationRepo) Delete(context.Context, *publication, *usecasepgx.DbTx) error {
	return errors.New("a publication is switched off, not deleted")
}

// EnableCommand switches publishing on for one owner.
type EnableCommand struct {
	OwnerType credentialpublish.OwnerType `json:"ownerType"`
	// OwnerID is the service account's or OAuth client's row id.
	OwnerID string `json:"ownerId"`
	// BasePath replaces the template's path before {credential}; it must lie
	// under the deployment's prefix.
	BasePath *string `json:"basePath,omitempty"`
}

// DisableCommand switches publishing off for one owner.
type DisableCommand struct {
	OwnerType credentialpublish.OwnerType `json:"ownerType"`
	OwnerID   string                      `json:"ownerId"`
}

func validateOwner(t credentialpublish.OwnerType, id string) error {
	if t != credentialpublish.OwnerServiceAccount && t != credentialpublish.OwnerOAuthClient {
		return usecase.Validation("OWNER_TYPE_INVALID", "ownerType must be SERVICE_ACCOUNT or OAUTH_CLIENT")
	}
	if strings.TrimSpace(id) == "" {
		return usecase.Validation("OWNER_ID_REQUIRED", "ownerId is required")
	}
	return nil
}

// ownerClient is an OAuth client as the operations need it: it must exist, be
// confidential, and not belong to a service account.
func ownerClient(ctx context.Context, clients *auth.OAuthClientRepo, id string) (*auth.OAuthClient, error) {
	c, err := clients.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("OAuthClient", id)
	}
	if c.ClientType != auth.OAuthClientConfidential {
		return nil, usecase.Conflict(credentialpublish.CodeNotConfidential,
			"only a CONFIDENTIAL client has credentials to publish")
	}
	if c.PrincipalID != nil && *c.PrincipalID != "" {
		linked, err := clients.IsServiceAccountPrincipal(ctx, *c.PrincipalID)
		if err != nil {
			return nil, usecase.Internal("REPO", "principal lookup failed", err)
		}
		if linked {
			return nil, usecase.Conflict(credentialpublish.CodeLinked,
				"this client belongs to a service account and is published with it")
		}
	}
	return c, nil
}

// EnableCredentialPublishing switches publishing on for a service account or a
// stand-alone confidential OAuth client and emits [CredentialPublishingEnabled].
// What the platform can read back (client id, auth token, signing secret) is
// queued to publish now; the client secret, whose plaintext it no longer has,
// is recorded as not publishable until the next rotation (spec §3). Platform
// admin config (Authorize: Public); the controllers gate on the owner's own
// write permission.
func EnableCredentialPublishing(
	cfg credentialpublish.Config,
	accounts *serviceaccount.Repository,
	principals *principal.Repository,
	clients *auth.OAuthClientRepo,
	apps *application.Repository,
) usecaseop.Operation[EnableCommand, CredentialPublishingEnabled] {
	return usecaseop.Operation[EnableCommand, CredentialPublishingEnabled]{
		Name: "EnableCredentialPublishing",
		Validate: func(_ context.Context, cmd EnableCommand) error {
			if !cfg.Enabled() {
				return usecase.Validation(credentialpublish.CodeUnavailable,
					"credential publishing is not configured on this platform")
			}
			return validateOwner(cmd.OwnerType, cmd.OwnerID)
		},
		Authorize: usecaseop.Public[EnableCommand],
		Execute: func(ctx context.Context, cmd EnableCommand, ec usecase.ExecutionContext) (usecaseop.Plan[CredentialPublishingEnabled], error) {
			owner := credentialpublish.Owner{Type: cmd.OwnerType, ID: cmd.OwnerID}
			var vars credentialpublish.Vars
			var changes []credentialpublish.Change
			switch owner.Type {
			case credentialpublish.OwnerServiceAccount:
				sa, err := accounts.FindByID(ctx, owner.ID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_id failed", err)
				}
				if sa == nil {
					return nil, httperror.NotFound("ServiceAccount", owner.ID)
				}
				vars.Account = credentialpublish.AccountSegment(sa.Code)
				if sa.ApplicationID != nil {
					app, err := apps.FindByID(ctx, string(*sa.ApplicationID))
					if err != nil {
						return nil, usecase.Internal("REPO", "find_application failed", err)
					}
					if app != nil {
						vars.App = app.Code
					}
				}
				hasClient, err := accountHasClient(ctx, principals, clients, sa.ID)
				if err != nil {
					return nil, err
				}
				if hasClient {
					changes = append(changes, credentialpublish.Change{Credential: credentialpublish.CredClientID})
				}
				creds := sa.WebhookCredentials
				if (creds.AuthType == serviceaccount.AuthBearer || creds.AuthType == serviceaccount.AuthAPIKey) &&
					creds.Token != nil && *creds.Token != "" {
					changes = append(changes, credentialpublish.Change{Credential: credentialpublish.CredAuthToken})
				}
				if creds.SigningSecret != nil && *creds.SigningSecret != "" {
					changes = append(changes, credentialpublish.Change{Credential: credentialpublish.CredSigningSecret})
				}
				if hasClient {
					changes = append(changes, credentialpublish.Change{Credential: credentialpublish.CredClientSecret, Unavailable: true})
				}
			case credentialpublish.OwnerOAuthClient:
				c, err := ownerClient(ctx, clients, owner.ID)
				if err != nil {
					return nil, err
				}
				vars.OAuthClient = c.ClientID
				changes = []credentialpublish.Change{
					{Credential: credentialpublish.CredClientID},
					{Credential: credentialpublish.CredClientSecret, Unavailable: true},
				}
			default:
				// Validate refuses any other owner type; kept so a new one cannot slip through.
				return nil, usecase.Validation("OWNER_TYPE_INVALID", "ownerType must be SERVICE_ACCOUNT or OAUTH_CLIENT")
			}
			base, err := cfg.ResolveBasePath(owner.Type, vars, cmd.BasePath)
			if err != nil {
				return nil, err
			}
			agg := &publication{Owner: owner, Pending: credentialpublish.Pending{
				Enable:  &credentialpublish.EnableRequest{BasePath: base, EnabledBy: ec.PrincipalID},
				Changes: changes,
			}}
			event := CredentialPublishingEnabled{
				Metadata:  usecase.NewEventMetadata(ec, CredentialPublishingEnabledType, Source, subjectFor(owner)),
				OwnerType: string(owner.Type),
				OwnerID:   owner.ID,
				BasePath:  base,
			}
			return usecaseop.Save(agg, publicationRepo{}, event), nil
		},
	}
}

// accountHasClient reports whether the service account has an OAuth client
// wired to its principal.
func accountHasClient(ctx context.Context, principals *principal.Repository, clients *auth.OAuthClientRepo, accountID string) (bool, error) {
	p, err := principals.FindByServiceAccount(ctx, accountID)
	if err != nil {
		return false, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p == nil {
		return false, nil
	}
	cs, err := clients.FindByPrincipalID(ctx, string(p.ID))
	if err != nil {
		return false, usecase.Internal("REPO", "find_by_principal failed", err)
	}
	return len(cs) > 0, nil
}

// DisableCredentialPublishing switches publishing off for an owner: every
// published parameter is queued for deletion (the credentials on the platform
// are untouched) and [CredentialPublishingDisabled] names the paths. A client
// linked to a service account has no publication of its own to switch off.
func DisableCredentialPublishing(
	accounts *serviceaccount.Repository,
	clients *auth.OAuthClientRepo,
	pubs *credentialpublish.Repository,
) usecaseop.Operation[DisableCommand, CredentialPublishingDisabled] {
	return usecaseop.Operation[DisableCommand, CredentialPublishingDisabled]{
		Name: "DisableCredentialPublishing",
		Validate: func(_ context.Context, cmd DisableCommand) error {
			return validateOwner(cmd.OwnerType, cmd.OwnerID)
		},
		Authorize: usecaseop.Public[DisableCommand],
		Execute: func(ctx context.Context, cmd DisableCommand, ec usecase.ExecutionContext) (usecaseop.Plan[CredentialPublishingDisabled], error) {
			owner := credentialpublish.Owner{Type: cmd.OwnerType, ID: cmd.OwnerID}
			if owner.Type == credentialpublish.OwnerServiceAccount {
				sa, err := accounts.FindByID(ctx, owner.ID)
				if err != nil {
					return nil, usecase.Internal("REPO", "find_by_id failed", err)
				}
				if sa == nil {
					return nil, httperror.NotFound("ServiceAccount", owner.ID)
				}
			} else if _, err := ownerClient(ctx, clients, owner.ID); err != nil {
				return nil, err
			}
			pub, items, err := pubs.Status(ctx, owner)
			if err != nil {
				return nil, usecase.Internal("REPO", "publication status failed", err)
			}
			if pub == nil {
				return nil, usecase.BusinessRule(credentialpublish.CodeNotEnabled, "credential publishing is not switched on")
			}
			paths := make([]string, 0, len(items))
			for _, it := range items {
				if it.State != credentialpublish.StateNotPublishable {
					paths = append(paths, it.Path)
				}
			}
			event := CredentialPublishingDisabled{
				Metadata:  usecase.NewEventMetadata(ec, CredentialPublishingDisabledType, Source, subjectFor(owner)),
				OwnerType: string(owner.Type),
				OwnerID:   owner.ID,
				Paths:     paths,
			}
			return usecaseop.Save(&publication{Owner: owner, Disable: true}, publicationRepo{}, event), nil
		},
	}
}

// RetryCredentialPublishing makes the owner's waiting items due now. It is a
// plain function, not an operation: it changes no credential or setting, only
// when the publisher next looks, so spec §7 gives it no event, and an
// operation without an event has no envelope to run in.
func RetryCredentialPublishing(ctx context.Context, pubs *credentialpublish.Repository, owner credentialpublish.Owner) error {
	if err := validateOwner(owner.Type, owner.ID); err != nil {
		return err
	}
	if err := pubs.RetryNow(ctx, owner); err != nil {
		return usecase.Internal("REPO", "retry failed", err)
	}
	return nil
}
