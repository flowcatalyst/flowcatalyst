package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// UpdateCommand mirrors CreateCommand but with the mapping ID + optional
// fields. A nil pointer means "do not change"; an empty slice means "clear".
type UpdateCommand struct {
	ID                   string   `json:"id"`
	IdentityProviderID   *string  `json:"identityProviderId,omitempty"`
	PrimaryClientID      *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs     []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID *string  `json:"requiredOidcTenantId,omitempty"`
	AllowedRoleIDs       []string `json:"allowedRoleIds,omitempty"`
	SyncRolesFromIDP     *bool    `json:"syncRolesFromIdp,omitempty"`
}

// UpdateMapping mutates an existing mapping and emits
// EmailDomainMappingUpdated.
func UpdateMapping(
	ctx context.Context,
	repo *emaildomainmapping.Repository,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[EmailDomainMappingUpdated], error) {
	var zero commit.Committed[EmailDomainMappingUpdated]

	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.IdentityProviderID != nil && strings.TrimSpace(*cmd.IdentityProviderID) == "" {
		return zero, usecase.Validation("INVALID_IDP", "identityProviderId cannot be empty when supplied")
	}

	e, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if e == nil {
		return zero, httperror.NotFound("EmailDomainMapping", cmd.ID)
	}

	if cmd.IdentityProviderID != nil {
		e.IdentityProviderID = *cmd.IdentityProviderID
	}
	e.PrimaryClientID = cmd.PrimaryClientID
	e.RequiredOIDCTenantID = cmd.RequiredOIDCTenantID
	if cmd.AdditionalClientIDs != nil {
		e.AdditionalClientIDs = cmd.AdditionalClientIDs
	}
	if cmd.GrantedClientIDs != nil {
		e.GrantedClientIDs = cmd.GrantedClientIDs
	}
	if cmd.AllowedRoleIDs != nil {
		e.AllowedRoleIDs = cmd.AllowedRoleIDs
	}
	if cmd.SyncRolesFromIDP != nil {
		e.SyncRolesFromIDP = *cmd.SyncRolesFromIDP
	}

	event := EmailDomainMappingUpdated{
		Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingUpdatedType, Source, subjectFor(e.ID)),
		MappingID:   e.ID,
		EmailDomain: e.EmailDomain,
	}
	return commit.Save(ctx, uow, e, repo, event, cmd)
}
