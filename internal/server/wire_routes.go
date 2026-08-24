package server

import (
	"encoding/json"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/appdocs"
	applicationapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/api"
	auditapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit/api"
	authapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/bridge"
	clientselectionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/clientselection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	clientapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/api"
	connectionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/api"
	corsapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/api"
	dispatchjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/api"
	dispatchpoolapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/docsapi"
	emaildomainapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/api"
	eventapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/event/api"
	eventtypeapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/api"
	identityproviderapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/api"
	loginattemptapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	passwordresetapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/passwordreset/api"
	platformconfigapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalauth"
	portalusersapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity/api"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	processapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/api"
	resetapprovalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/resetapproval/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	scheduledjobapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/sdksync"
	serviceaccountapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/api"
	bff "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/bff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httpcompat"
	meapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/me"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	sdkapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	subscriptionapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/api"
	webauthnapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/webauthn/api"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// registerPlatformAPI wires the authenticated platform surface: a chi
// Group carrying the CorrelationID + Authenticator middleware, the huma
// API every aggregate registers against, and the chi-mounted BFF/SDK/me
// endpoints. Returns the huma API so the unauthenticated spec/docs
// handlers (mounted on the parent router by registerSpecRoutes) can
// serve the generated OpenAPI document.
//
// Wrap the platform routes in a chi Group so the middleware applies
// only to platform routes, not to whatever surrounding routes the
// caller (fcdev, fc-server) registered around us (e.g. /health).
// chi requires middleware to be defined before any routes on a given
// mux; the Group creates its own scope so that ordering rule is
// satisfied locally regardless of caller ordering.
func registerPlatformAPI(r chi.Router, cfg EnvCfg, pool *pgxpool.Pool, uow *usecasepgx.UnitOfWork, repos *repoSet, svcs *serviceSet) huma.API {
	// humaAPI is assigned inside the auth Group below so its routes
	// inherit chi auth middleware. Captured at function scope so the
	// spec/docs handlers (mounted on the parent router OUTSIDE the
	// Group, so they remain unauthenticated for tooling — oasdiff,
	// hey-api codegen, the Hey-API frontend client) can access it.
	var humaAPI huma.API

	// Admin-triggered reset (POST /api/principals/{id}/send-password-reset)
	// shares the public flow's token repo + mailer.
	principalResetEmailer := passwordresetapi.NewPrincipalEmailer(repos.resetTokenRepo, cfg.JWTIssuer, svcs.emailSvc, repos.platformConfigRepo)

	r.Group(func(r chi.Router) {
		r.Use(platformmw.CorrelationID)
		r.Use(platformmw.Authenticator(platformmw.AuthConfig{
			Provider:         svcs.authProvider,
			AllowTestHeaders: cfg.AuthAllowTestHeaders,
		}))
		// /auth/me — needs the AuthContext, so mounted INSIDE the auth
		// group. /auth/check-domain + /auth/login + /auth/logout are
		// public (see registerPublicRoutes).
		svcs.loginEP.RegisterAuthenticatedRoutes(r)

		// huma API shared by every aggregate's Register call. Routes
		// register against this; the chi router scope above gives them
		// the same middleware (CorrelationID + Authenticator) as the
		// remaining chi handlers. OpenAPIPath/DocsPath cleared so huma
		// doesn't auto-mount inside the auth Group — we serve the spec
		// from the parent router (registerSpecRoutes).
		humaCfg := huma.DefaultConfig("FlowCatalyst Platform API", "dev")
		humaCfg.OpenAPIPath = ""
		humaCfg.DocsPath = ""
		// Drop huma's $schema link injection. The wire contract never emits it
		// and the field clutters response bodies that SPAs / SDKs parse
		// strictly. The OpenAPI document still describes every response
		// (served from the parent router via /openapi.json) — clients
		// that want the schema can fetch it there.
		humaCfg.SchemasPath = ""
		humaAPI = humachi.New(r, humaCfg)

		// ── api.State + RegisterRoutes per subdomain ───────────────────
		clientapi.Register(humaAPI, &clientapi.State{
			Repo:          repos.clientRepo,
			Applications:  repos.applicationRepo,
			ClientConfigs: repos.applicationClientConfigRepo,
			UoW:           uow,
		})

		roleapi.Register(humaAPI, &roleapi.State{
			Repo:        repos.roleRepo,
			Permissions: role.NewPermissionRepo(pool),
			UoW:         uow,
		})

		applicationapi.Register(humaAPI, &applicationapi.State{
			Repo:             repos.applicationRepo,
			ClientConfigRepo: repos.applicationClientConfigRepo,
			ClientRepo:       repos.clientRepo,
			Principals:       repos.principalRepo,
			Roles:            repos.roleRepo,
			ServiceAccounts:  repos.serviceAccountRepo,
			OAuthClients:     repos.authRepo.OAuthClients,
			UoW:              uow,
		})

		principalapi.Register(humaAPI, &principalapi.State{
			Repo:              repos.principalRepo,
			Versions:          svcs.principalVersions,
			GrantRepo:         repos.principalGrantRepo,
			Roles:             repos.roleRepo,
			Applications:      repos.applicationRepo,
			ClientConfigs:     repos.applicationClientConfigRepo,
			Clients:           repos.clientRepo,
			Mappings:          repos.edmRepo,
			IdentityProviders: repos.idpRepo,
			AnchorDomains:     repos.authRepo.AnchorDomains,
			PasswordEmailer:   principalResetEmailer,
			InviteEmailer:     principalResetEmailer,
			Notifier:          svcs.notifier,
			MFA:               svcs.mfaSvc,
			Audit:             repos.auditRepo,
			UoW:               uow,
		})

		portalusersapi.Register(humaAPI, &portalusersapi.State{
			Identities:   repos.portalIdentityRepo,
			Clients:      repos.clientRepo,
			OAuthClients: repos.authRepo.OAuthClients,
			UoW:          uow,
			Invites:      principalResetEmailer,
			IdPs:         repos.idpRepo,
		})

		// Phase 8: lost-device reset approval queue (client-admin gated).
		resetapprovalapi.Register(humaAPI, &resetapprovalapi.State{
			Approvals:  repos.resetApprovalRepo,
			Principals: repos.principalRepo,
			Sender:     principalResetEmailer,
		})

		serviceaccountapi.Register(humaAPI, &serviceaccountapi.State{
			Repo:         repos.serviceAccountRepo,
			Principals:   repos.principalRepo,
			OAuthClients: repos.authRepo.OAuthClients,
			UoW:          uow,
			// Admin token mint (POST /{id}/token) reuses the exact
			// client_credentials mint machinery.
			Auth:               svcs.authSvc,
			FlattenPermissions: svcs.oauthTokenEP.FlattenPermissions,
			Audit:              repos.auditRepo,
		})

		authapi.Register(humaAPI, &authapi.State{
			Repo:         repos.authRepo,
			Applications: repos.applicationRepo,
			UoW:          uow,
			Enc:          svcs.encSvc,
		})

		// OAuth provider routes (all hand-rolled: authservice + encryption)
		// are NOT registered here — the whole /oauth + /.well-known surface
		// lives outside this auth group, in registerPublicRoutes. See the
		// rationale there.

		// OIDC bridge — POST /auth/check-domain, GET /auth/oidc/login,
		// GET /auth/oidc/callback. The bridge resolves the external IDP
		// for an email's domain, drives the redirect dance, and on
		// callback either uses the existing FlowCatalyst Principal or
		// auto-provisions one via the EmailDomainMapping that drove the
		// login. The default SessionWriter just emits JSON; we override
		// here to mint a session-cookie JWT (same path as /auth/login)
		// so a successful SSO round-trip produces a usable browser
		// session.
		// Field-level encryption (FLOWCATALYST_APP_KEY) — nil-safe; the
		// bridge will surface a clear error if a confidential OIDC config
		// needs a secret and the key isn't set.
		appEnc, _ := encryption.FromEnv()
		bridgeClient := bridge.NewBridge(repos.edmRepo, repos.idpRepo, appEnc)
		loginStateRepo := bridge.NewLoginStateRepo(pool)
		bridgeLoginEP := bridge.NewLoginEndpoint(bridgeClient, loginStateRepo, repos.principalRepo, repos.edmRepo,
			repos.roleRepo, repos.authRepo.IdpRoleMappings, uow, repos.authRepo.OAuthClients)
		// Pin the OIDC callback URL to the configured public base instead of
		// deriving it from forwardable X-Forwarded-Proto/Host headers, and
		// give the logout cookie-clear the same Secure attribute the
		// SessionWriter sets below.
		bridgeLoginEP.ExternalBaseURL = cfg.JWTIssuer
		bridgeLoginEP.CookieSecure = !cfg.AuthAllowTestHeaders
		bridgeLoginEP.SessionWriter = func(w http.ResponseWriter, r *http.Request, principalID, returnURL string) {
			token, err := svcs.authProvider.MintSessionToken(r.Context(), principalID, login.SessionTTL)
			if err != nil {
				http.Error(w, "session mint failed: "+err.Error(), http.StatusInternalServerError)
				return
			}
			http.SetCookie(w, &http.Cookie{
				Name:     platformmw.SessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				Secure:   !cfg.AuthAllowTestHeaders,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   int(login.SessionTTL.Seconds()),
			})
			if returnURL != "" {
				http.Redirect(w, r, returnURL, http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"principalId": principalID})
		}
		// Per-IP rate limit on the public OIDC bridge routes (login start +
		// callback + session/end) — blunts authorization-code probing / DoS
		// without impeding a real interactive login.
		// Portal identity plane (docs/portal-identity-plan.md Phase 2.5 v2):
		// /portal/authorize + check-domain + password login live in
		// portalauth; the bridge serves the portal SSO start and callback
		// sink through the shared machinery.
		portalFlowRepo := portalauth.NewFlowRepo(pool)
		portalAuthState := &portalauth.State{
			OAuthClients:      repos.authRepo.OAuthClients,
			Identities:        repos.portalIdentityRepo,
			IdPs:              repos.idpRepo,
			Flows:             portalFlowRepo,
			AuthCodes:         grantstore.NewAuthorizationCodeRepository(pool),
			PasswordReset:     principalResetEmailer,
			RateLimit:         svcs.rlStore,
			RateLimitPolicies: svcs.rlPolicies,
		}
		bridgeLoginEP.Portal = &bridge.PortalBridge{
			Flows:      portalFlowRepo,
			Identities: repos.portalIdentityRepo,
			Clients:    repos.clientRepo,
			IssueCode:  portalAuthState.IssueCode,
		}
		oidcGov := ratelimit.NewGovernor(ratelimit.OIDCBridgeGovernorFromEnv())
		r.Group(func(g chi.Router) {
			g.Use(ratelimit.GovernorMiddleware(oidcGov, "Too many authentication requests"))
			bridgeLoginEP.RegisterRoutes(g)
			bridgeLoginEP.RegisterPortalRoutes(g)
			portalAuthState.RegisterRoutes(g)
		})

		corsapi.Register(humaAPI, &corsapi.State{
			Repo: repos.corsRepo,
			UoW:  uow,
		})

		connectionapi.Register(humaAPI, &connectionapi.State{
			Repo: repos.connectionRepo,
			UoW:  uow,
		})

		subscriptionapi.Register(humaAPI, &subscriptionapi.State{
			Repo: repos.subscriptionRepo,
			UoW:  uow,
		})

		dispatchpoolapi.Register(humaAPI, &dispatchpoolapi.State{
			Repo: repos.dispatchPoolRepo,
			UoW:  uow,
		})

		eventtypeapi.Register(humaAPI, &eventtypeapi.State{
			Repo: repos.eventTypeRepo,
			UoW:  uow,
		})

		// SDK self-registration ("sync") endpoints, scoped under
		// /api/applications/{appCode}.
		appDocsRepo := appdocs.NewRepository(pool)
		sdksync.Register(humaAPI, &sdksync.State{
			Apps:          repos.applicationRepo,
			EventTypes:    repos.eventTypeRepo,
			Roles:         repos.roleRepo,
			Subscriptions: repos.subscriptionRepo,
			Connections:   repos.connectionRepo,
			Processes:     repos.processRepo,
			DispatchPools: repos.dispatchPoolRepo,
			Principals:    repos.principalRepo,
			ScheduledJobs: repos.scheduledJobRepo,
			Specs:         openapispecs.NewRepository(pool),
			AppDocs:       appDocsRepo,
			UoW:           uow,
		})

		eventapi.Register(humaAPI, &eventapi.State{Repo: repos.eventRepo, Clients: repos.clientRepo})
		auditapi.Register(humaAPI, &auditapi.State{Repo: repos.auditRepo})

		// Documentation (admin-gated): the platform's published pages
		// (embedded docs/published/*.md) + application-synced pages.
		docsapi.Register(humaAPI, &docsapi.State{
			AppDocs: appDocsRepo,
			Apps:    repos.applicationRepo,
		})
		dispatchjobapi.Register(humaAPI, &dispatchjobapi.State{Repo: repos.dispatchJobRepo})

		identityproviderapi.Register(humaAPI, &identityproviderapi.State{
			Repo:       repos.idpRepo,
			Mappings:   repos.edmRepo,
			Principals: repos.principalRepo,
			UoW:        uow,
			Enc:        svcs.encSvc,
		})

		emaildomainapi.Register(humaAPI, &emaildomainapi.State{
			Repo:       repos.edmRepo,
			IDPRepo:    repos.idpRepo,
			Principals: repos.principalRepo,
			UoW:        uow,
		})

		loginattemptapi.Register(humaAPI, &loginattemptapi.State{Repo: repos.loginAttemptRepo})

		platformconfigapi.Register(humaAPI, &platformconfigapi.State{
			Repo: repos.platformConfigRepo,
			UoW:  uow,
		})

		processapi.Register(humaAPI, &processapi.State{
			Repo: repos.processRepo,
			UoW:  uow,
		})

		scheduledjobapi.Register(humaAPI, &scheduledjobapi.State{
			Repo:      repos.scheduledJobRepo,
			Instances: scheduledjob.NewInstanceRepository(pool),
			UoW:       uow,
		})

		webauthnapi.Register(humaAPI, &webauthnapi.State{
			Service:      svcs.webauthnService,
			Principals:   repos.principalRepo,
			Creds:        repos.webauthnCredRepo,
			UoW:          uow,
			Provider:     svcs.authProvider,
			CookieSecure: !cfg.AuthAllowTestHeaders,
			SessionTTL:   login.SessionTTL,
			Notifier:     svcs.notifier,
			// Passkey sign-ins record to the same attempt store and share
			// the same (email, IP) backoff budget as password logins.
			LoginAttempts: repos.loginAttemptRepo,
			BackoffPolicy: loginbackoff.PolicyFromEnv(),
		})

		// Shared BFF/SDK endpoints (dashboard + SDK ingest)
		bff.RegisterRoutes(r, &bff.DashboardState{Pool: pool})
		bff.RegisterFilterOptions(r, &bff.FilterOptionsState{
			Clients:    repos.clientRepo,
			EventTypes: repos.eventTypeRepo,
		})
		bff.RegisterEventTypes(r, &bff.EventTypesState{
			Repo: repos.eventTypeRepo,
			UoW:  uow,
		})
		bff.RegisterRoles(r, &bff.RolesState{
			Roles:        repos.roleRepo,
			Applications: repos.applicationRepo,
			Permissions:  role.NewPermissionRepo(pool),
			UoW:          uow,
		})
		bff.RegisterScheduledJobs(r, &bff.ScheduledJobsState{
			Jobs:         repos.scheduledJobRepo,
			Instances:    scheduledjob.NewInstanceRepository(pool),
			Clients:      repos.clientRepo,
			Applications: repos.applicationRepo,
		})
		bff.RegisterDeveloper(r, &bff.DeveloperState{
			Applications: repos.applicationRepo,
			Specs:        openapispecs.NewRepository(pool),
			EventTypes:   repos.eventTypeRepo,
			UoW:          uow,
			PlatformOpenAPI: func() (json.RawMessage, error) {
				// humaAPI is captured by closure — the OpenAPI() call
				// reflects whatever routes are registered at request time.
				return humaAPI.OpenAPI().MarshalJSON()
			},
		})
		meapi.RegisterRoutes(r, &meapi.State{Principals: repos.principalRepo, Applications: repos.applicationRepo, Clients: repos.clientRepo, AppConfigs: repos.applicationClientConfigRepo})
		clientselectionapi.RegisterRoutes(r, &clientselectionapi.State{
			Principals: repos.principalRepo,
			Clients:    repos.clientRepo,
			Roles:      repos.roleRepo,
			Grants:     repos.principalGrantRepo,
			Auth:       svcs.authSvc,
		})
		sdkapi.RegisterRoutes(r, &sdkapi.DispatchJobsBatchState{Repo: repos.dispatchJobRepo})
		sdkapi.RegisterAuditRoutes(r, &sdkapi.AuditBatchState{Repo: repos.auditRepo, Apps: repos.applicationRepo, Clients: repos.clientRepo})
	})

	// Accept-and-ignore unknown request-body fields (serde-style leniency) so
	// the SPA's superset payloads stop 400-ing. Must run after every route has
	// registered; keep in sync with the dump-spec tool so the lockfile matches.
	httpcompat.RelaxRequestBodies(humaAPI)

	// Exclude /bff/* from the published OpenAPI spec (the BFF
	// handlers stay mounted and keep serving). Must run after every route
	// has registered.
	httpcompat.StripBFFPaths(humaAPI)

	return humaAPI
}
