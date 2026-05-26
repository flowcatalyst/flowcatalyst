package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
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

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *emaildomainmapping.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *emaildomainmapping.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.IdentityProviderID != nil && strings.TrimSpace(*cmd.IdentityProviderID) == "" {
		return usecase.Validation("INVALID_IDP", "identityProviderId cannot be empty when supplied")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[EmailDomainMappingUpdated] {
	e, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[EmailDomainMappingUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if e == nil {
		return usecase.Failure[EmailDomainMappingUpdated](httperror.NotFound("EmailDomainMapping", cmd.ID))
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
	return usecasepgx.Commit[emaildomainmapping.EmailDomainMapping, EmailDomainMappingUpdated, UpdateCommand](
		ctx, uc.uow, e, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, EmailDomainMappingUpdated] = (*UpdateUseCase)(nil)
