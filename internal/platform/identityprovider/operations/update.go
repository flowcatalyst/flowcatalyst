package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// UpdateCommand is the input DTO.
type UpdateCommand struct {
	ID                  string   `json:"id"`
	Name                *string  `json:"name,omitempty"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     *bool    `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
}

// UpdateIdentityProvider mutates an existing IdP and emits
// [IdentityProviderUpdated].
func UpdateIdentityProvider(repo *identityprovider.Repository) usecaseop.Operation[UpdateCommand, IdentityProviderUpdated] {
	return usecaseop.Operation[UpdateCommand, IdentityProviderUpdated]{
		Name: "UpdateIdentityProvider",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return nil
		},
		// The coarse "may write identity providers" permission (anchor-only) is
		// enforced at the controller; there is no per-resource authz dimension.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityProviderUpdated], error) {
			ip, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if ip == nil {
				return nil, httperror.NotFound("IdentityProvider", cmd.ID)
			}
			if cmd.Name != nil {
				ip.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.OIDCIssuerURL != nil {
				ip.OIDCIssuerURL = cmd.OIDCIssuerURL
			}
			if cmd.OIDCClientID != nil {
				ip.OIDCClientID = cmd.OIDCClientID
			}
			if cmd.OIDCClientSecretRef != nil {
				ip.OIDCClientSecretRef = cmd.OIDCClientSecretRef
			}
			if cmd.OIDCMultiTenant != nil {
				ip.OIDCMultiTenant = *cmd.OIDCMultiTenant
			}
			if cmd.OIDCIssuerPattern != nil {
				ip.OIDCIssuerPattern = cmd.OIDCIssuerPattern
			}
			if cmd.AllowedEmailDomains != nil {
				ip.AllowedEmailDomains = cmd.AllowedEmailDomains
			}

			event := IdentityProviderUpdated{
				Metadata:           usecase.NewEventMetadata(ec, IdentityProviderUpdatedType, Source, subjectFor(ip.ID)),
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
			}
			return usecaseop.Save(ip, repo, event), nil
		},
	}
}
