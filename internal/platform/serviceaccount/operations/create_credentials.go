package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/validate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// CreateWithCredentialsResult carries the freshly-minted account plus the
// one-time plaintext secrets. The OAuth client secret is verify-only and
// stored as a keyed hash; the webhook token + signing secret are things the
// platform must send/sign with later, so they stay reversibly encrypted
// (see serviceaccount.Repository.encryptCreds). Either way, the plaintext is
// only ever returned here, once.
type CreateWithCredentialsResult struct {
	ServiceAccount    *serviceaccount.ServiceAccount
	PrincipalID       string
	OAuthClientRowID  string
	OAuthClientID     string
	OAuthClientSecret string
	AuthToken         string
	SigningSecret     string
}

// CreateServiceAccountWithCredentials creates a service account, its linked
// SERVICE principal, and a confidential OAuth client in a single
// transaction, and mints a webhook bearer token + HMAC signing secret on the
// account. Mirrors the application provision_service_account flow but for
// standalone service-account creation. Returns every identifier + plaintext
// secret so the HTTP handler can present them once.
//
// This is a multi-aggregate orchestration: Execute receives the open
// transaction and drives every write through the scoped commit helpers
// (usecasepgx.CommitScoped / s.WithTx + WrapTxForBootstrap) so the service
// account, its principal, and the OAuth client land atomically with their
// domain events. The coarse "may write service accounts" permission stays on
// the controller; there is no per-client resource check here, so Authorize is
// usecaseop.Public.
func CreateServiceAccountWithCredentials(
	saRepo *serviceaccount.Repository,
	principals *principal.Repository,
	oauthRepo *platformauth.OAuthClientRepo,
	clients *client.Repository,
	grants *principal.ClientAccessGrantRepo,
	publish credentialpublish.Config,
) usecaseop.TxOperation[CreateCommand, CreateWithCredentialsResult] {
	return usecaseop.TxOperation[CreateCommand, CreateWithCredentialsResult]{
		Name: "CreateServiceAccountWithCredentials",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			if code == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if strings.HasPrefix(code, "app:") {
				return usecase.Validation("RESERVED_CODE",
					"codes starting with 'app:' are reserved for application service accounts")
			}
			if !validate.CodePattern.MatchString(code) {
				return usecase.Validation("INVALID_CODE_FORMAT",
					"code must start with a lowercase letter and contain only lowercase alphanumeric and hyphens")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if cmd.AllApplications != nil && *cmd.AllApplications &&
				cmd.ApplicationID != nil && strings.TrimSpace(*cmd.ApplicationID) != "" {
				return usecase.Validation("ALL_APPLICATIONS_WITH_APPLICATION_ID",
					"allApplications cannot be combined with applicationId")
			}
			// Asked for while publishing is not configured: refused here, before
			// anything is written, so the caller does not believe the
			// credentials were copied somewhere.
			_, err := publish.Want(cmd.PublishCredentials)
			return err
		},
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CreateCommand, ec usecase.ExecutionContext) (CreateWithCredentialsResult, error) {
			var zero CreateWithCredentialsResult

			code := strings.ToLower(strings.TrimSpace(cmd.Code))
			existing, err := saRepo.FindByCode(ctx, code)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return zero, usecase.Conflict("CODE_EXISTS", "Service account with code '"+code+"' already exists")
			}

			sa := serviceaccount.New(code, strings.TrimSpace(cmd.Name))
			sa.Description = cmd.Description
			sa.Scope = cmd.Scope
			sa.ApplicationID = ids.PtrOf[ids.ApplicationID](cmd.ApplicationID)
			if cmd.ClientIDs != nil {
				sa.ClientIDs = cmd.ClientIDs
			}
			if err := requireScopeMatchesReach(sa.Scope, sa.ClientIDs); err != nil {
				return zero, err
			}
			authToken := generateAuthToken()
			signingSecret := generateSigningSecret()
			sa.WebhookCredentials = serviceaccount.WebhookCredentials{
				AuthType:      serviceaccount.AuthBearer,
				Token:         &authToken,
				SigningSecret: &signingSecret,
			}

			plaintext, ref, err := generateOAuthClientSecret()
			if err != nil {
				return zero, usecase.Internal("SECRET", "generate client secret failed", err)
			}
			if err := queuePublication(publish, sa, cmd.PublishCredentials, cmd.PublishBasePath, "", ec.PrincipalID, &plaintext); err != nil {
				return zero, err
			}

			saPrincipal := principal.NewService(sa.ID, sa.Name)
			// No application access unless asked for: an account made here
			// starts with none, and access is granted explicitly afterwards.
			saPrincipal.AllApplications = cmd.AllApplications != nil && *cmd.AllApplications
			// Reach follows the account's client links. Without this the
			// principal is always ANCHOR, so a token built from it reaches every
			// tenant however the account was linked.
			if err := requireClientsExist(ctx, clients, sa.ClientIDs); err != nil {
				return zero, err
			}
			grantClientIDs := applyClientReach(saPrincipal, sa.ClientIDs)

			oauthClientID := tsid.Generate(tsid.OAuthClient)
			oc := platformauth.NewOAuthClient(oauthClientID, sa.Name+" Client", platformauth.OAuthClientConfidential)
			oc.SetSecretRef(ref)
			oc.PrincipalID = ids.StringPtr(&saPrincipal.ID)
			oc.GrantTypes = []string{"client_credentials", "refresh_token"}
			oc.Scopes = []string{"openid"}

			// 1. Service account.
			if r := usecasepgx.CommitScoped(ctx, s, sa, saRepo,
				NewServiceAccountCreatedEvent(ec, sa.ID, sa.Code, sa.Name), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}
			// 2. Linked SERVICE principal (persistence detail of SA creation).
			//    When the SA is created for a specific application, confine it
			//    exactly like the application-provision flow: AllApplications=false
			//    plus a single application-access grant, so the token's
			//    `applications` claim carries only that app and
			//    sdksync.requireAppAccess confines its writes. The id is stored
			//    as supplied (no existence check), matching the posture of
			//    iam_service_accounts.application_id on this endpoint.
			appScoped := cmd.ApplicationID != nil && strings.TrimSpace(*cmd.ApplicationID) != ""
			if appScoped {
				saPrincipal.AllApplications = false
				saPrincipal.AccessibleApplicationIDs = []ids.ApplicationID{ids.ApplicationID(*cmd.ApplicationID)}
			}
			persistPrincipal := func(tx pgx.Tx) error {
				return principal.ClientAssociationPersister{
					Repository:       principals,
					Grants:           grants,
					GrantClientIDs:   grantClientIDs,
					GrantedBy:        ec.PrincipalID,
					ReplaceAppAccess: appScoped,
				}.Persist(ctx, saPrincipal, usecasepgx.WrapTxForBootstrap(tx))
			}
			if err := s.WithTx(ctx, persistPrincipal); err != nil {
				return zero, usecase.Internal("PERSIST", "service principal persist failed", err)
			}
			// 3. Confidential OAuth client.
			if r := usecasepgx.CommitScoped(ctx, s, oc, oauthRepo,
				authops.NewOAuthClientCreatedEvent(ec, oc.ID, oc.ClientID, oc.ClientName), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			return CreateWithCredentialsResult{
				ServiceAccount:    sa,
				PrincipalID:       string(saPrincipal.ID),
				OAuthClientRowID:  oc.ID,
				OAuthClientID:     oc.ClientID,
				OAuthClientSecret: plaintext,
				AuthToken:         authToken,
				SigningSecret:     signingSecret,
			}, nil
		},
	}
}

// generateOAuthClientSecret returns a fresh URL-safe secret + its at-rest
// reference: a keyed hash ("hashed:v1:" + HMAC-SHA256(FLOWCATALYST_APP_KEY,
// plaintext)), not a reversible encryption — the OAuth client secret is
// verify-only, so there is nothing to decrypt back out. See
// auth/operations.generateSecret, which this mirrors.
func generateOAuthClientSecret() (plaintext, ref string, err error) {
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
		return "", "", errors.New("FLOWCATALYST_APP_KEY not configured; cannot hash client secret")
	}
	return plaintext, enc.Hash(plaintext), nil
}

// QueuePublication sets the publish a create that mints credentials hands to
// the account's Persist: enable publishing (the custom base path, else the
// template's for this account) and put the client id, the client secret (the
// plaintext just minted, which exists nowhere else), the auth token and the
// signing secret. Nothing is queued when publishing is not wanted. appCode is
// the account's application code, "" when it has none. Exported for the
// application provision flow, which mints the same four.
func QueuePublication(cfg credentialpublish.Config, sa *serviceaccount.ServiceAccount, want *bool, basePath *string, appCode, enabledBy string, clientSecret *string) error {
	return queuePublication(cfg, sa, want, basePath, appCode, enabledBy, clientSecret)
}

func queuePublication(cfg credentialpublish.Config, sa *serviceaccount.ServiceAccount, want *bool, basePath *string, appCode, enabledBy string, clientSecret *string) error {
	on, err := cfg.Want(want)
	if err != nil || !on {
		return err
	}
	base, err := cfg.ResolveBasePath(credentialpublish.OwnerServiceAccount,
		credentialpublish.Vars{Account: credentialpublish.AccountSegment(sa.Code), App: appCode}, basePath)
	if err != nil {
		return err
	}
	sa.Publish = credentialpublish.Pending{
		Enable: &credentialpublish.EnableRequest{BasePath: base, EnabledBy: enabledBy},
		Changes: []credentialpublish.Change{
			{Credential: credentialpublish.CredClientID},
			{Credential: credentialpublish.CredClientSecret, Plaintext: clientSecret},
			{Credential: credentialpublish.CredAuthToken},
			{Credential: credentialpublish.CredSigningSecret},
		},
	}
	return nil
}
