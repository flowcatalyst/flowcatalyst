package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ClientAssociationMode disambiguates what setting a clientId on an existing,
// non-anchor principal means — the ambiguity that made the generic update punt
// on scope/client:
//
//   - CHANGE_CLIENT replaces the home client and keeps the principal CLIENT-scoped.
//   - TO_PARTNER promotes the principal to PARTNER, preserving the old home
//     client and adding the new one as access grants.
//
// A clientId of "*" makes the principal an ANCHOR (all-client access),
// regardless of mode.
type ClientAssociationMode string

const (
	ModeChangeClient ClientAssociationMode = "CHANGE_CLIENT"
	ModeToPartner    ClientAssociationMode = "TO_PARTNER"

	// AnchorClientWildcard is the clientId sentinel meaning "all clients".
	AnchorClientWildcard = "*"
)

type SetClientAssociationCommand struct {
	UserID   string
	ClientID string
	Mode     ClientAssociationMode
}

// SetClientAssociation changes a principal's scope + client association and
// emits [UserUpdated]. The TO_PARTNER promotion additionally writes the old home
// client (and the new one) as PARTNER access grants, atomically with the scope
// change, via ClientAssociationPersister.
//
// Authorize is intentionally Public: the only caller is the admin endpoint,
// gated by auth.RequireAnchor at the controller — scope/client changes are
// anchor-only, so there is no per-resource dimension for the use case to add.
func SetClientAssociation(repo *principal.Repository, clients *client.Repository, grants *principal.ClientAccessGrantRepo) usecaseop.Operation[SetClientAssociationCommand, UserUpdated] {
	return usecaseop.Operation[SetClientAssociationCommand, UserUpdated]{
		Name: "SetClientAssociation",
		Validate: func(_ context.Context, cmd SetClientAssociationCommand) error {
			if strings.TrimSpace(cmd.UserID) == "" {
				return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required (use \"*\" for anchor)")
			}
			return nil
		},
		Authorize: usecaseop.Public[SetClientAssociationCommand],
		Execute: func(ctx context.Context, cmd SetClientAssociationCommand, ec usecase.ExecutionContext) (usecaseop.Plan[UserUpdated], error) {
			clientID := strings.TrimSpace(cmd.ClientID)

			p, err := repo.FindByID(ctx, cmd.UserID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_user failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.UserID)
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER", "Client association only applies to USER principals")
			}

			// Grants to add after the principal change (TO_PARTNER only).
			var grantClientIDs []string

			switch {
			case clientID == AnchorClientWildcard:
				p.Scope = principal.ScopeAnchor
				p.ClientID = nil
			case cmd.Mode == ModeChangeClient:
				if err := requireClientExists(ctx, clients, clientID); err != nil {
					return nil, err
				}
				p.Scope = principal.ScopeClient
				p.ClientID = &clientID
			case cmd.Mode == ModeToPartner:
				if err := requireClientExists(ctx, clients, clientID); err != nil {
					return nil, err
				}
				// Promoting from CLIENT: keep the old home client as access too.
				if p.Scope == principal.ScopeClient && p.ClientID != nil && *p.ClientID != "" && *p.ClientID != clientID {
					grantClientIDs = append(grantClientIDs, *p.ClientID)
				}
				grantClientIDs = append(grantClientIDs, clientID)
				p.Scope = principal.ScopePartner
				p.ClientID = nil
			default:
				return nil, usecase.Validation("MODE_REQUIRED",
					"mode must be CHANGE_CLIENT or TO_PARTNER for a specific clientId (use \"*\" for anchor)")
			}

			event := UserUpdated{
				Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				Name:     p.Name,
			}
			// ClientAssociationPersister writes the principal row AND the partner
			// grants (idempotently) in the same tx as the event. With no grants
			// (anchor / change-client) it degrades to a plain principal upsert.
			persister := principal.ClientAssociationPersister{
				Repository:     repo,
				Grants:         grants,
				GrantClientIDs: grantClientIDs,
				GrantedBy:      ec.PrincipalID,
			}
			return usecaseop.Save(p, persister, event), nil
		},
	}
}

func requireClientExists(ctx context.Context, clients *client.Repository, clientID string) error {
	c, err := clients.FindByID(ctx, clientID)
	if err != nil {
		return usecase.Internal("REPO", "find_client failed", err)
	}
	if c == nil {
		return httperror.NotFound("Client", clientID)
	}
	return nil
}
