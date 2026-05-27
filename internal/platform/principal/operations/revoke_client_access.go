package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

type RevokeClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

func RevokeClientAccess(
	ctx context.Context,
	principals *principal.Repository,
	grants *principal.ClientAccessGrantRepo,
	uow *usecasepgx.UnitOfWork,
	cmd RevokeClientAccessCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[ClientAccessRevoked], error) {
	var zero commit.Committed[ClientAccessRevoked]
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
			"Client access can only be revoked from USER type principals")
	}

	grant, err := grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_grant failed", err)
	}
	if grant == nil {
		return zero, httperror.NotFound("Grant", cmd.UserID+":"+cmd.ClientID)
	}

	event := ClientAccessRevoked{
		Metadata: usecase.NewEventMetadata(ec, ClientAccessRevokedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		ClientID: cmd.ClientID,
	}
	return commit.Delete(ctx, uow, grant, grants, event, cmd)
}
