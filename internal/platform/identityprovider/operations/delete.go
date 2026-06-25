package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteIdentityProvider removes an IdP and emits [IdentityProviderDeleted].
func DeleteIdentityProvider(repo *identityprovider.Repository) usecaseop.Operation[DeleteCommand, IdentityProviderDeleted] {
	return usecaseop.Operation[DeleteCommand, IdentityProviderDeleted]{
		Name: "DeleteIdentityProvider",
		Validate: func(_ context.Context, cmd DeleteCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		// The coarse "may write identity providers" permission (anchor-only) is
		// enforced at the controller; there is no per-resource authz dimension.
		Authorize: usecaseop.Public[DeleteCommand],
		Execute: func(ctx context.Context, cmd DeleteCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityProviderDeleted], error) {
			ip, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if ip == nil {
				return nil, httperror.NotFound("IdentityProvider", cmd.ID)
			}
			event := IdentityProviderDeleted{
				Metadata:           usecase.NewEventMetadata(ec, IdentityProviderDeletedType, Source, subjectFor(ip.ID)),
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
			}
			return usecaseop.Delete(ip, repo, event), nil
		},
	}
}
