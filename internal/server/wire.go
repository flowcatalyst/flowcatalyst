package server

import (
	"fmt"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	applicationapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/api"
	applicationops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	auditapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	authapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/api"
	authops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/payload"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/provider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/api"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	connectionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/api"
	connectionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	corsapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/api"
	corsops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	dispatchjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	dispatchpoolapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/api"
	dispatchpoolops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	emaildomainapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/api"
	emaildomainops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	eventapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/event/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	eventtypeapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/api"
	eventtypeops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	identityproviderapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/api"
	identityproviderops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	platformconfigapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/api"
	platformconfigops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	processapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/api"
	processops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/api"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	scheduledjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/api"
	scheduledjobops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	serviceaccountapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/api"
	serviceaccountops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	bff "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	platformsink "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	sdkapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	subscriptionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/api"
	subscriptionops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn"
	webauthnapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/api"
	webauthnops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// WirePlatform instantiates every subdomain's repository + operations +
// HTTP routes against the supplied pool and registers them on r. The
// resulting router is the same surface the Rust fc-platform exposes.
//
// This function is the single source of truth for which subdomains the
// platform API serves. Adding a new subdomain is a four-line ritual:
// build the repo, build the use cases, build the api.State, call
// api.RegisterRoutes(r, &state).
func WirePlatform(r chi.Router, pool *pgxpool.Pool, cfg EnvCfg) error {
	sink := platformsink.New()
	uow := usecasepgx.New(pool, sink)

	// ── Repos ───────────────────────────────────────────────────────────
	clientRepo := client.NewRepository(pool)
	roleRepo := role.NewRepository(pool)
	applicationRepo := application.NewRepository(pool)
	applicationClientConfigRepo := application.NewClientConfigRepo(pool)
	principalRepo := principal.NewRepository(pool)
	principalGrantRepo := principal.NewClientAccessGrantRepo(pool)
	serviceAccountRepo := serviceaccount.NewRepository(pool)
	authRepo := auth.NewRepository(pool)
	authPayloadRepo := payload.NewRepository(pool)
	corsRepo := cors.NewRepository(pool)
	connectionRepo := connection.NewRepository(pool)
	subscriptionRepo := subscription.NewRepository(pool)
	dispatchPoolRepo := dispatchpool.NewRepository(pool)
	dispatchJobRepo := dispatchjob.NewRepository(pool)
	eventTypeRepo := eventtype.NewRepository(pool)
	eventRepo := event.NewRepository(pool)
	auditRepo := audit.NewRepository(pool)
	idpRepo := identityprovider.NewRepository(pool)
	edmRepo := emaildomainmapping.NewRepository(pool)
	platformConfigRepo := platformconfig.NewRepository(pool)
	processRepo := process.NewRepository(pool)
	scheduledJobRepo := scheduledjob.NewRepository(pool)
	webauthnCredRepo := webauthn.NewRepository(pool)
	webauthnCeremonyRepo := webauthn.NewCeremonyRepository(pool)

	// ── OAuth provider (fosite) ────────────────────────────────────────
	// SigningKey is supplied via cfg.JWTSigningKeyPath in production. In
	// dev we fall back to a generated ephemeral key so the binary can
	// boot without filesystem deps. See fc-dev for the persistent-key
	// path used by local development.
	signingKey := LoadSigningKeyOrEphemeral(cfg.JWTSigningKeyPath)
	authProvider, err := provider.NewProvider(provider.Config{
		Issuer:       cfg.JWTIssuer,
		SigningKey:   signingKey,
		SigningKeyID: cfg.JWTSigningKeyID,
		GlobalSecret: []byte(cfg.OAuthGlobalSecret),
	}, authRepo, authPayloadRepo, principalRepo, roleRepo)
	if err != nil {
		return fmt.Errorf("auth provider init: %w", err)
	}

	// ── Webauthn service ───────────────────────────────────────────────
	webauthnService, err := webauthn.NewService(webauthn.Config{
		RPDisplayName: "FlowCatalyst",
		RPID:          envOr("FC_WEBAUTHN_RP_ID", "localhost"),
		RPOrigins:     []string{envOr("FC_WEBAUTHN_RP_ORIGIN", "http://localhost:3000")},
	}, webauthnCredRepo, webauthnCeremonyRepo)
	if err != nil {
		return fmt.Errorf("webauthn service init: %w", err)
	}

	// ── Platform middleware + routes ────────────────────────────────────
	// Wrap the platform routes in a chi Group so the middleware applies
	// only to platform routes, not to whatever surrounding routes the
	// caller (fc-dev, fc-server) registered around us (e.g. /health).
	// chi requires middleware to be defined before any routes on a given
	// mux; the Group creates its own scope so that ordering rule is
	// satisfied locally regardless of caller ordering.
	r.Group(func(r chi.Router) {
		r.Use(platformmw.CorrelationID)
		r.Use(platformmw.Authenticator(platformmw.AuthConfig{
			Provider:         authProvider,
			AllowTestHeaders: cfg.AuthAllowTestHeaders,
		}))

		// ── api.State + RegisterRoutes per subdomain ───────────────────
		clientapi.RegisterRoutes(r, &clientapi.State{
			Repo:       clientRepo,
			CreateUC:   clientops.NewCreateUseCase(clientRepo, uow),
			UpdateUC:   clientops.NewUpdateUseCase(clientRepo, uow),
			ActivateUC: clientops.NewActivateUseCase(clientRepo, uow),
			SuspendUC:  clientops.NewSuspendUseCase(clientRepo, uow),
			AddNoteUC:  clientops.NewAddNoteUseCase(clientRepo, uow),
			DeleteUC:   clientops.NewDeleteUseCase(clientRepo, uow),
		})

		roleapi.RegisterRoutes(r, &roleapi.State{
			Repo:     roleRepo,
			CreateUC: roleops.NewCreateUseCase(roleRepo, uow),
			UpdateUC: roleops.NewUpdateUseCase(roleRepo, uow),
			DeleteUC: roleops.NewDeleteUseCase(roleRepo, uow),
		})

		applicationapi.RegisterRoutes(r, &applicationapi.State{
			Repo:                 applicationRepo,
			ClientConfigRepo:     applicationClientConfigRepo,
			CreateUC:             applicationops.NewCreateUseCase(applicationRepo, uow),
			UpdateUC:             applicationops.NewUpdateUseCase(applicationRepo, uow),
			ActivateUC:           applicationops.NewActivateUseCase(applicationRepo, uow),
			DeactivateUC:         applicationops.NewDeactivateUseCase(applicationRepo, uow),
			DeleteUC:             applicationops.NewDeleteUseCase(applicationRepo, uow),
			AttachServiceAccount: applicationops.NewAttachServiceAccountUseCase(applicationRepo, principalRepo, uow),
			EnableForClient:      applicationops.NewEnableForClientUseCase(applicationRepo, clientRepo, applicationClientConfigRepo, uow),
			DisableForClient:     applicationops.NewDisableForClientUseCase(applicationClientConfigRepo, uow),
		})

		principalapi.RegisterRoutes(r, &principalapi.State{
			Repo:                      principalRepo,
			GrantRepo:                 principalGrantRepo,
			CreateUC:                  principalops.NewCreateUseCase(principalRepo, uow),
			UpdateUC:                  principalops.NewUpdateUseCase(principalRepo, uow),
			ActivateUC:                principalops.NewActivateUseCase(principalRepo, uow),
			DeactivateUC:              principalops.NewDeactivateUseCase(principalRepo, uow),
			DeleteUC:                  principalops.NewDeleteUseCase(principalRepo, uow),
			ResetPasswordUC:           principalops.NewResetPasswordUseCase(principalRepo, uow),
			AssignRolesUC:             principalops.NewAssignRolesUseCase(principalRepo, roleRepo, uow),
			AssignApplicationAccessUC: principalops.NewAssignApplicationAccessUseCase(principalRepo, applicationRepo, uow),
			GrantClientAccessUC:       principalops.NewGrantClientAccessUseCase(principalRepo, clientRepo, principalGrantRepo, uow),
			RevokeClientAccessUC:      principalops.NewRevokeClientAccessUseCase(principalRepo, principalGrantRepo, uow),
		})

		serviceaccountapi.RegisterRoutes(r, &serviceaccountapi.State{
			Repo:               serviceAccountRepo,
			CreateUC:           serviceaccountops.NewCreateUseCase(serviceAccountRepo, uow),
			UpdateUC:           serviceaccountops.NewUpdateUseCase(serviceAccountRepo, uow),
			DeactivateUC:       serviceaccountops.NewDeactivateUseCase(serviceAccountRepo, uow),
			DeleteUC:           serviceaccountops.NewDeleteUseCase(serviceAccountRepo, uow),
			AssignRolesUC:      serviceaccountops.NewAssignRolesUseCase(serviceAccountRepo, uow),
			RegenerateTokenUC:  serviceaccountops.NewRegenerateAuthTokenUseCase(serviceAccountRepo, uow),
			RegenerateSecretUC: serviceaccountops.NewRegenerateSigningSecretUseCase(serviceAccountRepo, uow),
		})

		authapi.RegisterRoutes(r, &authapi.State{
			Repo:                  authRepo,
			CreateOAuthClient:     authops.NewCreateOAuthClientUseCase(authRepo.OAuthClients, uow),
			UpdateOAuthClient:     authops.NewUpdateOAuthClientUseCase(authRepo.OAuthClients, uow),
			ActivateOAuthClient:   authops.NewActivateOAuthClientUseCase(authRepo.OAuthClients, uow),
			DeactivateOAuthClient: authops.NewDeactivateOAuthClientUseCase(authRepo.OAuthClients, uow),
			DeleteOAuthClient:     authops.NewDeleteOAuthClientUseCase(authRepo.OAuthClients, uow),
			RotateSecret:          authops.NewRotateOAuthClientSecretUseCase(authRepo.OAuthClients, uow),
			CreateAnchorDomain:    authops.NewCreateAnchorDomainUseCase(authRepo.AnchorDomains, uow),
			UpdateAnchorDomain:    authops.NewUpdateAnchorDomainUseCase(authRepo.AnchorDomains, uow),
			DeleteAnchorDomain:    authops.NewDeleteAnchorDomainUseCase(authRepo.AnchorDomains, uow),
			CreateAuthConfig:      authops.NewCreateAuthConfigUseCase(authRepo.ClientAuthConfigs, uow),
			UpdateAuthConfig:      authops.NewUpdateAuthConfigUseCase(authRepo.ClientAuthConfigs, uow),
			DeleteAuthConfig:      authops.NewDeleteAuthConfigUseCase(authRepo.ClientAuthConfigs, uow),
			CreateIdpRoleMapping:  authops.NewCreateIdpRoleMappingUseCase(authRepo.IdpRoleMappings, uow),
			DeleteIdpRoleMapping:  authops.NewDeleteIdpRoleMappingUseCase(authRepo.IdpRoleMappings, uow),
		})

		// OAuth provider routes (token, authorize, revoke, introspect, .well-known/*)
		provider.NewTokenEndpoint(authProvider).RegisterRoutes(r)
		provider.NewAuthorizeEndpoint(authProvider).RegisterRoutes(r)
		provider.NewRevokeEndpoint(authProvider).RegisterRoutes(r)
		provider.NewIntrospectEndpoint(authProvider).RegisterRoutes(r)
		if disc, err := provider.NewDiscoveryEndpoint(provider.Config{
			Issuer:       cfg.JWTIssuer,
			SigningKey:   signingKey,
			SigningKeyID: cfg.JWTSigningKeyID,
		}, cfg.JWTIssuer); err == nil {
			disc.RegisterRoutes(r)
		}

		corsapi.RegisterRoutes(r, &corsapi.State{
			Repo:     corsRepo,
			AddUC:    corsops.NewAddUseCase(corsRepo, uow),
			DeleteUC: corsops.NewDeleteUseCase(corsRepo, uow),
		})

		connectionapi.RegisterRoutes(r, &connectionapi.State{
			Repo:     connectionRepo,
			CreateUC: connectionops.NewCreateUseCase(connectionRepo, uow),
			UpdateUC: connectionops.NewUpdateUseCase(connectionRepo, uow),
			DeleteUC: connectionops.NewDeleteUseCase(connectionRepo, uow),
		})

		subscriptionapi.RegisterRoutes(r, &subscriptionapi.State{
			Repo:     subscriptionRepo,
			CreateUC: subscriptionops.NewCreateUseCase(subscriptionRepo, uow),
			UpdateUC: subscriptionops.NewUpdateUseCase(subscriptionRepo, uow),
			DeleteUC: subscriptionops.NewDeleteUseCase(subscriptionRepo, uow),
			PauseUC:  subscriptionops.NewPauseUseCase(subscriptionRepo, uow),
			ResumeUC: subscriptionops.NewResumeUseCase(subscriptionRepo, uow),
		})

		dispatchpoolapi.RegisterRoutes(r, &dispatchpoolapi.State{
			Repo:      dispatchPoolRepo,
			CreateUC:  dispatchpoolops.NewCreateUseCase(dispatchPoolRepo, uow),
			UpdateUC:  dispatchpoolops.NewUpdateUseCase(dispatchPoolRepo, uow),
			ArchiveUC: dispatchpoolops.NewArchiveUseCase(dispatchPoolRepo, uow),
			DeleteUC:  dispatchpoolops.NewDeleteUseCase(dispatchPoolRepo, uow),
		})

		eventtypeapi.RegisterRoutes(r, &eventtypeapi.State{
			Repo:        eventTypeRepo,
			CreateUC:    eventtypeops.NewCreateUseCase(eventTypeRepo, uow),
			UpdateUC:    eventtypeops.NewUpdateUseCase(eventTypeRepo, uow),
			DeleteUC:    eventtypeops.NewDeleteUseCase(eventTypeRepo, uow),
			AddSchemaUC: eventtypeops.NewAddSchemaUseCase(eventTypeRepo, uow),
		})

		eventapi.RegisterRoutes(r, &eventapi.State{Repo: eventRepo})
		auditapi.RegisterRoutes(r, &auditapi.State{Repo: auditRepo})
		dispatchjobapi.RegisterRoutes(r, &dispatchjobapi.State{Repo: dispatchJobRepo})

		identityproviderapi.RegisterRoutes(r, &identityproviderapi.State{
			Repo:     idpRepo,
			CreateUC: identityproviderops.NewCreateUseCase(idpRepo, uow),
			UpdateUC: identityproviderops.NewUpdateUseCase(idpRepo, uow),
			DeleteUC: identityproviderops.NewDeleteUseCase(idpRepo, uow),
		})

		emaildomainapi.RegisterRoutes(r, &emaildomainapi.State{
			Repo:     edmRepo,
			CreateUC: emaildomainops.NewCreateUseCase(edmRepo, uow),
			UpdateUC: emaildomainops.NewUpdateUseCase(edmRepo, uow),
			DeleteUC: emaildomainops.NewDeleteUseCase(edmRepo, uow),
		})

		platformconfigapi.RegisterRoutes(r, &platformconfigapi.State{
			Repo:           platformConfigRepo,
			SetPropertyUC:  platformconfigops.NewSetPropertyUseCase(platformConfigRepo, uow),
			GrantAccessUC:  platformconfigops.NewGrantAccessUseCase(platformConfigRepo, uow),
			RevokeAccessUC: platformconfigops.NewRevokeAccessUseCase(platformConfigRepo, uow),
		})

		processapi.RegisterRoutes(r, &processapi.State{
			Repo:      processRepo,
			CreateUC:  processops.NewCreateUseCase(processRepo, uow),
			UpdateUC:  processops.NewUpdateUseCase(processRepo, uow),
			ArchiveUC: processops.NewArchiveUseCase(processRepo, uow),
			DeleteUC:  processops.NewDeleteUseCase(processRepo, uow),
		})

		scheduledjobapi.RegisterRoutes(r, &scheduledjobapi.State{
			Repo:      scheduledJobRepo,
			CreateUC:  scheduledjobops.NewCreateUseCase(scheduledJobRepo, uow),
			UpdateUC:  scheduledjobops.NewUpdateUseCase(scheduledJobRepo, uow),
			PauseUC:   scheduledjobops.NewPauseUseCase(scheduledJobRepo, uow),
			ResumeUC:  scheduledjobops.NewResumeUseCase(scheduledJobRepo, uow),
			ArchiveUC: scheduledjobops.NewArchiveUseCase(scheduledJobRepo, uow),
			DeleteUC:  scheduledjobops.NewDeleteUseCase(scheduledJobRepo, uow),
			FireNowUC: scheduledjobops.NewFireNowUseCase(scheduledJobRepo, uow),
		})

		webauthnapi.RegisterRoutes(r, &webauthnapi.State{
			Service:        webauthnService,
			Principals:     principalRepo,
			RegisterUC:     webauthnops.NewRegisterUseCase(webauthnCredRepo, uow),
			AuthenticateUC: webauthnops.NewAuthenticateUseCase(webauthnCredRepo, uow),
			RevokeUC:       webauthnops.NewRevokeUseCase(webauthnCredRepo, uow),
		})

		// Shared BFF/SDK endpoints (dashboard + SDK ingest)
		bff.RegisterRoutes(r, &bff.DashboardState{Pool: pool})
		sdkapi.RegisterRoutes(r, &sdkapi.DispatchJobsBatchState{Repo: dispatchJobRepo})
	})

	return nil
}
