package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
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

// CreateAuthConfig validates the command, enforces per-domain uniqueness,
// persists the client auth config, and emits [AuthConfigCreated]. Auth configs
// are platform-level config with no per-client resource dimension (Authorize:
// Public); the controller gates writes with auth.RequireAnchor.
func CreateAuthConfig(repo *auth.ClientAuthConfigRepo) usecaseop.Operation[CreateAuthConfigCommand, AuthConfigCreated] {
	return usecaseop.Operation[CreateAuthConfigCommand, AuthConfigCreated]{
		Name: "CreateAuthConfig",
		Validate: func(_ context.Context, cmd CreateAuthConfigCommand) error {
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
		},
		Authorize: usecaseop.Public[CreateAuthConfigCommand],
		Execute: func(ctx context.Context, cmd CreateAuthConfigCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AuthConfigCreated], error) {
			d := strings.ToLower(strings.TrimSpace(cmd.EmailDomain))
			existing, err := repo.FindByEmailDomain(ctx, d)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_email_domain failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("DOMAIN_ALREADY_CONFIGURED", "Auth config for '"+d+"' already exists")
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
			return usecaseop.Save(c, repo, event), nil
		},
	}
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

// UpdateAuthConfig mutates the supplied fields and emits [AuthConfigUpdated].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func UpdateAuthConfig(repo *auth.ClientAuthConfigRepo) usecaseop.Operation[UpdateAuthConfigCommand, AuthConfigUpdated] {
	return usecaseop.Operation[UpdateAuthConfigCommand, AuthConfigUpdated]{
		Name: "UpdateAuthConfig",
		Validate: func(_ context.Context, cmd UpdateAuthConfigCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateAuthConfigCommand],
		Execute: func(ctx context.Context, cmd UpdateAuthConfigCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AuthConfigUpdated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("AuthConfig", cmd.ID)
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
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteAuthConfigCommand struct {
	ID string `json:"id"`
}

// DeleteAuthConfig removes the config and emits [AuthConfigDeleted].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func DeleteAuthConfig(repo *auth.ClientAuthConfigRepo) usecaseop.Operation[DeleteAuthConfigCommand, AuthConfigDeleted] {
	return usecaseop.Operation[DeleteAuthConfigCommand, AuthConfigDeleted]{
		Name: "DeleteAuthConfig",
		Validate: func(_ context.Context, cmd DeleteAuthConfigCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteAuthConfigCommand],
		Execute: func(ctx context.Context, cmd DeleteAuthConfigCommand, ec usecase.ExecutionContext) (usecaseop.Plan[AuthConfigDeleted], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("AuthConfig", cmd.ID)
			}
			event := AuthConfigDeleted{
				Metadata:     usecase.NewEventMetadata(ec, AuthConfigDeletedType, Source, configSubject(c.ID)),
				AuthConfigID: c.ID,
				EmailDomain:  c.EmailDomain,
			}
			return usecaseop.Delete(c, repo, event), nil
		},
	}
}
