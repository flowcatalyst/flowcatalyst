package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteIdentityProvider removes an IdP and emits IdentityProviderDeleted.
func DeleteIdentityProvider(
	ctx context.Context,
	repo *identityprovider.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[IdentityProviderDeleted], error) {
	var zero commit.Committed[IdentityProviderDeleted]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}

	ip, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ip == nil {
		return zero, httperror.NotFound("IdentityProvider", cmd.ID)
	}
	event := IdentityProviderDeleted{
		Metadata:           usecase.NewEventMetadata(ec, IdentityProviderDeletedType, Source, subjectFor(ip.ID)),
		IdentityProviderID: ip.ID,
		Code:               ip.Code,
	}
	return commit.Delete(ctx, uow, ip, repo, event, cmd)
}
