package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/commit"
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

func CreateAuthConfig(
	ctx context.Context,
	repo *auth.ClientAuthConfigRepo,
	uow *usecasepgx.UnitOfWork,
	cmd CreateAuthConfigCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AuthConfigCreated], error) {
	var zero commit.Committed[AuthConfigCreated]
	d := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))
	if d == "" || !strings.Contains(d, ".") {
		return zero, usecase.Validation("INVALID_EMAIL_DOMAIN", "emailDomain must be a valid DNS name")
	}
	switch cmd.ConfigType {
	case "ANCHOR", "PARTNER", "CLIENT":
	default:
		return zero, usecase.Validation("INVALID_CONFIG_TYPE", "configType must be ANCHOR, PARTNER, or CLIENT")
	}
	if auth.ParseAuthProvider(cmd.AuthProvider) == auth.ProviderOIDC {
		if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
			return zero, usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC provider requires oidcIssuerUrl")
		}
		if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
			return zero, usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC provider requires oidcClientId")
		}
	}

	existing, err := repo.FindByEmailDomain(ctx, d)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_email_domain failed", err)
	}
	if existing != nil {
		return zero, usecase.Conflict("DOMAIN_ALREADY_CONFIGURED", "Auth config for '"+d+"' already exists")
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
	return commit.Save(ctx, uow, c, repo, event, cmd)
}

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

func UpdateAuthConfig(
	ctx context.Context,
	repo *auth.ClientAuthConfigRepo,
	uow *usecasepgx.UnitOfWork,
	cmd UpdateAuthConfigCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AuthConfigUpdated], error) {
	var zero commit.Committed[AuthConfigUpdated]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("AuthConfig", cmd.ID)
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
	return commit.Save(ctx, uow, c, repo, event, cmd)
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAuthConfigCommand struct {
	ID string `json:"id"`
}

func DeleteAuthConfig(
	ctx context.Context,
	repo *auth.ClientAuthConfigRepo,
	uow *usecasepgx.UnitOfWork,
	cmd DeleteAuthConfigCommand,
	ec usecase.ExecutionContext,
) (commit.Committed[AuthConfigDeleted], error) {
	var zero commit.Committed[AuthConfigDeleted]
	if strings.TrimSpace(cmd.ID) == "" {
		return zero, usecase.Validation("ID_REQUIRED", "id is required")
	}
	c, err := repo.FindByID(ctx, cmd.ID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return zero, httperror.NotFound("AuthConfig", cmd.ID)
	}
	event := AuthConfigDeleted{
		Metadata:     usecase.NewEventMetadata(ec, AuthConfigDeletedType, Source, configSubject(c.ID)),
		AuthConfigID: c.ID,
		EmailDomain:  c.EmailDomain,
	}
	return commit.Delete(ctx, uow, c, repo, event, cmd)
}
