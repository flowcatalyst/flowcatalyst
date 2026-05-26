package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateAuthConfigCommand struct {
	EmailDomain         string   `json:"emailDomain"`
	ConfigType          string   `json:"configType"`
	PrimaryClientID     *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs    []string `json:"grantedClientIds,omitempty"`
	AuthProvider        string   `json:"authProvider"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
}

type CreateAuthConfigUseCase struct {
	repo *auth.ClientAuthConfigRepo
	uow  *usecasepgx.UnitOfWork
}

func NewCreateAuthConfigUseCase(repo *auth.ClientAuthConfigRepo, uow *usecasepgx.UnitOfWork) *CreateAuthConfigUseCase {
	return &CreateAuthConfigUseCase{repo: repo, uow: uow}
}

func (uc *CreateAuthConfigUseCase) Validate(_ context.Context, cmd CreateAuthConfigCommand) error {
	d := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))
	if d == "" || !strings.Contains(d, ".") {
		return usecase.Validation("INVALID_EMAIL_DOMAIN", "emailDomain must be a valid DNS name")
	}
	switch cmd.ConfigType {
	case "ANCHOR", "PARTNER", "CLIENT":
	default:
		return usecase.Validation("INVALID_CONFIG_TYPE", "configType must be ANCHOR, PARTNER, or CLIENT")
	}
	if auth.ParseAuthProvider(cmd.AuthProvider) == auth.ProviderOIDC {
		if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
			return usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC provider requires oidcIssuerUrl")
		}
		if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
			return usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC provider requires oidcClientId")
		}
	}
	return nil
}

func (uc *CreateAuthConfigUseCase) Authorize(_ context.Context, _ CreateAuthConfigCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *CreateAuthConfigUseCase) Execute(ctx context.Context, cmd CreateAuthConfigCommand, ec usecase.ExecutionContext) usecase.Result[AuthConfigCreated] {
	d := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))
	existing, err := uc.repo.FindByEmailDomain(ctx, d)
	if err != nil {
		return usecase.Failure[AuthConfigCreated](usecase.Internal("REPO", "find_by_email_domain failed", err))
	}
	if existing != nil {
		return usecase.Failure[AuthConfigCreated](usecase.Conflict(
			"DOMAIN_ALREADY_CONFIGURED", "Auth config for '"+d+"' already exists"))
	}
	c := auth.NewClientAuthConfig(d, auth.ParseAuthConfigType(cmd.ConfigType))
	c.AuthProvider = auth.ParseAuthProvider(cmd.AuthProvider)
	c.PrimaryClientID = cmd.PrimaryClientID
	if cmd.AdditionalClientIDs != nil {
		c.AdditionalClientIDs = cmd.AdditionalClientIDs
	}
	if cmd.GrantedClientIDs != nil {
		c.GrantedClientIDs = cmd.GrantedClientIDs
	}
	c.OIDCIssuerURL = cmd.OIDCIssuerURL
	c.OIDCClientID = cmd.OIDCClientID
	c.OIDCMultiTenant = cmd.OIDCMultiTenant
	c.OIDCIssuerPattern = cmd.OIDCIssuerPattern
	c.OIDCClientSecretRef = cmd.OIDCClientSecretRef

	event := AuthConfigCreated{
		Metadata:     usecase.NewEventMetadata(ec, AuthConfigCreatedType, Source, configSubject(c.ID)),
		AuthConfigID: c.ID,
		EmailDomain:  c.EmailDomain,
	}
	return usecasepgx.Commit[auth.ClientAuthConfig, AuthConfigCreated, CreateAuthConfigCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[CreateAuthConfigCommand, AuthConfigCreated] = (*CreateAuthConfigUseCase)(nil)

// ── Update ────────────────────────────────────────────────────────────────

type UpdateAuthConfigCommand struct {
	ID                  string   `json:"id"`
	PrimaryClientID     *string  `json:"primaryClientId,omitempty"`
	AdditionalClientIDs []string `json:"additionalClientIds,omitempty"`
	GrantedClientIDs    []string `json:"grantedClientIds,omitempty"`
	AuthProvider        *string  `json:"authProvider,omitempty"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCMultiTenant     *bool    `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
}

type UpdateAuthConfigUseCase struct {
	repo *auth.ClientAuthConfigRepo
	uow  *usecasepgx.UnitOfWork
}

func NewUpdateAuthConfigUseCase(repo *auth.ClientAuthConfigRepo, uow *usecasepgx.UnitOfWork) *UpdateAuthConfigUseCase {
	return &UpdateAuthConfigUseCase{repo: repo, uow: uow}
}

func (uc *UpdateAuthConfigUseCase) Validate(_ context.Context, cmd UpdateAuthConfigCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *UpdateAuthConfigUseCase) Authorize(_ context.Context, _ UpdateAuthConfigCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *UpdateAuthConfigUseCase) Execute(ctx context.Context, cmd UpdateAuthConfigCommand, ec usecase.ExecutionContext) usecase.Result[AuthConfigUpdated] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[AuthConfigUpdated](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[AuthConfigUpdated](httperror.NotFound("AuthConfig", cmd.ID))
	}
	if cmd.PrimaryClientID != nil {
		c.PrimaryClientID = cmd.PrimaryClientID
	}
	if cmd.AdditionalClientIDs != nil {
		c.AdditionalClientIDs = cmd.AdditionalClientIDs
	}
	if cmd.GrantedClientIDs != nil {
		c.GrantedClientIDs = cmd.GrantedClientIDs
	}
	if cmd.AuthProvider != nil {
		c.AuthProvider = auth.ParseAuthProvider(*cmd.AuthProvider)
	}
	if cmd.OIDCIssuerURL != nil {
		c.OIDCIssuerURL = cmd.OIDCIssuerURL
	}
	if cmd.OIDCClientID != nil {
		c.OIDCClientID = cmd.OIDCClientID
	}
	if cmd.OIDCMultiTenant != nil {
		c.OIDCMultiTenant = *cmd.OIDCMultiTenant
	}
	if cmd.OIDCIssuerPattern != nil {
		c.OIDCIssuerPattern = cmd.OIDCIssuerPattern
	}
	if cmd.OIDCClientSecretRef != nil {
		c.OIDCClientSecretRef = cmd.OIDCClientSecretRef
	}

	event := AuthConfigUpdated{
		Metadata:     usecase.NewEventMetadata(ec, AuthConfigUpdatedType, Source, configSubject(c.ID)),
		AuthConfigID: c.ID,
		EmailDomain:  c.EmailDomain,
	}
	return usecasepgx.Commit[auth.ClientAuthConfig, AuthConfigUpdated, UpdateAuthConfigCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[UpdateAuthConfigCommand, AuthConfigUpdated] = (*UpdateAuthConfigUseCase)(nil)

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAuthConfigCommand struct {
	ID string `json:"id"`
}

type DeleteAuthConfigUseCase struct {
	repo *auth.ClientAuthConfigRepo
	uow  *usecasepgx.UnitOfWork
}

func NewDeleteAuthConfigUseCase(repo *auth.ClientAuthConfigRepo, uow *usecasepgx.UnitOfWork) *DeleteAuthConfigUseCase {
	return &DeleteAuthConfigUseCase{repo: repo, uow: uow}
}

func (uc *DeleteAuthConfigUseCase) Validate(_ context.Context, cmd DeleteAuthConfigCommand) error {
	if strings.TrimSpace(cmd.ID) == "" {
		return usecase.Validation("ID_REQUIRED", "id is required")
	}
	return nil
}

func (uc *DeleteAuthConfigUseCase) Authorize(_ context.Context, _ DeleteAuthConfigCommand, _ usecase.ExecutionContext) error {
	return nil
}

func (uc *DeleteAuthConfigUseCase) Execute(ctx context.Context, cmd DeleteAuthConfigCommand, ec usecase.ExecutionContext) usecase.Result[AuthConfigDeleted] {
	c, err := uc.repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return usecase.Failure[AuthConfigDeleted](usecase.Internal("REPO", "find_by_id failed", err))
	}
	if c == nil {
		return usecase.Failure[AuthConfigDeleted](httperror.NotFound("AuthConfig", cmd.ID))
	}
	event := AuthConfigDeleted{
		Metadata:     usecase.NewEventMetadata(ec, AuthConfigDeletedType, Source, configSubject(c.ID)),
		AuthConfigID: c.ID,
		EmailDomain:  c.EmailDomain,
	}
	return usecasepgx.CommitDelete[auth.ClientAuthConfig, AuthConfigDeleted, DeleteAuthConfigCommand](
		ctx, uc.uow, c, uc.repo, event, cmd,
	)
}

var _ usecase.UseCase[DeleteAuthConfigCommand, AuthConfigDeleted] = (*DeleteAuthConfigUseCase)(nil)
