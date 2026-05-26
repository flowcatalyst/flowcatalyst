package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO.
type CreateCommand struct {
	EmailDomain          string   `json:"emailDomain"`
	IdentityProviderID   string   `json:"identityProviderId"`
	ScopeType            string   `json:"scopeType"`
	PrimaryClientID      *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs  []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs     []string `json:"grantedClientIds,omitempty"`
	RequiredOIDCTenantID *string  `json:"requiredOidcTenantId,omitempty"`
	AllowedRoleIDs       []string `json:"allowedRoleIds,omitempty"`
	SyncRolesFromIDP     bool     `json:"syncRolesFromIdp"`
}

// CreateUseCase implements UseCase.
type CreateUseCase struct {
	repo *emaildomainmapping.Repository
	uow  *usecasepgx.UnitOfWork
	// TODO(wave-3d): inject *idp.Repository to validate IdentityProviderID exists.
}

// NewCreateUseCase wires the use case.
func NewCreateUseCase(repo *emaildomainmapping.Repository, uow *usecasepgx.UnitOfWork) *CreateUseCase {
	return &CreateUseCase{repo: repo, uow: uow}
}

func (uc *CreateUseCase) Validate(_ context.Context, cmd CreateCommand) error {
	domain := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))
	if domain == "" {
		return usecase.Validation("EMAIL_DOMAIN_REQUIRED", "Email domain is required")
	}
	if !strings.Contains(domain, ".") || strings.ContainsAny(domain, " /@") {
		return usecase.Validation("INVALID_EMAIL_DOMAIN", "Email domain must be a valid DNS name (e.g. example.com)")
	}
	if strings.TrimSpace(cmd.IdentityProviderID) == "" {
		return usecase.Validation("IDP_REQUIRED", "identityProviderId is required")
	}
	switch cmd.ScopeType {
	case "ANCHOR", "PARTNER", "CLIENT":
	default:
		return usecase.Validation("INVALID_SCOPE_TYPE", "scopeType must be ANCHOR, PARTNER, or CLIENT")
	}
	if (cmd.ScopeType == "PARTNER" || cmd.ScopeType == "CLIENT") && cmd.PrimaryClientID == nil {
		return usecase.Validation("PRIMARY_CLIENT_REQUIRED",
			"primaryClientId is required for PARTNER and CLIENT scope")
	}
	return nil
}

func (uc *CreateUseCase) Authorize(_ context.Context, _ CreateCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateUseCase) Execute(ctx context.Context, cmd CreateCommand, ec usecase.ExecutionContext) usecase.Result[EmailDomainMappingCreated] {
	domain := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))

	existing, err := uc.repo.FindByEmailDomain(ctx, domain)
	if err != nil {
		return usecase.Failure[EmailDomainMappingCreated](usecase.Internal("REPO", "find_by_email_domain failed", err))
	}
	if existing != nil {
		return usecase.Failure[EmailDomainMappingCreated](usecase.Conflict(
			"DOMAIN_ALREADY_MAPPED",
			"Email domain '"+domain+"' is already mapped"))
	}

	// TODO(wave-3d): once identity_provider is ported, validate cmd.IdentityProviderID exists.

	e := emaildomainmapping.New(domain, cmd.IdentityProviderID, emaildomainmapping.ParseScopeType(cmd.ScopeType))
	e.PrimaryClientID = cmd.PrimaryClientID
	e.RequiredOIDCTenantID = cmd.RequiredOIDCTenantID
	e.SyncRolesFromIDP = cmd.SyncRolesFromIDP
	if cmd.AdditionalClientIDs != nil {
		e.AdditionalClientIDs = cmd.AdditionalClientIDs
	}
	if cmd.GrantedClientIDs != nil {
		e.GrantedClientIDs = cmd.GrantedClientIDs
	}
	if cmd.AllowedRoleIDs != nil {
		e.AllowedRoleIDs = cmd.AllowedRoleIDs
	}

	event := EmailDomainMappingCreated{
		Metadata:    usecase.NewEventMetadata(ec, EmailDomainMappingCreatedType, Source, subjectFor(e.ID)),
		MappingID:   e.ID,
		EmailDomain: e.EmailDomain,
	}
	return usecasepgx.Commit[emaildomainmapping.EmailDomainMapping, EmailDomainMappingCreated, CreateCommand](
		ctx, uc.uow, e, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateCommand, EmailDomainMappingCreated] = (*CreateUseCase)(nil)
