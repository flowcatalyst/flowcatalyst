package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

type RevokeClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

// RevokeClientAccess removes a PARTNER user's client-access grant and emits
// [ClientAccessRevoked].
//
// Authorize is intentionally Public: the only caller is the admin endpoint,
// gated by auth.RequireAnchor at the controller. The use case enforces the
// domain invariants (USER type, grant exists).
func RevokeClientAccess(repo *principal.Repository, grants *principal.ClientAccessGrantRepo) usecaseop.Operation[RevokeClientAccessCommand, ClientAccessRevoked] {
	return usecaseop.Operation[RevokeClientAccessCommand, ClientAccessRevoked]{
		Name: "RevokeClientAccess",
		Validate: func(_ context.Context, cmd RevokeClientAccessCommand) error {
			if strings.TrimSpace(cmd.UserID) == "" {
				return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
			}
			if strings.TrimSpace(cmd.ClientID) == "" {
				return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[RevokeClientAccessCommand],
		Execute: func(ctx context.Context, cmd RevokeClientAccessCommand, ec usecase.ExecutionContext) (usecaseop.Plan[ClientAccessRevoked], error) {
			p, err := repo.FindByID(ctx, cmd.UserID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_user failed", err)
			}
			if p == nil {
				return nil, httperror.NotFound("User", cmd.UserID)
			}
			if p.Type != principal.TypeUser {
				return nil, usecase.BusinessRule("NOT_A_USER",
					"Client access can only be revoked from USER type principals")
			}

			grant, err := grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_grant failed", err)
			}
			if grant == nil {
				return nil, httperror.NotFound("Grant", cmd.UserID+":"+cmd.ClientID)
			}

			event := ClientAccessRevoked{
				Metadata: usecase.NewEventMetadata(ec, ClientAccessRevokedType, Source, subjectFor(p.ID)),
				UserID:   p.ID,
				ClientID: cmd.ClientID,
			}
			return usecaseop.Delete(grant, grants, event), nil
		},
	}
}
