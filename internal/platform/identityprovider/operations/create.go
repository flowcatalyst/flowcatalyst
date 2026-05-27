package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
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

// CreateIdentityProvider creates a new IdP and emits IdentityProviderCreated.
func CreateIdentityProvider(
	ctx context.Context,
	repo *identityprovider.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd CreateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[IdentityProviderCreated], error) {
	var zero commit.Committed[IdentityProviderCreated]

	if strings.TrimSpace(cmd.Code) == "" {
		return zero, usecase.Validation("CODE_REQUIRED", "code is required")
	}
	if strings.TrimSpace(cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name is required")
	}
	t := identityprovider.ParseType(cmd.Type)
	if t == identityprovider.TypeOIDC {
		if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
			return zero, usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC IDPs require oidcIssuerUrl")
		}
		if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
			return zero, usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC IDPs require oidcClientId")
		}
	}

	existing, err := repo.FindByCode(ctx, cmd.Code)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("CODE_EXISTS", "Identity provider with code '"+cmd.Code+"' already exists")
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
	return commit.Save(ctx, uow, ip, repo, event, cmd)
}
