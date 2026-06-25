package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
}

// CreateIdentityProvider validates cmd, enforces code uniqueness, persists the
// IdP, and emits [IdentityProviderCreated]. The coarse anchor-only write
// permission is enforced at the controller.
func CreateIdentityProvider(repo *identityprovider.Repository) usecaseop.Operation[CreateCommand, IdentityProviderCreated] {
	return usecaseop.Operation[CreateCommand, IdentityProviderCreated]{
		Name: "CreateIdentityProvider",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.Code) == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if identityprovider.ParseType(cmd.Type) == identityprovider.TypeOIDC {
				if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
					return usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC IDPs require oidcIssuerUrl")
				}
				if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
					return usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC IDPs require oidcClientId")
				}
			}
			return nil
		},
		// The coarse "may write identity providers" permission (anchor-only) is
		// enforced at the controller; there is no per-resource authz dimension.
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) (usecaseop.Plan[IdentityProviderCreated], error) {
			existing, err := repo.FindByCode(ctx, cmd.Code)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CODE_EXISTS", "Identity provider with code '"+cmd.Code+"' already exists")
			}

			ip := identityprovider.New(cmd.Code, cmd.Name, identityprovider.ParseType(cmd.Type))
			ip.OIDCIssuerURL = cmd.OIDCIssuerURL
			ip.OIDCClientID = cmd.OIDCClientID
			ip.OIDCClientSecretRef = cmd.OIDCClientSecretRef
			ip.OIDCMultiTenant = cmd.OIDCMultiTenant
			ip.OIDCIssuerPattern = cmd.OIDCIssuerPattern
			if cmd.AllowedEmailDomains != nil {
				ip.AllowedEmailDomains = cmd.AllowedEmailDomains
			}

			event := IdentityProviderCreated{
				Metadata:           usecase.NewEventMetadata(ec, IdentityProviderCreatedType, Source, subjectFor(ip.ID)),
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
			}
			return usecaseop.Save(ip, repo, event), nil
		},
	}
}
