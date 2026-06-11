package operations

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// ProvisionServiceAccountCommand provisions a dedicated service account
// (+ its SERVICE principal + a confidential OAuth client) for an
// application, atomically. Mirrors Rust fc-platform's
// provision_service_account, which runs all three writes in one
// transaction.
type ProvisionServiceAccountCommand struct {
	ApplicationID string `json:"applicationId"`
}

// ProvisionServiceAccountResult carries the freshly-minted identifiers.
// The OAuth client secret is plaintext and only returned once — it's
// stored hashed (argon2id) on the client row.
type ProvisionServiceAccountResult struct {
	ServiceAccountID   string
	ServiceAccountCode string
	ServiceAccountName string
	// ServicePrincipalID is the SERVICE principal id (`sac_…`) linked to
	// the service account.
	ServicePrincipalID string
	// OAuthClientRowID is the OAuth client's internal row id (`oac_…`).
	OAuthClientRowID string
	// OAuthClientID is the public `client_id` used in OAuth flows.
	OAuthClientID     string
	OAuthClientSecret string
}

// ProvisionServiceAccount creates the service account, its linked
// SERVICE principal (required by the app→principal FK), attaches it to
// the application, and creates a confidential OAuth client — all in a
// single transaction. On any failure every write rolls back.
func ProvisionServiceAccount(
	ctx context.Context,
	appRepo *application.Repository,
	saRepo *serviceaccount.Repository,
	principals *principal.Repository,
	oauthRepo *platformauth.OAuthClientRepo,
	uow *usecasepgx.UnitOfWork,
	cmd ProvisionServiceAccountCommand,
	ec usecase.ExecutionContext,
) (ProvisionServiceAccountResult, error) {
	var zero ProvisionServiceAccountResult

	if strings.TrimSpace(cmd.ApplicationID) == "" {
		return zero, usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
	}
	app, err := appRepo.FindByID(ctx, cmd.ApplicationID)
	if err != nil {
		return zero, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if app == nil {
		return zero, httperror.NotFound("Application", cmd.ApplicationID)
	}
	if app.ServiceAccountID != nil && *app.ServiceAccountID != "" {
		return zero, usecase.Conflict("ALREADY_PROVISIONED",
			"Application already has a service account provisioned")
	}

	// Build every aggregate up-front so the IDs + plaintext secret live
	// in locals we can return after the tx commits. (usecase.Success can
	// only be minted by a Commit* call, so Run must return one of the
	// committed event types — never a custom struct.)
	saCode := "app:" + app.Code
	saName := app.Name + " Service Account"
	desc := "Service account for application: " + app.Name
	appID := app.ID

	sa := serviceaccount.New(saCode, saName)
	sa.Description = &desc
	sa.ApplicationID = &appID

	saPrincipal := principal.NewService(sa.ID, saName)

	plaintext, ref, err := generateClientSecret()
	if err != nil {
		return zero, usecase.Internal("SECRET", "generate client secret failed", err)
	}
	oauthClientID := tsid.Generate(tsid.OAuthClient)
	oc := platformauth.NewOAuthClient(oauthClientID, app.Name+" Service Account Client", platformauth.OAuthClientConfidential)
	oc.SetSecretRef(ref)
	oc.PrincipalID = &saPrincipal.ID
	oc.GrantTypes = []string{"client_credentials", "refresh_token"}
	oc.Scopes = []string{"openid"}

	res := usecasepgx.Run(ctx, uow, func(s *usecasepgx.TxScopedUnitOfWork) usecase.Result[authops.OAuthClientCreated] {
		// 1. Service account.
		if r := usecasepgx.CommitScoped(ctx, s, sa, saRepo,
			saops.NewServiceAccountCreatedEvent(ec, sa.ID, sa.Code, sa.Name), cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return usecase.Failure[authops.OAuthClientCreated](e)
		}

		// 2. Linked SERVICE principal — raw persist (no separate event;
		//    the principal row is a persistence detail of SA creation,
		//    matching Rust where the principal is created "behind" the SA).
		//    Scope the SA to its own application: AllApplications=false plus a
		//    single application-access grant. The token's `applications` claim
		//    then carries exactly this app, and resource-level authorization
		//    (sdksync.requireAppAccess) confines the SA's writes to it — even at
		//    anchor tier. AppAccessPersister writes the base row AND the
		//    iam_principal_application_access junction in this transaction.
		saPrincipal.AllApplications = false
		saPrincipal.AccessibleApplicationIDs = []string{app.ID}
		if err := s.WithTx(ctx, func(tx pgx.Tx) error {
			return principal.AppAccessPersister{Repository: principals}.Persist(
				ctx, saPrincipal, usecasepgx.WrapTxForBootstrap(tx))
		}); err != nil {
			return usecase.Failure[authops.OAuthClientCreated](
				usecase.Internal("PERSIST", "service principal persist failed", err))
		}

		// 3. Attach the SA's principal to the application.
		app.ServiceAccountID = &saPrincipal.ID
		app.UpdatedAt = time.Now().UTC()
		attached := ApplicationServiceAccountProvisionedEvent{
			Metadata:           usecase.NewEventMetadata(ec, ApplicationServiceAccountProvisioned, Source, subjectFor(app.ID)),
			ApplicationID:      app.ID,
			ApplicationCode:    app.Code,
			ServiceAccountID:   sa.ID,
			ServiceAccountCode: saCode,
		}
		if r := usecasepgx.CommitScoped(ctx, s, app, appRepo, attached, cmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return usecase.Failure[authops.OAuthClientCreated](e)
		}

		// 4. Confidential OAuth client (last write → its result is the
		//    Run result).
		return usecasepgx.CommitScoped(ctx, s, oc, oauthRepo,
			authops.NewOAuthClientCreatedEvent(ec, oc.ID, oc.ClientID, oc.ClientName), cmd)
	})

	if _, err := usecase.Into(res); err != nil {
		return zero, err
	}
	return ProvisionServiceAccountResult{
		ServiceAccountID:   sa.ID,
		ServiceAccountCode: saCode,
		ServiceAccountName: saName,
		ServicePrincipalID: saPrincipal.ID,
		OAuthClientRowID:   oc.ID,
		OAuthClientID:      oc.ClientID,
		OAuthClientSecret:  plaintext,
	}, nil
}

// generateClientSecret returns a fresh URL-safe secret + its encrypted
// reference (client_secret_ref). Same scheme as
// auth/operations.generateSecret — Rust parity: secrets are reversibly
// encrypted, not hashed.
func generateClientSecret() (plaintext, ref string, err error) {
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
	ref, err = enc.Encrypt(plaintext)
	if err != nil {
		return "", "", err
	}
	return plaintext, ref, nil
}
