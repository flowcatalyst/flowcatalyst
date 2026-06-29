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
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// applicationServiceRoleName is the seeded role granted to every
// application service account at provision time. It carries the
// least-privilege platform:application-service:* permission set (event-type,
// subscription, role, and process sync), all scoped to the SA's own
// application. See internal/platform/seed/roles.go ("application-service").
const applicationServiceRoleName = "platform:application-service"

// ptrStr returns a pointer to s. Used for the optional AssignmentSource on a
// role assignment.
func ptrStr(s string) *string { return &s }

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
//
// This is a multi-aggregate orchestration: it writes several aggregates and
// returns a custom result, so it uses the [usecaseop.TxOperation] envelope
// run by [usecaseop.RunTx]. An application is platform-level, so the use case
// has no resource-level access check — the coarse anchor requirement
// (auth.RequireAnchor) is enforced at the controller.
func ProvisionServiceAccount(
	appRepo *application.Repository,
	saRepo *serviceaccount.Repository,
	principals *principal.Repository,
	oauthRepo *platformauth.OAuthClientRepo,
) usecaseop.TxOperation[ProvisionServiceAccountCommand, ProvisionServiceAccountResult] {
	return usecaseop.TxOperation[ProvisionServiceAccountCommand, ProvisionServiceAccountResult]{
		Name: "ProvisionServiceAccount",
		Validate: func(_ context.Context, cmd ProvisionServiceAccountCommand) error {
			if strings.TrimSpace(cmd.ApplicationID) == "" {
				return usecase.Validation("APPLICATION_ID_REQUIRED", "Application ID is required")
			}
			return nil
		},
		Authorize: usecaseop.Public[ProvisionServiceAccountCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd ProvisionServiceAccountCommand, ec usecase.ExecutionContext) (ProvisionServiceAccountResult, error) {
			var zero ProvisionServiceAccountResult

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
			// in locals we can return after the tx commits.
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
			// Limit the OAuth client to the application it was provisioned under
			// (oauth_client_application_ids). This formally links the client to its
			// application — it surfaces "under" the app in the OAuth-client UI and
			// makes the scoping explicit alongside the principal's app confinement.
			oc.ApplicationIDs = []string{app.ID}

			// 1. Service account.
			if r := usecasepgx.CommitScoped(ctx, s, sa, saRepo,
				saops.NewServiceAccountCreatedEvent(ec, sa.ID, sa.Code, sa.Name), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			// 2. Linked SERVICE principal — raw persist (no separate event;
			//    the principal row is a persistence detail of SA creation,
			//    matching Rust where the principal is created "behind" the SA).
			//    Scope the SA to its own application: AllApplications=false plus a
			//    single application-access grant. The token's `applications` claim
			//    then carries exactly this app, and resource-level authorization
			//    (sdksync.requireAppAccess) confines the SA's writes to it — even at
			//    anchor tier.
			//
			//    The SA's principal is also granted the seeded
			//    platform:application-service role: without a role its token carries
			//    an empty permissions claim and leans entirely on the anchor-tier
			//    bypass for every sync endpoint. That role grants exactly the
			//    least-privilege permission set an application SA needs (event-type /
			//    subscription / role / process sync), so the token reflects real
			//    granted scope and the role surfaces in the UI.
			//
			//    RolesAndAppAccessPersister writes the base row AND both the
			//    iam_principal_roles and iam_principal_application_access junctions
			//    in this transaction.
			saPrincipal.AllApplications = false
			saPrincipal.AccessibleApplicationIDs = []string{app.ID}
			saPrincipal.Roles = []serviceaccount.RoleAssignment{{
				Role:             applicationServiceRoleName,
				AssignmentSource: ptrStr("PROVISIONED"),
				AssignedAt:       time.Now().UTC(),
			}}
			if err := s.WithTx(ctx, func(tx pgx.Tx) error {
				return principal.RolesAndAppAccessPersister{Repository: principals}.Persist(
					ctx, saPrincipal, usecasepgx.WrapTxForBootstrap(tx))
			}); err != nil {
				return zero, usecase.Internal("PERSIST", "service principal persist failed", err)
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
				return zero, e
			}

			// 4. Confidential OAuth client.
			if r := usecasepgx.CommitScoped(ctx, s, oc, oauthRepo,
				authops.NewOAuthClientCreatedEvent(ec, oc.ID, oc.ClientID, oc.ClientName), cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
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
		},
	}
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
