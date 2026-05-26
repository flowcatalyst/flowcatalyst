package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// RevokeClientAccessCommand removes a user's access to a client.
type RevokeClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

// RevokeClientAccessUseCase implements UseCase. Deletes the matching
// ClientAccessGrant via UoW.CommitDelete.
type RevokeClientAccessUseCase struct {
	principals *principal.Repository
	grants     *principal.ClientAccessGrantRepo
	uow        *usecasepgx.UnitOfWork
}

// NewRevokeClientAccessUseCase wires the use case.
func NewRevokeClientAccessUseCase(principals *principal.Repository, grants *principal.ClientAccessGrantRepo, uow *usecasepgx.UnitOfWork) *RevokeClientAccessUseCase {
	return &RevokeClientAccessUseCase{principals: principals, grants: grants, uow: uow}
}

func (uc *RevokeClientAccessUseCase) Validate(_ context.Context, cmd RevokeClientAccessCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}
	return nil
}

func (uc *RevokeClientAccessUseCase) Authorize(_ context.Context, _ RevokeClientAccessCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *RevokeClientAccessUseCase) Execute(ctx context.Context, cmd RevokeClientAccessCommand, ec usecase.ExecutionContext) usecase.Result[ClientAccessRevoked] {
	p, err := uc.principals.FindByID(ctx, cmd.UserID)
	if err != nil {
		return usecase.Failure[ClientAccessRevoked](usecase.Internal("REPO", "find_user failed", err))
	}
	if p == nil {
		return usecase.Failure[ClientAccessRevoked](httperror.NotFound("User", cmd.UserID))
	}
	if p.Type != principal.TypeUser {
		return usecase.Failure[ClientAccessRevoked](usecase.BusinessRule("NOT_A_USER",
			"Client access can only be revoked from USER type principals"))
	}

	grant, err := uc.grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ClientAccessRevoked](usecase.Internal("REPO", "find_grant failed", err))
	}
	if grant == nil {
		return usecase.Failure[ClientAccessRevoked](httperror.NotFound("Grant",
			cmd.UserID+":"+cmd.ClientID))
	}

	event := ClientAccessRevoked{
		Metadata: usecase.NewEventMetadata(ec, ClientAccessRevokedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		ClientID: cmd.ClientID,
	}
	return usecasepgx.CommitDelete[principal.ClientAccessGrant, ClientAccessRevoked, RevokeClientAccessCommand](
		ctx, uc.uow, grant, uc.grants, event, cmd,
	)
}

var _ usecase.UseCase[RevokeClientAccessCommand, ClientAccessRevoked] = (*RevokeClientAccessUseCase)(nil)
