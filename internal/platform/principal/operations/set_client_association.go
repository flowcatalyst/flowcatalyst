package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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

// SetClientAssociation changes a principal's scope + client association. The
// handler gates this with require_anchor — scope/client changes are sensitive.
// Returns the principal-update commit; the caller re-reads the principal for the
// response.
func SetClientAssociation(
	ctx context.Context,
	repo *principal.Repository,
	clients *client.Repository,
	grants *principal.ClientAccessGrantRepo,
	uow *usecasepgx.UnitOfWork,
	cmd SetClientAssociationCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[UserUpdated], error) {
	var zero commit.Committed[UserUpdated]
	if strings.TrimSpace(cmd.UserID) == "" {
		return zero, usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	clientID := strings.TrimSpace(cmd.ClientID)
	if clientID == "" {
		return zero, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required (use \"*\" for anchor)")
	}

	p, err := repo.FindByID(ctx, cmd.UserID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_user failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("User", cmd.UserID)
	}
	if p.Type != principal.TypeUser {
		return zero, usecase.BusinessRule("NOT_A_USER", "Client association only applies to USER principals")
	}

	// Grants to add after the principal change (TO_PARTNER only).
	var grantClientIDs []string

	switch {
	case clientID == AnchorClientWildcard:
		p.Scope = principal.ScopeAnchor
		p.ClientID = nil
	case cmd.Mode == ModeChangeClient:
		if err := requireClientExists(ctx, clients, clientID); err != nil {
			return zero, err
		}
		p.Scope = principal.ScopeClient
		p.ClientID = &clientID
	case cmd.Mode == ModeToPartner:
		if err := requireClientExists(ctx, clients, clientID); err != nil {
			return zero, err
		}
		// Promoting from CLIENT: keep the old home client as access too.
		if p.Scope == principal.ScopeClient && p.ClientID != nil && *p.ClientID != "" && *p.ClientID != clientID {
			grantClientIDs = append(grantClientIDs, *p.ClientID)
		}
		grantClientIDs = append(grantClientIDs, clientID)
		p.Scope = principal.ScopePartner
		p.ClientID = nil
	default:
		return zero, usecase.Validation("MODE_REQUIRED",
			"mode must be CHANGE_CLIENT or TO_PARTNER for a specific clientId (use \"*\" for anchor)")
	}

	event := UserUpdated{
		Metadata: usecase.NewEventMetadata(ec, UserUpdatedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		Name:     p.Name,
	}
	committed, err := commit.Save(ctx, uow, p, repo, event, cmd)
	if err != nil {
		return zero, err
	}

	// Add the partner grants (idempotent — skip ones that already exist). Grants
	// live in a separate junction table, so they're saved separately from the
	// principal row.
	for _, cid := range grantClientIDs {
		existing, err := grants.FindByPrincipalAndClient(ctx, p.ID, cid)
		if err != nil {
			return zero, usecase.Internal("REPO", "find_existing_grant failed", err)
		}
		if existing != nil {
			continue
		}
		grant := principal.NewClientAccessGrant(p.ID, cid, ec.PrincipalID)
		gev := ClientAccessGranted{
			Metadata: usecase.NewEventMetadata(ec, ClientAccessGrantedType, Source, subjectFor(p.ID)),
			UserID:   p.ID,
			ClientID: cid,
		}
		if _, err := commit.Save(ctx, uow, grant, grants, gev, cmd); err != nil {
			return zero, err
		}
	}
	return committed, nil
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
