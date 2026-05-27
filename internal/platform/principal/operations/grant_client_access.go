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

type GrantClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

func GrantClientAccess(
	ctx context.Context,
	principals *principal.Repository,
	clients *client.Repository,
	grants *principal.ClientAccessGrantRepo,
	uow *usecasepgx.UnitOfWork,
	cmd GrantClientAccessCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientAccessGranted], error) {
	var zero commit.Committed[ClientAccessGranted]
	if strings.TrimSpace(cmd.UserID) == "" {
		return zero, usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return zero, usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}

	p, err := principals.FindByID(ctx, cmd.UserID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_user failed", err)
	}
	if p == nil {
		return zero, httperror.NotFound("User", cmd.UserID)
	}
	if p.Type != principal.TypeUser {
		return zero, usecase.BusinessRule("NOT_A_USER",
			"Client access can only be granted to USER type principals")
	}
	if p.Scope != principal.ScopePartner {
		return zero, usecase.BusinessRule("NOT_PARTNER_SCOPE",
			"Client access grants are only for PARTNER scope users")
	}

	c, err := clients.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_client failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("Client", cmd.ClientID)
	}

	existing, err := grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_existing_grant failed", err)
	}
	if existing != nil {
		return zero, usecase.BusinessRule("GRANT_EXISTS", "User already has access to this client")
	}

	grant := principal.NewClientAccessGrant(cmd.UserID, cmd.ClientID, ec.PrincipalID)

	event := ClientAccessGranted{
		Metadata: usecase.NewEventMetadata(ec, ClientAccessGrantedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		ClientID: cmd.ClientID,
	}
	return commit.Save(ctx, uow, grant, grants, event, cmd)
}
