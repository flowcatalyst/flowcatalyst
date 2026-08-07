package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// DeleteCommand is the input DTO.
type DeleteCommand struct {
	ID string `json:"id"`
}

// DeleteIdentityProvider removes an IdP and emits [IdentityProviderDeleted].
// Deletion is blocked while email-domain mappings still route to the provider
// — a dangling mapping silently flips its domain's users to the internal
// password prompt. Move or delete the mappings first. The seeded internal
// provider is not deletable at all (it is the fallback target for domain
// releases and the anchor of password auth).
func DeleteIdentityProvider(deps Deps) usecaseop.Operation[DeleteCommand, IdentityProviderDeleted] {
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
			repo := deps.Repo
			ip, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if ip == nil {
				return nil, httperror.NotFound("IdentityProvider", cmd.ID)
			}
			if ip.Code == internalIdPCode {
				return nil, usecase.BusinessRule("INTERNAL_IDP_PROTECTED",
					"The internal identity provider cannot be deleted")
			}
			mappings, err := deps.MoveDeps.Mappings.FindByIdentityProvider(ctx, ip.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "mappings by identity provider failed", err)
			}
			if len(mappings) > 0 {
				domains := make([]string, 0, len(mappings))
				for _, m := range mappings {
					domains = append(domains, m.EmailDomain)
				}
				return nil, usecase.Conflict("DOMAINS_STILL_MAPPED",
					"Identity provider still routes email domains ("+strings.Join(domains, ", ")+"); move or delete those mappings first")
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
