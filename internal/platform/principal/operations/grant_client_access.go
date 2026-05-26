package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// GrantClientAccessCommand grants a PARTNER user access to a client.
type GrantClientAccessCommand struct {
	UserID   string `json:"userId"`
	ClientID string `json:"clientId"`
}

// GrantClientAccessUseCase implements UseCase. Persists a
// ClientAccessGrant aggregate (its own row in iam_client_access_grants).
type GrantClientAccessUseCase struct {
	principals *principal.Repository
	clients    *client.Repository
	grants     *principal.ClientAccessGrantRepo
	uow        *usecasepgx.UnitOfWork
}

// NewGrantClientAccessUseCase wires the use case.
func NewGrantClientAccessUseCase(principals *principal.Repository, clients *client.Repository, grants *principal.ClientAccessGrantRepo, uow *usecasepgx.UnitOfWork) *GrantClientAccessUseCase {
	return &GrantClientAccessUseCase{principals: principals, clients: clients, grants: grants, uow: uow}
}

func (uc *GrantClientAccessUseCase) Validate(_ context.Context, cmd GrantClientAccessCommand) error {
	if strings.TrimSpace(cmd.UserID) == "" {
		return usecase.Validation("USER_ID_REQUIRED", "User ID is required")
	}
	if strings.TrimSpace(cmd.ClientID) == "" {
		return usecase.Validation("CLIENT_ID_REQUIRED", "Client ID is required")
	}
	return nil
}

func (uc *GrantClientAccessUseCase) Authorize(_ context.Context, _ GrantClientAccessCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *GrantClientAccessUseCase) Execute(ctx context.Context, cmd GrantClientAccessCommand, ec usecase.ExecutionContext) usecase.Result[ClientAccessGranted] {
	p, err := uc.principals.FindByID(ctx, cmd.UserID)
	if err != nil {
		return usecase.Failure[ClientAccessGranted](usecase.Internal("REPO", "find_user failed", err))
	}
	if p == nil {
		return usecase.Failure[ClientAccessGranted](httperror.NotFound("User", cmd.UserID))
	}
	if p.Type != principal.TypeUser {
		return usecase.Failure[ClientAccessGranted](usecase.BusinessRule("NOT_A_USER",
			"Client access can only be granted to USER type principals"))
	}
	if p.Scope != principal.ScopePartner {
		return usecase.Failure[ClientAccessGranted](usecase.BusinessRule("NOT_PARTNER_SCOPE",
			"Client access grants are only for PARTNER scope users"))
	}

	c, err := uc.clients.FindByID(ctx, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ClientAccessGranted](usecase.Internal("REPO", "find_client failed", err))
	}
	if c == nil {
		return usecase.Failure[ClientAccessGranted](httperror.NotFound("Client", cmd.ClientID))
	}

	existing, err := uc.grants.FindByPrincipalAndClient(ctx, cmd.UserID, cmd.ClientID)
	if err != nil {
		return usecase.Failure[ClientAccessGranted](usecase.Internal("REPO", "find_existing_grant failed", err))
	}
	if existing != nil {
		return usecase.Failure[ClientAccessGranted](usecase.BusinessRule("GRANT_EXISTS",
			"User already has access to this client"))
	}

	grant := principal.NewClientAccessGrant(cmd.UserID, cmd.ClientID, ec.PrincipalID)

	event := ClientAccessGranted{
		Metadata: usecase.NewEventMetadata(ec, ClientAccessGrantedType, Source, subjectFor(p.ID)),
		UserID:   p.ID,
		ClientID: cmd.ClientID,
	}
	return usecasepgx.Commit[principal.ClientAccessGrant, ClientAccessGranted, GrantClientAccessCommand](
		ctx, uc.uow, grant, uc.grants, event, cmd,
	)
}

var _ usecase.UseCase[GrantClientAccessCommand, ClientAccessGranted] = (*GrantClientAccessUseCase)(nil)
