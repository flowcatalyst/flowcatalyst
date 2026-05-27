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

// UpdateIdentityProvider mutates an existing IdP and emits IdentityProviderUpdated.
func UpdateIdentityProvider(
	ctx context.Context,
	repo *identityprovider.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[IdentityProviderUpdated], error) {
	var zero commit.Committed[IdentityProviderUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return zero, usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}

	ip, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ip == nil {
		return zero, httperror.NotFound("IdentityProvider", cmd.ID)
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
	return commit.Save(ctx, uow, ip, repo, event, cmd)
}
