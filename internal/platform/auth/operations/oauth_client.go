package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// ── Create ────────────────────────────────────────────────────────────────

type CreateOAuthClientCommand struct {
	ClientID               string   `json:"clientId"`
	ClientName             string   `json:"clientName"`
	ClientType             string   `json:"clientType"`
	RedirectURIs           []string `json:"redirectUris,omitempty"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
	GrantTypes             []string `json:"grantTypes,omitempty"`
	Scopes                 []string `json:"scopes,omitempty"`
	AllowedOrigins         []string `json:"allowedOrigins,omitempty"`
	ApplicationIDs         []string `json:"applicationIds,omitempty"`
	PrincipalID            *string  `json:"principalId,omitempty"`
	PKCERequired           *bool    `json:"pkceRequired,omitempty"`
}

// CreateOAuthClient validates the command, persists the OAuth client, and
// emits [OAuthClientCreated]. OAuth clients are platform-level config: the
// "ClientID" is the OAuth2 client_id string, not a tenant, so there is no
// per-client resource dimension and the operation is intentionally open
// (Authorize: Public). The controller gates writes with auth.RequireAnchor.
func CreateOAuthClient(repo *auth.OAuthClientRepo) usecaseop.Operation[CreateOAuthClientCommand, OAuthClientCreated] {
	return usecaseop.Operation[CreateOAuthClientCommand, OAuthClientCreated]{
		Name: "CreateOAuthClient",
		Validate: func(_ context.Context, cmd CreateOAuthClientCommand) error {
			if strings.TrimSpace(cmd.ClientName) == "" {
				return usecase.Validation("CLIENT_NAME_REQUIRED", "clientName is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[CreateOAuthClientCommand],
		Execute: func(ctx context.Context, cmd CreateOAuthClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientCreated], error) {
			// The OAuth2 client_id is backend-generated — a branded TSID, exactly
			// like the service-account provision flows — when the caller omits it.
			// Callers that supply their own (SDK/legacy) keep working and still hit
			// the uniqueness check below.
			clientID := strings.TrimSpace(cmd.ClientID)
			if clientID == "" {
				clientID = tsid.Generate(tsid.OAuthClient)
			}
			existing, err := repo.FindByClientID(ctx, clientID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_client_id failed", err)
			}
			if existing != nil {
				return nil, usecase.Conflict("CLIENT_ID_EXISTS", "OAuth client_id '"+clientID+"' already exists")
			}
			t := auth.ParseOAuthClientType(cmd.ClientType)
			c := auth.NewOAuthClient(clientID, cmd.ClientName, t)
			c.RedirectURIs = cmd.RedirectURIs
			c.PostLogoutRedirectURIs = cmd.PostLogoutRedirectURIs
			c.GrantTypes = cmd.GrantTypes
			c.Scopes = cmd.Scopes
			c.AllowedOrigins = cmd.AllowedOrigins
			c.ApplicationIDs = cmd.ApplicationIDs
			c.PrincipalID = cmd.PrincipalID
			if cmd.PKCERequired != nil {
				c.PKCERequired = *cmd.PKCERequired
			}
			if t == auth.OAuthClientConfidential {
				plaintext, ref, err := generateSecret()
				if err != nil {
					return nil, usecase.Internal("SECRET", "generate client secret failed", err)
				}
				c.SetSecretRef(ref)
				stashSecret(c.ID, plaintext)
			}

			event := OAuthClientCreated{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientCreatedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
				ClientID:      c.ClientID,
				ClientName:    c.ClientName,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── Update ────────────────────────────────────────────────────────────────

type UpdateOAuthClientCommand struct {
	ID                     string   `json:"id"`
	ClientName             *string  `json:"clientName,omitempty"`
	RedirectURIs           []string `json:"redirectUris,omitempty"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectUris,omitempty"`
	GrantTypes             []string `json:"grantTypes,omitempty"`
	Scopes                 []string `json:"scopes,omitempty"`
	AllowedOrigins         []string `json:"allowedOrigins,omitempty"`
	ApplicationIDs         []string `json:"applicationIds,omitempty"`
	PKCERequired           *bool    `json:"pkceRequired,omitempty"`
}

// UpdateOAuthClient mutates the supplied fields and emits [OAuthClientUpdated].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func UpdateOAuthClient(repo *auth.OAuthClientRepo) usecaseop.Operation[UpdateOAuthClientCommand, OAuthClientUpdated] {
	return usecaseop.Operation[UpdateOAuthClientCommand, OAuthClientUpdated]{
		Name: "UpdateOAuthClient",
		Validate: func(_ context.Context, cmd UpdateOAuthClientCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.ClientName != nil && strings.TrimSpace(*cmd.ClientName) == "" {
				return usecase.Validation("CLIENT_NAME_REQUIRED", "clientName cannot be empty")
			}
			return nil
		},
		Authorize: usecaseop.Public[UpdateOAuthClientCommand],
		Execute: func(ctx context.Context, cmd UpdateOAuthClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientUpdated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("OAuthClient", cmd.ID)
			}
			if cmd.ClientName != nil {
				c.ClientName = strings.TrimSpace(*cmd.ClientName)
			}
			if cmd.RedirectURIs != nil {
				c.RedirectURIs = cmd.RedirectURIs
			}
			if cmd.PostLogoutRedirectURIs != nil {
				c.PostLogoutRedirectURIs = cmd.PostLogoutRedirectURIs
			}
			if cmd.GrantTypes != nil {
				c.GrantTypes = cmd.GrantTypes
			}
			if cmd.Scopes != nil {
				c.Scopes = cmd.Scopes
			}
			if cmd.AllowedOrigins != nil {
				c.AllowedOrigins = cmd.AllowedOrigins
			}
			if cmd.ApplicationIDs != nil {
				c.ApplicationIDs = cmd.ApplicationIDs
			}
			if cmd.PKCERequired != nil {
				c.PKCERequired = *cmd.PKCERequired
			}

			event := OAuthClientUpdated{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientUpdatedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
				ClientName:    c.ClientName,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── Activate ──────────────────────────────────────────────────────────────

type ActivateOAuthClientCommand struct {
	ID string `json:"id"`
}

// ActivateOAuthClient flips the client Active and emits [OAuthClientActivated].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func ActivateOAuthClient(repo *auth.OAuthClientRepo) usecaseop.Operation[ActivateOAuthClientCommand, OAuthClientActivated] {
	return usecaseop.Operation[ActivateOAuthClientCommand, OAuthClientActivated]{
		Name: "ActivateOAuthClient",
		Validate: func(_ context.Context, cmd ActivateOAuthClientCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ActivateOAuthClientCommand],
		Execute: func(ctx context.Context, cmd ActivateOAuthClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientActivated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("OAuthClient", cmd.ID)
			}
			c.Activate()
			event := OAuthClientActivated{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientActivatedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── Deactivate ────────────────────────────────────────────────────────────

type DeactivateOAuthClientCommand struct {
	ID string `json:"id"`
}

// DeactivateOAuthClient flips the client inactive and emits
// [OAuthClientDeactivated]. Platform-level config (Authorize: Public); the
// controller gates on anchor.
func DeactivateOAuthClient(repo *auth.OAuthClientRepo) usecaseop.Operation[DeactivateOAuthClientCommand, OAuthClientDeactivated] {
	return usecaseop.Operation[DeactivateOAuthClientCommand, OAuthClientDeactivated]{
		Name: "DeactivateOAuthClient",
		Validate: func(_ context.Context, cmd DeactivateOAuthClientCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeactivateOAuthClientCommand],
		Execute: func(ctx context.Context, cmd DeactivateOAuthClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientDeactivated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("OAuthClient", cmd.ID)
			}
			c.Deactivate()
			event := OAuthClientDeactivated{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientDeactivatedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── Delete ────────────────────────────────────────────────────────────────

type DeleteOAuthClientCommand struct {
	ID string `json:"id"`
}

// DeleteOAuthClient removes the client and emits [OAuthClientDeleted].
// Platform-level config (Authorize: Public); the controller gates on anchor.
func DeleteOAuthClient(repo *auth.OAuthClientRepo) usecaseop.Operation[DeleteOAuthClientCommand, OAuthClientDeleted] {
	return usecaseop.Operation[DeleteOAuthClientCommand, OAuthClientDeleted]{
		Name: "DeleteOAuthClient",
		Validate: func(_ context.Context, cmd DeleteOAuthClientCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[DeleteOAuthClientCommand],
		Execute: func(ctx context.Context, cmd DeleteOAuthClientCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientDeleted], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("OAuthClient", cmd.ID)
			}
			event := OAuthClientDeleted{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientDeletedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
				ClientID:      c.ClientID,
			}
			return usecaseop.Delete(c, repo, event), nil
		},
	}
}

// ── RotateSecret ──────────────────────────────────────────────────────────

type RotateOAuthClientSecretCommand struct {
	ID string `json:"id"`
}

// RotateOAuthClientSecret mints a fresh secret for a CONFIDENTIAL client,
// stashes the plaintext for one-shot retrieval, and emits
// [OAuthClientSecretRotated]. Platform-level config (Authorize: Public); the
// controller gates on anchor.
func RotateOAuthClientSecret(repo *auth.OAuthClientRepo) usecaseop.Operation[RotateOAuthClientSecretCommand, OAuthClientSecretRotated] {
	return usecaseop.Operation[RotateOAuthClientSecretCommand, OAuthClientSecretRotated]{
		Name: "RotateOAuthClientSecret",
		Validate: func(_ context.Context, cmd RotateOAuthClientSecretCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[RotateOAuthClientSecretCommand],
		Execute: func(ctx context.Context, cmd RotateOAuthClientSecretCommand, ec usecase.ExecutionContext) (usecaseop.Plan[OAuthClientSecretRotated], error) {
			c, err := repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return nil, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if c == nil {
				return nil, httperror.NotFound("OAuthClient", cmd.ID)
			}
			if c.ClientType != auth.OAuthClientConfidential {
				return nil, usecase.Conflict("NOT_CONFIDENTIAL", "Only CONFIDENTIAL clients have rotatable secrets")
			}
			plaintext, ref, err := generateSecret()
			if err != nil {
				return nil, usecase.Internal("SECRET", "generate client secret failed", err)
			}
			c.SetSecretRef(ref)
			stashSecret(c.ID, plaintext)

			event := OAuthClientSecretRotated{
				Metadata:      usecase.NewEventMetadata(ec, OAuthClientSecretRotatedType, Source, oauthSubject(c.ID)),
				OAuthClientID: c.ID,
			}
			return usecaseop.Save(c, repo, event), nil
		},
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

// generateSecret mints a random client secret and returns it alongside
// its encrypted reference (the value stored in client_secret_ref).
// Mirrors Rust: the secret is reversibly encrypted with FLOWCATALYST_APP_KEY
// and verified at /oauth/token by decrypt-and-compare. Fails if no app
// key is configured rather than storing a plaintext or unverifiable secret.
func generateSecret() (plaintext, ref string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	enc, err := encryption.FromEnv()
	if err != nil {
		return "", "", err
	}
	if enc == nil {
		return "", "", errors.New("FLOWCATALYST_APP_KEY not configured; cannot encrypt client secret")
	}
	encrypted, err := enc.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}
	// Store with the "encrypted:" prefix so the persisted string is
	// byte-identical to what Rust writes (oauth_clients_api.rs:
	// format!("encrypted:{}", encrypted)). Decrypt strips the prefix on
	// read, so verification is unaffected; this keeps client_secret_ref
	// values uniform across a mixed Go/Rust deployment.
	ref = "encrypted:" + encrypted
	return plaintext, ref, nil
}
