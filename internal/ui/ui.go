// Package ui mounts the server-rendered UI under /ui: templ components from
// the framework's UI kit (internal/ui/kit), Datastar for interaction
// (internal/ui/ds), served next to the Vue SPA and switched over area by area
// (only with the owner's approval; until then the SPA stays the default).
// Architecture, conventions and the CSRF design: docs/ui/README.md.
package ui

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/assets"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/compress"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/pages"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// Config is what the UI needs from the platform wiring.
type Config struct {
	// Authenticate attaches the caller's AuthContext from the fc_session
	// cookie (the platform's Authenticator, configured as for /api).
	Authenticate func(http.Handler) http.Handler
	// CookieSecure must match the Secure flag the session cookie was set with
	// (sign-out clears it with the same attributes).
	CookieSecure bool
	// TrustedOrigins are extra origins allowed to post to /ui (the public base
	// URL when a proxy rewrites Host). Same-origin requests need nothing here.
	TrustedOrigins []string
	// Version is shown in the user menu.
	Version string
	// DisplayName resolves the signed-in principal's name (optional).
	DisplayName func(ctx context.Context, principalID string) string

	DispatchPools   pages.PoolStore
	DispatchPoolOps pages.PoolCommands
	// Clients backs the client select (search and resolution).
	Clients pages.ClientDirectory

	Connections     pages.ConnectionStore
	ConnectionOps   pages.ConnectionCommands
	ServiceAccounts pages.ServiceAccountDirectory

	EventTypes   pages.EventTypeStore
	EventTypeOps pages.EventTypeCommands
	Applications pages.EventTypeApps

	Subscriptions   pages.SubscriptionStore
	SubscriptionOps pages.SubscriptionCommands

	ApplicationStore pages.ApplicationStore
	ApplicationOps   pages.ApplicationCommands
	LoginClients     pages.LoginClientLookup
	AppDocs          pages.AppDocsLister

	ClientStore   pages.ClientStore
	ClientOps     pages.ClientCommands
	ClientConfigs pages.ClientConfigReader

	ScheduledJobs         pages.ScheduledJobStore
	ScheduledJobInstances pages.ScheduledJobInstanceStore
	ScheduledJobOps       pages.ScheduledJobCommands

	DispatchJobs     pages.DispatchJobStore
	DispatchJobOps   pages.DispatchJobCommands
	DispatchJobAudit pages.DispatchJobAudit
	ClientIdentifier func(ctx context.Context, clientID string) (string, bool)
	// DispatchJobMaxWindow is FC_DISPATCH_JOB_MAX_WINDOW_DAYS: the widest
	// created_at window the dispatch jobs page may cover (< 1: the default).
	DispatchJobMaxWindow int

	Events pages.EventStore

	Processes  pages.ProcessStore
	ProcessOps pages.ProcessCommands

	// PlatformConfig and PlatformConfigOps back the Configuration page; its
	// application select reads Applications.
	PlatformConfig    pages.ConfigService
	PlatformConfigOps pages.ConfigCommands

	// The Theme and Names settings and a client's Login Branding read and write
	// platform config through PlatformConfig and PlatformConfigOps (their
	// client page reads ClientStore). Docs backs the Documentation pages;
	// DebugEvents and DebugDispatchJobs the raw views.
	Docs              pages.DocsSource
	DebugEvents       pages.RawEventStore
	DebugDispatchJobs pages.RawDispatchJobStore

	// FunctionTrace and FunctionTraceOps back the trace switches and the trace
	// log page of a function (/ui/functions/{id}/traces).
	FunctionTrace    pages.TraceStore
	FunctionTraceOps pages.TraceCommands

	// Functions, FunctionOps and FunctionZones back the functions list and
	// drawer; its pickers read Clients, ClientStore, Applications' store
	// (ApplicationStore) and ServiceAccounts.
	Functions     pages.FunctionStore
	FunctionOps   pages.FunctionCommands
	FunctionZones pages.FunctionZones

	// IngestionEnabled is FC_INGESTION_ENABLED: only then are the ingestion
	// pages mounted and linked from the sidebar. Ingestions and IngestionOps
	// back the ingestions page; its pickers read ApplicationStore, Clients and
	// ClientStore.
	IngestionEnabled bool
	Ingestions       pages.IngestionStore
	IngestionOps     pages.IngestionCommands
	// IngestionTabs and IngestionAudit back the drawer's activity, failures
	// and history panels; IngestionSets and IngestionSetOps the sets pages
	// (who applied is named with DisplayName); IngestionTests and IngestionResolver
	// test connection (IngestionTestSubject reads the form of an unsaved
	// definition: nil leaves that route out).
	IngestionTabs        pages.IngestionTabStore
	IngestionAudit       pages.IngestionAudit
	IngestionSets        pages.IngestionSetStore
	IngestionSetOps      pages.IngestionSetCommands
	IngestionTests       pages.IngestionTestStore
	IngestionResolver    ingestion.Resolver
	IngestionTestSubject func(r *http.Request) (pages.IngestionTestSubject, error)

	// The developer portal reads the applications, their OpenAPI documents and
	// event types; its processes tab reads Processes above. The Developer Users
	// page reads and changes principals (DeveloperUsers, DeveloperUserOps).
	DeveloperApps       pages.DeveloperApps
	DeveloperSpecs      pages.DeveloperSpecs
	DeveloperEventTypes pages.DeveloperEventTypes
	DeveloperUsers      pages.DeveloperUserStore
	DeveloperUserOps    pages.DeveloperUserCommands

	// Users backs the users page (the principal API's handlers); the role
	// picker and the roles filter read UserRoles, the client names and selects
	// ClientStore.
	Users     pages.UserService
	UserRoles pages.UserRoles

	// The service accounts page reads the clients, applications and
	// connections through ClientStore, ApplicationStore, Connections and
	// ConnectionOps above.
	ServiceAccountStore pages.ServiceAccountStore
	ServiceAccountOps   pages.ServiceAccountCommands
	ServicePrincipals   pages.ServicePrincipals
	ServiceOAuthClients pages.ServiceOAuthClients
	ServiceAccountRoles pages.ServiceAccountRoles

	// Roles, RoleOps and Permissions back the roles and permissions pages; their
	// application filters and selects read Applications.
	Roles       pages.RolesStore
	RoleOps     pages.RolesCommands
	Permissions pages.PermissionCatalogue

	// Portal and PortalOps back the portal apps and portal users pages; their
	// client select reads ClientStore.
	Portal    pages.PortalStore
	PortalOps pages.PortalCommands

	// OAuthClients and OAuthClientOps back the OAuth clients page; its pickers
	// read ApplicationStore, ClientStore and Portal. ResetApprovals backs the
	// password reset approvals page.
	OAuthClients   pages.OAuthClientStore
	OAuthClientOps pages.OAuthClientCommands
	ResetApprovals pages.ResetApprovalService

	// IdentityProviders and EmailDomains back the identity providers and email
	// domain mappings pages; the role allow-list reads Roles above, the
	// primary client select and names Clients and ClientStore.
	IdentityProviders   pages.IdpStore
	IdentityProviderOps pages.IdpCommands
	EmailDomains        pages.EmailDomainStore
	EmailDomainOps      pages.EmailDomainCommands

	// The audit log and login attempts pages read their repositories; the audit
	// log's filters and labels read Applications' store (ApplicationStore) and
	// ClientStore, the login attempt drawer links through LoginPrincipals. The
	// CORS origins page reads Cors and changes it through CorsOps.
	Audit           pages.AuditStore
	LoginAttempts   pages.LoginAttemptStore
	LoginPrincipals pages.LoginPrincipals
	Cors            pages.CorsStore
	CorsOps         pages.CorsCommands

	// SignInUpstream, when set, mounts the new sign-in pages under /ui/auth
	// (docs/ui/README.md, "Sign-in pages"). It is the platform's router: each
	// step of a sign-in is one in-process call to the public endpoint the
	// current sign-in uses, so the rules are theirs. Nil leaves the pages out.
	SignInUpstream http.Handler

	// TwoFactor, when set, mounts the two-factor and passkey sign-in pages
	// (/ui/auth/2fa, /ui/auth/passkey) and the signed-in security page
	// (/ui/auth/security). Mount sets its Shell.
	TwoFactor *pages.AuthMFA
}

// Mount registers /ui and everything under it on r.
func Mount(r chi.Router, cfg Config) error {
	sh := &shell.Shell{DisplayName: cfg.DisplayName, Version: cfg.Version, Flashes: pages.FlashMessages, CookieSecure: cfg.CookieSecure, Features: shell.Features{Ingestion: cfg.IngestionEnabled}}
	csrf, err := guard.CSRF(cfg.TrustedOrigins, sh.Deny)
	if err != nil {
		return err
	}
	r.Route("/ui", func(r chi.Router) {
		// Static, public, long-cached: no session needed (the future login
		// page uses them too).
		r.Handle("/assets/*", assets.Handler())

		// The sign-in pages are what a caller without a session reaches: no
		// session guard, but the same compression, headers and CSRF check.
		if cfg.SignInUpstream != nil {
			r.Group(func(r chi.Router) {
				r.Use(compress.Handler, platformmw.CorrelationID, guard.SecurityHeaders, csrf, withSprite)
				(&pages.SignIn{Upstream: cfg.SignInUpstream, Authenticate: cfg.Authenticate}).Mount(r)
			})
		}

		// The two-factor and passkey pages are reached from the sign-in, so
		// without a session either (the challenge is held by its own cookie).
		if cfg.TwoFactor != nil {
			cfg.TwoFactor.Shell = sh
			r.Group(func(r chi.Router) {
				r.Use(compress.Handler, platformmw.CorrelationID, guard.SecurityHeaders, csrf, withSprite)
				cfg.TwoFactor.MountPublic(r)
				(&pages.AuthPasskeys{}).Mount(r)
			})
		}

		r.Group(func(r chi.Router) {
			r.Use(
				// Pages and Datastar streams go out compressed (Brotli or
				// gzip); every guard's own answer too.
				compress.Handler,
				platformmw.CorrelationID,
				cfg.Authenticate,
				guard.SecurityHeaders,
				csrf,
				guard.RequireSession,
				guard.ProfileOnly(sh.Deny),
				withSprite,
			)
			r.Get("/", func(w http.ResponseWriter, r *http.Request) {
				http.Redirect(w, r, "/ui/dispatch-pools", http.StatusFound)
			})
			r.Post("/logout", signOut(cfg.CookieSecure))
			if cfg.TwoFactor != nil {
				cfg.TwoFactor.MountSession(r)
			}
			(&pages.DispatchPools{Pools: cfg.DispatchPools, Ops: cfg.DispatchPoolOps, Clients: cfg.Clients, Shell: sh}).Mount(r)
			(&pages.Connections{Connections: cfg.Connections, Ops: cfg.ConnectionOps, Clients: cfg.Clients, ServiceAccounts: cfg.ServiceAccounts, Shell: sh}).Mount(r)
			(&pages.EventTypes{Store: cfg.EventTypes, Ops: cfg.EventTypeOps, Apps: cfg.Applications, Shell: sh}).Mount(r)
			(&pages.Subscriptions{Subs: cfg.Subscriptions, Ops: cfg.SubscriptionOps, Pools: cfg.DispatchPools, Connections: cfg.Connections, EventTypes: cfg.EventTypes, Apps: cfg.Applications, Clients: cfg.Clients, Shell: sh}).Mount(r)
			(&pages.Applications{Apps: cfg.ApplicationStore, Ops: cfg.ApplicationOps, Logins: cfg.LoginClients, Docs: cfg.AppDocs, Shell: sh}).Mount(r)
			(&pages.ClientsPage{Clients: cfg.ClientStore, Ops: cfg.ClientOps, Apps: cfg.ApplicationStore, Configs: cfg.ClientConfigs, Shell: sh}).Mount(r)
			(&pages.ScheduledJobs{Jobs: cfg.ScheduledJobs, Instances: cfg.ScheduledJobInstances, Ops: cfg.ScheduledJobOps, Clients: cfg.ClientStore, Apps: cfg.ApplicationStore, Shell: sh}).Mount(r)
			(&pages.DispatchJobs{Jobs: cfg.DispatchJobs, Ops: cfg.DispatchJobOps, Subs: cfg.Subscriptions, Connections: cfg.Connections, Audit: cfg.DispatchJobAudit, Clients: cfg.ClientStore, ClientIdentifier: cfg.ClientIdentifier, MaxWindowDays: cfg.DispatchJobMaxWindow, Shell: sh}).Mount(r)
			(&pages.Events{Events: cfg.Events, Clients: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.Processes{Store: cfg.Processes, Ops: cfg.ProcessOps, Shell: sh}).Mount(r)
			(&pages.PlatformConfig{Config: cfg.PlatformConfig, Ops: cfg.PlatformConfigOps, Apps: cfg.Applications, ClientIdentifier: cfg.ClientIdentifier, Shell: sh}).Mount(r)
			(&pages.ThemeSettings{Config: cfg.PlatformConfig, Ops: cfg.PlatformConfigOps, Shell: sh}).Mount(r)
			(&pages.ClientTheme{Clients: cfg.ClientStore, Config: cfg.PlatformConfig, Ops: cfg.PlatformConfigOps, Shell: sh}).Mount(r)
			(&pages.NamesSettings{Config: cfg.PlatformConfig, Ops: cfg.PlatformConfigOps, Shell: sh}).Mount(r)
			(&pages.Docs{Source: cfg.Docs, Shell: sh}).Mount(r)
			(&pages.DebugEvents{Events: cfg.DebugEvents, Shell: sh}).Mount(r)
			(&pages.DebugDispatchJobs{Jobs: cfg.DebugDispatchJobs, Shell: sh}).Mount(r)
			// The functions area embeds the switches (trace.Switches) in its drawer.
			trace := &pages.FunctionTrace{Store: cfg.FunctionTrace, Ops: cfg.FunctionTraceOps, Shell: sh, Reauth: pages.ReauthWith(cfg.Authenticate)}
			trace.Mount(r)
			(&pages.Functions{Functions: cfg.Functions, Ops: cfg.FunctionOps, Zones: cfg.FunctionZones, Clients: cfg.Clients, ClientNames: cfg.ClientStore, Apps: cfg.ApplicationStore, ServiceAccounts: cfg.ServiceAccounts, Trace: trace, Shell: sh}).Mount(r)
			(&pages.PortalApps{Store: cfg.Portal, Ops: cfg.PortalOps, Clients: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.PortalUsers{Store: cfg.Portal, Ops: cfg.PortalOps, Clients: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.OAuthClients{Clients: cfg.OAuthClients, Ops: cfg.OAuthClientOps, Apps: cfg.ApplicationStore, ClientNames: cfg.ClientStore, Portal: cfg.Portal, Shell: sh}).Mount(r)
			(&pages.ResetApprovals{Service: cfg.ResetApprovals, Shell: sh}).Mount(r)
			(&pages.IdentityProviders{Store: cfg.IdentityProviders, Ops: cfg.IdentityProviderOps, Roles: cfg.Roles, Clients: cfg.Clients, ClientNames: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.EmailDomains{Store: cfg.EmailDomains, Ops: cfg.EmailDomainOps, IdPs: cfg.IdentityProviders, Clients: cfg.Clients, ClientNames: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.AuditLogPage{Store: cfg.Audit, Apps: cfg.ApplicationStore, Clients: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.LoginAttempts{Store: cfg.LoginAttempts, Principals: cfg.LoginPrincipals, Shell: sh}).Mount(r)
			(&pages.CorsOrigins{Store: cfg.Cors, Ops: cfg.CorsOps, Shell: sh}).Mount(r)
			(&pages.RolesPage{Store: cfg.Roles, Ops: cfg.RoleOps, Permissions: cfg.Permissions, Apps: cfg.Applications, Shell: sh}).Mount(r)
			(&pages.PermissionsPage{Catalogue: cfg.Permissions, Apps: cfg.Applications, Shell: sh}).Mount(r)
			(&pages.ServiceAccounts{Accounts: cfg.ServiceAccountStore, Ops: cfg.ServiceAccountOps, Principals: cfg.ServicePrincipals, OAuthClients: cfg.ServiceOAuthClients, Roles: cfg.ServiceAccountRoles, Clients: cfg.ClientStore, Apps: cfg.ApplicationStore, Connections: cfg.Connections, ConnectionOps: cfg.ConnectionOps, Shell: sh}).Mount(r)
			(&pages.Users{Users: cfg.Users, Roles: cfg.UserRoles, Clients: cfg.ClientStore, Shell: sh}).Mount(r)
			(&pages.Developer{Apps: cfg.DeveloperApps, Specs: cfg.DeveloperSpecs, EventTypes: cfg.DeveloperEventTypes, Processes: cfg.Processes, Shell: sh}).Mount(r)
			(&pages.DeveloperUsers{Store: cfg.DeveloperUsers, Ops: cfg.DeveloperUserOps, Shell: sh}).Mount(r)
			if cfg.IngestionEnabled {
				(&pages.Ingestions{Store: cfg.Ingestions, Ops: cfg.IngestionOps, Apps: cfg.ApplicationStore, Clients: cfg.Clients, ClientNames: cfg.ClientStore, Shell: sh}).Mount(r)
				(&pages.IngestionTabs{Store: cfg.IngestionTabs, Audit: cfg.IngestionAudit, Shell: sh}).Mount(r)
				(&pages.IngestionSets{Sets: cfg.IngestionSets, Ops: cfg.IngestionSetOps, Names: cfg.DisplayName, Shell: sh}).Mount(r)
				(&pages.IngestionTests{Store: cfg.IngestionTests, Resolver: cfg.IngestionResolver, Subject: cfg.IngestionTestSubject, Shell: sh}).Mount(r)
			}
			(&pages.ClientOptions{Clients: cfg.Clients, Shell: sh}).Mount(r)
		})

		r.NotFound(func(w http.ResponseWriter, r *http.Request) {
			guard.SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sh.Deny(w, r, http.StatusNotFound, "There is no page here (yet). The current UI has everything that has not been ported.")
			})).ServeHTTP(w, r)
		})
	})
	return nil
}

// withSprite makes kit icons reference the served, content-hashed sprite.
func withSprite(next http.Handler) http.Handler {
	sprite := assets.URL("lucide-sprite.svg")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(kit.WithSpriteURL(r.Context(), sprite)))
	})
}

// signOut clears the session cookie (the same attributes /auth/logout uses)
// and sends the browser to the login page: a 303 after a native form post,
// a Datastar redirect after an action.
func signOut(secure bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, platformmw.ClearSessionCookie(secure))
		if ds.IsDatastar(r) {
			_ = ds.NewSSE(w, r).Redirect(guard.LoginPath)
			return
		}
		http.Redirect(w, r, guard.LoginPath, http.StatusSeeOther)
	}
}
