package operations

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// MoveProviderCommand re-points an email domain to a different identity
// provider.
type MoveProviderCommand struct {
	ID                 string `json:"id"`
	IdentityProviderID string `json:"identityProviderId"`
}

// MoveProviderResult summarises the move for the caller.
type MoveProviderResult struct {
	MappingID              string `json:"mappingId"`
	EmailDomain            string `json:"emailDomain"`
	FromIdentityProviderID string `json:"fromIdentityProviderId"`
	ToIdentityProviderID   string `json:"toIdentityProviderId"`
	// UsersReset is how many OIDC-provisioned USER principals on the domain
	// were converted back to internal auth (provider marker cleared, IDP_SYNC
	// roles dropped). Zero when moving toward an OIDC provider.
	UsersReset int `json:"usersReset"`
}

// MoveDeps bundles the repositories the move behaviour needs.
type MoveDeps struct {
	Mappings   *emaildomainmapping.Repository
	IDPs       *identityprovider.Repository
	Principals *principal.Repository
}

// MoveMappingToProvider is the domain operation behind "this domain now
// authenticates elsewhere". It re-points the mapping and applies the
// direction-specific side effects:
//
//   - → OIDC provider: routing flips at the next login; existing principals
//     are matched by email at the OIDC callback, so nothing else changes.
//   - → INTERNAL provider: the domain's OIDC-provisioned users (idp_type =
//     "OIDC") are converted back to internal auth — provider marker cleared so
//     the password-reset flow accepts them again (they have no password hash),
//     external IDP id dropped, and their IDP_SYNC-sourced roles removed (the
//     provider that vouched for those roles no longer authenticates the
//     domain, and nothing would ever reconcile them again).
//
// The coarse anchor check lives on the controller (same as the other mapping
// ops).
func MoveMappingToProvider(deps MoveDeps) usecaseop.TxOperation[MoveProviderCommand, MoveProviderResult] {
	return usecaseop.TxOperation[MoveProviderCommand, MoveProviderResult]{
		Name: "MoveMappingToProvider",
		Validate: func(_ context.Context, cmd MoveProviderCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if strings.TrimSpace(cmd.IdentityProviderID) == "" {
				return usecase.Validation("IDP_REQUIRED", "identityProviderId is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[MoveProviderCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd MoveProviderCommand, ec usecase.ExecutionContext) (MoveProviderResult, error) {
			var zero MoveProviderResult

			m, err := deps.Mappings.FindByID(ctx, cmd.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if m == nil {
				return zero, httperror.NotFound("EmailDomainMapping", cmd.ID)
			}
			if m.IdentityProviderID == cmd.IdentityProviderID {
				return zero, usecase.Conflict("ALREADY_ON_PROVIDER",
					"Email domain '"+m.EmailDomain+"' is already mapped to that identity provider")
			}
			target, err := deps.IDPs.FindByID(ctx, cmd.IdentityProviderID)
			if err != nil {
				return zero, usecase.Internal("REPO", "identity_provider lookup failed", err)
			}
			if target == nil {
				return zero, httperror.NotFound("IdentityProvider", cmd.IdentityProviderID)
			}

			from := m.IdentityProviderID
			usersReset, err := MoveMappingTx(ctx, s, deps, m, target, ec, cmd)
			if err != nil {
				return zero, err
			}
			return MoveProviderResult{
				MappingID:              m.ID,
				EmailDomain:            m.EmailDomain,
				FromIdentityProviderID: from,
				ToIdentityProviderID:   target.ID,
				UsersReset:             usersReset,
			}, nil
		},
	}
}

// MoveMappingTx re-points m to target inside the open transaction and applies
// the direction-aware side effects (see [MoveMappingToProvider]). It is the
// single implementation of "a domain changes provider" — the identity-provider
// orchestration ops call it too (existing domain claimed by a new IdP; domain
// removed from an IdP falls back to internal). Returns the number of
// principals converted back to internal auth.
//
// auditCmd is the original command driving the surrounding operation; it is
// threaded through as the audit subject of the scoped commits.
func MoveMappingTx(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps MoveDeps,
	m *emaildomainmapping.EmailDomainMapping,
	target *identityprovider.IdentityProvider,
	ec usecase.ExecutionContext,
	auditCmd any,
) (int, error) {
	from := m.IdentityProviderID
	m.IdentityProviderID = target.ID

	event := EmailDomainMappingProviderChanged{
		Metadata:               usecase.NewEventMetadata(ec, EmailDomainMappingProviderChangedType, Source, subjectFor(m.ID)),
		MappingID:              m.ID,
		EmailDomain:            m.EmailDomain,
		FromIdentityProviderID: from,
		ToIdentityProviderID:   target.ID,
	}
	if r := usecasepgx.CommitScoped(ctx, s, m, deps.Mappings, event, auditCmd); !usecase.IsSuccess(r) {
		_, e := usecase.Into(r)
		return 0, e
	}

	// Moving toward an OIDC provider needs no principal changes; the OIDC
	// callback matches existing users by email.
	if target.Type != identityprovider.TypeInternal {
		return 0, nil
	}

	// Moving to internal auth: convert the domain's OIDC-provisioned users
	// back. Without this they are hard-locked out — they have no password
	// hash, and the password-reset flow refuses principals whose idp_type is
	// OIDC. Internal (hybrid) users on the domain are untouched. The principal
	// writes are a persistence detail of the move (no per-user event); the
	// rollup lands in the ProviderChanged event's usersReset via the caller.
	users, err := deps.Principals.FindUsersByEmailDomain(ctx, m.EmailDomain)
	if err != nil {
		return 0, usecase.Internal("REPO", "principals by email domain failed", err)
	}
	reset := 0
	for i := range users {
		p := &users[i]
		if p.UserIdentity == nil || p.UserIdentity.Provider == nil || *p.UserIdentity.Provider != "OIDC" {
			continue
		}
		internal := "INTERNAL"
		p.UserIdentity.Provider = &internal
		p.UserIdentity.ExternalID = nil
		p.ExternalIdentity = nil
		// Drop IDP_SYNC-sourced roles; keep admin-assigned and system ones.
		kept := p.Roles[:0]
		for _, ra := range p.Roles {
			if ra.AssignmentSource != nil && *ra.AssignmentSource == principalops.IdpSyncSource {
				continue
			}
			kept = append(kept, ra)
		}
		p.Roles = kept
		if err := s.WithTx(ctx, func(tx pgx.Tx) error {
			return principal.RolesPersister{Repository: deps.Principals}.Persist(
				ctx, p, usecasepgx.WrapTxForBootstrap(tx))
		}); err != nil {
			return 0, usecase.Internal("PERSIST", "principal reset failed", err)
		}
		reset++
	}
	return reset, nil
}
