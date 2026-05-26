package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
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

// UpdateUseCase implements UseCase.
type UpdateUseCase struct {
	repo *identityprovider.Repository
	uow  *usecasepgx.UnitOfWork
}

// NewUpdateUseCase wires the use case.
func NewUpdateUseCase(repo *identityprovider.Repository, uow *usecasepgx.UnitOfWork) *UpdateUseCase {
	return &UpdateUseCase{repo: repo, uow: uow}
}

func (uc *UpdateUseCase) Validate(_ context.Context, cmd UpdateCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
		return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
	}
	return nil
}

func (uc *UpdateUseCase) Authorize(_ context.Context, _ UpdateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateUseCase) Execute(ctx context.Context, cmd UpdateCommand, ec usecase.ExecutionContext) usecase.Result[IdentityProviderUpdated] {
	ip, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[IdentityProviderUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if ip == nil {
		return usecase.Failure[IdentityProviderUpdated](httperror.NotFound("IdentityProvider", cmd.ID))
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
	return usecasepgx.Commit[identityprovider.IdentityProvider, IdentityProviderUpdated, UpdateCommand](
		ctx, uc.uow, ip, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateCommand, IdentityProviderUpdated] = (*UpdateUseCase)(nil)
