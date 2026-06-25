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

type GrantClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

// GrantClientAccess records a PARTNER user's access to a specific client and
// emits [ClientAccessGranted].
//
// Authorize is intentionally Public: this op carries no per-resource dimension
// of its own — every caller is an admin controller that has already gated it
// (the dedicated endpoint with auth.RequireAnchor; createUser's partner-merge
// with auth.RequireUserAdmin against the resolved client). The use case enforces
// only the domain invariants (USER type, PARTNER scope, client exists, no
// duplicate grant).
func GrantClientAccess(repo *principal.Repository, clients *client.Repository, grants *principal.ClientAccessGrantRepo) usecaseop.Operation[GrantClientAccessCommand, ClientAccessGranted] {
	return usecaseop.Operation[GrantClientAccessCommand, ClientAccessGranted]{
		Name: "GrantClientAccess",
		Validate: func(_ context.Context, cmd GrantClientAccessCommand) error {
			if strings.TrimSpace(cmd.UserID) == "" {
				return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[GrantClientAccessCommand],
		Execute: func(ctx context.Context, cmd GrantClientAccessCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientAccessGranted], error) {
			p, err := repo.FindByID(ctx, cmd.UserID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_user failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.UserID)
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER",
					"Client access can only be granted to USER type principals")
			}
			if p.Scope != principal.ScopePartner {
				return nil, usecase.BusinessRule("NOT_PARTNER_SCOPE",
					"Client access grants are only for PARTNER scope users")
			}

			c, err := clients.FindByID(ctx, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_client failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("Client", cmd.ClientID)
			}

			existing, err := grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_existing_grant failed", err)
			}
			if existing != nil {
				return nil, usecase.BusinessRule("GRANT_EXISTS", "User already has access to this client")
			}

			grant := principal.NewClientAccessGrant(cmd.UserID, cmd.ClientID, ec.PrincipalID)

			event := ClientAccessGranted{
				Metadata: usecase.NewEventMetadata(ec, ClientAccessGrantedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				ClientID: cmd.ClientID,
			}
			return usecaseop.Save(grant, grants, event), nil
		},
	}
}
